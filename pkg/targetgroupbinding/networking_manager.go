package targetgroupbinding

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"net"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	ec2equality "sigs.k8s.io/aws-alb-ingress-controller/pkg/equality/ec2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
)

const (
	tgbNetworkingIPPermissionLabelKey   = "elbv2.k8s.aws/targetGroupBinding"
	tgbNetworkingIPPermissionLabelValue = "shared"
)

// NetworkingManager manages the networking for targetGroupBindings.
type NetworkingManager interface {
	// ReconcileForPodEndpoints reconcile network settings for TargetGroupBindings with podEndpoints.
	ReconcileForPodEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.PodEndpoint) error

	// ReconcileForNodePortEndpoints reconcile network settings for TargetGroupBindings with nodePortEndpoints.
	ReconcileForNodePortEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.NodePortEndpoint) error
}

// NewDefaultNetworkingManager constructs defaultNetworkingManager.
func NewDefaultNetworkingManager(k8sClient client.Client, podENIResolver networking.PodENIInfoResolver, nodeENIResolver networking.NodeENIInfoResolver,
	sgManager networking.SecurityGroupManager, sgReconciler networking.SecurityGroupReconciler, vpcID string, clusterName string, logger logr.Logger) *defaultNetworkingManager {
	return &defaultNetworkingManager{
		k8sClient:       k8sClient,
		podENIResolver:  podENIResolver,
		nodeENIResolver: nodeENIResolver,
		sgManager:       sgManager,
		sgReconciler:    sgReconciler,
		vpcID:           vpcID,
		clusterName:     clusterName,
		logger:          logger,

		endpointSGsByTGB:      make(map[types.NamespacedName][]string),
		endpointSGsByTGBMutex: sync.Mutex{},
		trackedEndpointSGs:    sets.NewString(),
	}
}

// default implementation for NetworkingManager.
type defaultNetworkingManager struct {
	k8sClient       client.Client
	podENIResolver  networking.PodENIInfoResolver
	nodeENIResolver networking.NodeENIInfoResolver
	sgManager       networking.SecurityGroupManager
	sgReconciler    networking.SecurityGroupReconciler
	vpcID           string
	clusterName     string
	logger          logr.Logger

	// endpointSGsByTGB are the SecurityGroups for each TargetGroupBinding's endpoints.
	endpointSGsByTGB map[types.NamespacedName][]string
	// endpointSGsByTGBMutex protects endpointSGsByTGB.
	endpointSGsByTGBMutex sync.Mutex

	// trackedEndpointSGs are the securityGroups that we have been managing it's rules.
	// we'll garbage collect rules from these trackedEndpointSGs if it's not needed.
	trackedEndpointSGs sets.String
	// trackedEndpointSGsMutex protects managedEndpointSGs
	trackedEndpointSGsMutex sync.Mutex
}

func (m *defaultNetworkingManager) ReconcileForPodEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.PodEndpoint) error {
	if tgb.Spec.Networking == nil {
		return nil
	}

	endpointSGs, err := m.resolveEndpointSGsForPodEndpoints(ctx, endpoints)
	if err != nil {
		return err
	}
	return m.reconcileForEndpointSGs(ctx, tgb, endpointSGs)
}

func (m *defaultNetworkingManager) ReconcileForNodePortEndpoints(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpoints []backend.NodePortEndpoint) error {
	if tgb.Spec.Networking == nil {
		return nil
	}

	endpointSGs, err := m.resolveEndpointSGsForNodePortEndpoints(ctx, endpoints)
	if err != nil {
		return err
	}
	return m.reconcileForEndpointSGs(ctx, tgb, endpointSGs)
}

func (m *defaultNetworkingManager) reconcileForEndpointSGs(ctx context.Context, tgb *elbv2api.TargetGroupBinding, endpointSGs []string) error {
	m.endpointSGsByTGBMutex.Lock()
	defer m.endpointSGsByTGBMutex.Unlock()

	tgbKey := k8s.NamespacedName(tgb)
	m.endpointSGsByTGB[tgbKey] = endpointSGs
	tgbsWithNetworking, err := m.fetchTGBsWithNetworking(ctx)
	if err != nil {
		return err
	}
	ingressIPPermissionsBySG, computedEndpointSGsForAllTGBs, err := m.computeDesiredIngressIPPermissionsBySG(tgbsWithNetworking)
	if err != nil {
		return err
	}

	permissionSelector := labels.SelectorFromSet(labels.Set{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue})
	for sgID, ipPermissions := range ingressIPPermissionsBySG {
		if err := m.sgReconciler.ReconcileIngress(ctx, sgID, ipPermissions,
			networking.WithPermissionSelector(permissionSelector),
			networking.WithAuthorizeOnly(!computedEndpointSGsForAllTGBs)); err != nil {
			return err
		}
	}
	return nil
}

// fetchTGBsWithNetworking returns all targetGroupsBindings with networking rules in cluster.
func (m *defaultNetworkingManager) fetchTGBsWithNetworking(ctx context.Context) (map[types.NamespacedName]*elbv2api.TargetGroupBinding, error) {
	tgbList := &elbv2api.TargetGroupBindingList{}
	if err := m.k8sClient.List(ctx, tgbList); err != nil {
		return nil, err
	}
	tgbWithNetworkingByKey := make(map[types.NamespacedName]*elbv2api.TargetGroupBinding, len(tgbList.Items))
	for i := range tgbList.Items {
		tgb := &tgbList.Items[i]
		if tgb.Spec.Networking != nil {
			tgbWithNetworkingByKey[k8s.NamespacedName(tgb)] = tgb
		}
	}
	return tgbWithNetworkingByKey, nil
}

// computeDesiredIngressIPPermissionsBySG will compute the desired Ingress IPPermissions per SecurityGroup.
// It will also GC unnecessary entries in endpointSGsByTGB, and return whether all tgb have endpointSGs specified.
func (m *defaultNetworkingManager) computeDesiredIngressIPPermissionsBySG(tgbWithNetworkingByKey map[types.NamespacedName]*elbv2api.TargetGroupBinding) (map[string][]networking.IPPermissionInfo, bool, error) {
	tgbNetworkingsBySG := make(map[string][]elbv2api.TargetGroupBindingNetworking)
	for tgbKey, endpointSGs := range m.endpointSGsByTGB {
		tgb, exists := tgbWithNetworkingByKey[tgbKey]
		if !exists {
			delete(m.endpointSGsByTGB, tgbKey)
			continue
		}
		for _, endpointSG := range endpointSGs {
			tgbNetworkingsBySG[endpointSG] = append(tgbNetworkingsBySG[endpointSG], *tgb.Spec.Networking)
		}
	}
	computedEndpointSGsForAllTGBs := len(tgbWithNetworkingByKey) == len(m.endpointSGsByTGB)
	ipPermissionsBySG := make(map[string][]networking.IPPermissionInfo)
	for sgID, tgbNetworkings := range tgbNetworkingsBySG {
		ipPermissions, err := m.computeDesiredIngressIPPermissions(tgbNetworkings)
		if err != nil {
			return nil, false, err
		}
		ipPermissionsBySG[sgID] = ipPermissions
	}
	return ipPermissionsBySG, computedEndpointSGsForAllTGBs, nil
}

func (m *defaultNetworkingManager) computeDesiredIngressIPPermissions(tgbNetworkings []elbv2api.TargetGroupBindingNetworking) ([]networking.IPPermissionInfo, error) {
	var ipPermissions []networking.IPPermissionInfo
	opts := cmp.Options{
		ec2equality.CompareOptionForIPPermission(),
		cmpopts.IgnoreFields(networking.IPPermissionInfo{}, "Labels"),
	}

	for _, tgbNetworking := range tgbNetworkings {
		for _, rule := range tgbNetworking.Ingress {
			for _, port := range rule.Ports {
				for _, peer := range rule.From {
					ipPermission, err := m.computeDesiredIngressIPPermission(port, peer)
					if err != nil {
						return nil, err
					}
					containsPermission := false
					for _, permission := range ipPermissions {
						if cmp.Equal(ipPermission, permission, opts) {
							containsPermission = true
							break
						}
					}
					if !containsPermission {
						ipPermissions = append(ipPermissions, ipPermission)
					}
				}
			}
		}
	}
	return ipPermissions, nil
}

func (m *defaultNetworkingManager) computeDesiredIngressIPPermission(port elbv2api.NetworkingPort, peer elbv2api.NetworkingPeer) (networking.IPPermissionInfo, error) {
	permissionLabels := map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue}
	protocol := "-1"
	if port.Protocol != nil {
		switch *port.Protocol {
		case elbv2api.NetworkingProtocolTCP:
			protocol = "tcp"
		case elbv2api.NetworkingProtocolUDP:
			protocol = "udp"
		}
	}
	var fromPort *int64
	var toPort *int64
	if port.Port != nil {
		fromPort = port.Port
		toPort = port.Port
	}
	if peer.SecurityGroup != nil {
		groupID := peer.SecurityGroup.GroupID
		return networking.NewGroupIDIPPermission(protocol, fromPort, toPort, groupID, permissionLabels), nil
	}
	if peer.IPBlock != nil {
		cidr := peer.IPBlock.CIDR
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return networking.IPPermissionInfo{}, err
		}
		if strings.Contains(cidr, ":") {
			return networking.NewCIDRv6IPPermission(protocol, fromPort, toPort, cidr, permissionLabels), nil
		}
		return networking.NewCIDRIPPermission(protocol, fromPort, toPort, cidr, permissionLabels), nil
	}
	return networking.IPPermissionInfo{}, errors.New("either SecurityGroup or IPBlock should be specified")
}

func (m *defaultNetworkingManager) resolveEndpointSGsForPodEndpoints(ctx context.Context, endpoints []backend.PodEndpoint) ([]string, error) {
	pods := make([]*corev1.Pod, 0, len(endpoints))
	for _, endpoint := range endpoints {
		pods = append(pods, endpoint.Pod)
	}
	eniInfoByPodKey, err := m.podENIResolver.Resolve(ctx, pods)
	if err != nil {
		return nil, err
	}
	sgIDs := sets.NewString()
	for _, eniInfo := range eniInfoByPodKey {
		sgID, err := m.resolveEndpointSGForENI(ctx, eniInfo)
		if err != nil {
			return nil, err
		}
		sgIDs.Insert(sgID)
	}
	return sgIDs.List(), nil
}

func (m *defaultNetworkingManager) resolveEndpointSGsForNodePortEndpoints(ctx context.Context, endpoints []backend.NodePortEndpoint) ([]string, error) {
	nodes := make([]*corev1.Node, 0, len(endpoints))
	for _, endpoint := range endpoints {
		nodes = append(nodes, endpoint.Node)
	}
	eniInfoByNodeKey, err := m.nodeENIResolver.Resolve(ctx, nodes)
	if err != nil {
		return nil, err
	}
	sgIDs := sets.NewString()
	for _, eniInfo := range eniInfoByNodeKey {
		sgID, err := m.resolveEndpointSGForENI(ctx, eniInfo)
		if err != nil {
			return nil, err
		}
		sgIDs.Insert(sgID)
	}
	return sgIDs.List(), nil
}

// resolveEndpointSGForENI will resolve the endpoint SecurityGroup for specific ENI.
// If there are only a single securityGroup attached, that one will be the endpoint SecurityGroup.
// If there are multiple securityGroup attached, we expect one and only one securityGroup is tagged with the cluster tag.
func (m *defaultNetworkingManager) resolveEndpointSGForENI(ctx context.Context, eniInfo networking.ENIInfo) (string, error) {
	sgIDs := eniInfo.SecurityGroups
	if len(sgIDs) == 1 {
		return sgIDs[0], nil
	}

	sgInfoByID, err := m.sgManager.FetchSGInfosByID(ctx, sgIDs...)
	if err != nil {
		return "", err
	}
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", m.clusterName)
	sgIDsWithClusterTag := sets.NewString()
	for sgID, sgInfo := range sgInfoByID {
		if _, ok := sgInfo.Tags[clusterResourceTagKey]; ok {
			sgIDsWithClusterTag.Insert(sgID)
		}
	}
	if len(sgIDsWithClusterTag) != 1 {
		return "", errors.Errorf("expect exactly one securityGroup tagged with %v for eni %v, got: %v",
			clusterResourceTagKey, eniInfo.NetworkInterfaceID, sgIDsWithClusterTag.List())
	}
	sgID, _ := sgIDsWithClusterTag.PopAny()
	return sgID, nil
}

// trackEndpointSGs will track these endpoint SecurityGroups.
func (m *defaultNetworkingManager) trackEndpointSGs(_ context.Context, sgIDs ...string) {
	m.trackedEndpointSGsMutex.Lock()
	defer m.trackedEndpointSGsMutex.Unlock()

	m.trackedEndpointSGs.Insert(sgIDs...)
}

// unTrackEndpointSGs will unTrack these endpoint SecurityGroups.
func (m *defaultNetworkingManager) unTrackEndpointSGs(_ context.Context, sgIDs ...string) {
	m.trackedEndpointSGsMutex.Lock()
	defer m.trackedEndpointSGsMutex.Unlock()

	m.trackedEndpointSGs.Delete(sgIDs...)
}

// fetchEndpointSGsFromAWS will return all endpoint securityGroups from AWS API.
// we consider a securityGroup as a endpoint securityGroup if it have the cluster tag.
// note: not all endpoint securityGroup have the cluster Tag(e.g. if a ENI only have a single securityGroup, it will still be used as endpoint securityGroup)
func (m *defaultNetworkingManager) fetchEndpointSGsFromAWS(ctx context.Context) ([]string, error) {
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", m.clusterName)
	req := &ec2sdk.DescribeSecurityGroupsInput{
		Filters: []*ec2sdk.Filter{
			{
				Name:   awssdk.String("tag:" + clusterResourceTagKey),
				Values: awssdk.StringSlice([]string{"owned", "shared"}),
			},
			{
				Name:   awssdk.String("vpc-id"),
				Values: awssdk.StringSlice([]string{m.vpcID}),
			},
		},
	}
	sgInfoByID, err := m.sgManager.FetchSGInfosByRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	return sets.StringKeySet(sgInfoByID).List(), nil
}
