package targetgroupbinding

import (
	"context"
	"errors"
	"testing"

	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/v3/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/backend"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/testutils"
)

func TestMissingTargetGroupDoesNotBlockUnrelatedSGCleanup(t *testing.T) {
	ctx := context.Background()
	staleTGB := newTGBForNetworkingCacheTest("stale")
	healthyTGB := newTGBForNetworkingCacheTest("healthy")
	staleEndpoint := newPodEndpointForNetworkingCacheTest("stale", "10.0.0.1")
	healthyEndpoint := newPodEndpointForNetworkingCacheTest("healthy", "10.0.0.2")
	k8sClient := testutils.GenerateTestClient()
	require.NoError(t, k8sClient.Create(ctx, staleTGB))
	require.NoError(t, k8sClient.Create(ctx, healthyTGB))

	sgReconciler := &recordingSecurityGroupReconciler{}
	net := networking.NewDefaultNetworkingManager(k8sClient,
		&staticPodENIInfoResolver{securityGroupID: "sg-endpoint"},
		&unexpectedNodeENIInfoResolver{}, &fakeSecurityGroupManager{},
		sgReconciler, "vpc-test", "cluster-test", nil, logr.Discard(), false)
	resourceManager := newResourceManagerForReconcileOrderingTest(
		&fakeEndpointResolver{podEndpoints: []backend.PodEndpoint{staleEndpoint}},
		net, &fakeTargetsManager{calls: &[]string{}, listErr: &elbv2types.TargetGroupNotFoundException{}})

	for attempt := 1; attempt <= 3; attempt++ {
		_, _, _, err := resourceManager.reconcileWithIPTargetType(ctx, staleTGB)
		require.ErrorContains(t, err, "TargetGroupNotFound")
		sgReconciler.calls = nil
		require.NoError(t, net.ReconcileForPodEndpoints(ctx, healthyTGB, []backend.PodEndpoint{healthyEndpoint}))
		require.Len(t, sgReconciler.calls, 1)
		t.Logf("attempt %d: healthy TGB AuthorizeOnly=%t", attempt, sgReconciler.calls[0].authorizeOnly)
	}
	assert.False(t, sgReconciler.calls[0].authorizeOnly,
		"one missing target group must not indefinitely disable SG cleanup for healthy TGBs")
}

func newResourceManagerForReconcileOrderingTest(endpointResolver backend.EndpointResolver, networkingManager networking.NetworkingManager, targetsManager TargetsManager) *defaultResourceManager {
	return &defaultResourceManager{
		endpointResolver:  endpointResolver,
		networkingManager: networkingManager,
		targetsManager:    targetsManager,
		eventRecorder:     record.NewFakeRecorder(10),
		logger:            logr.Discard(),
		metricsCollector:  lbcmetrics.NewMockCollector(),
	}
}

func newTGBForNetworkingCacheTest(name string) *elbv2api.TargetGroupBinding {
	targetType := elbv2api.TargetTypeIP
	protocol := elbv2api.NetworkingProtocolTCP
	port := intstr.FromInt32(80)
	return &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/" + name + "/1234567890abcdef",
			TargetType:     &targetType,
			ServiceRef: elbv2api.ServiceReference{
				Name: name,
				Port: port,
			},
			Networking: &elbv2api.TargetGroupBindingNetworking{
				Ingress: []elbv2api.NetworkingIngressRule{
					{
						From: []elbv2api.NetworkingPeer{
							{SecurityGroup: &elbv2api.SecurityGroup{GroupID: "sg-source"}},
						},
						Ports: []elbv2api.NetworkingPort{
							{Protocol: &protocol, Port: &port},
						},
					},
				},
			},
		},
	}
}

func newPodEndpointForNetworkingCacheTest(name string, ip string) backend.PodEndpoint {
	return backend.PodEndpoint{
		IP:   ip,
		Port: 80,
		Pod: k8s.PodInfo{
			Key: types.NamespacedName{Namespace: "default", Name: name},
			UID: types.UID(name),
		},
	}
}

type staticPodENIInfoResolver struct {
	securityGroupID string
}

func (r *staticPodENIInfoResolver) Resolve(_ context.Context, pods []k8s.PodInfo) (map[types.NamespacedName]networking.ENIInfo, error) {
	result := make(map[types.NamespacedName]networking.ENIInfo, len(pods))
	for _, pod := range pods {
		result[pod.Key] = networking.ENIInfo{
			NetworkInterfaceID: "eni-" + pod.Key.Name,
			SecurityGroups:     []string{r.securityGroupID},
		}
	}
	return result, nil
}

type unexpectedNodeENIInfoResolver struct{}

func (*unexpectedNodeENIInfoResolver) Resolve(_ context.Context, _ []*corev1.Node) (map[types.NamespacedName]networking.ENIInfo, error) {
	return nil, errors.New("unexpected NodeENIInfoResolver.Resolve call")
}

type fakeSecurityGroupManager struct{}

func (*fakeSecurityGroupManager) FetchSGInfosByID(_ context.Context, _ []string, _ ...networking.FetchSGInfoOption) (map[string]networking.SecurityGroupInfo, error) {
	return nil, errors.New("unexpected SecurityGroupManager.FetchSGInfosByID call")
}

func (*fakeSecurityGroupManager) FetchSGInfosByRequest(_ context.Context, _ *ec2sdk.DescribeSecurityGroupsInput) (map[string]networking.SecurityGroupInfo, error) {
	return map[string]networking.SecurityGroupInfo{}, nil
}

func (*fakeSecurityGroupManager) AuthorizeSGIngress(_ context.Context, _ string, _ []networking.IPPermissionInfo) error {
	return errors.New("unexpected SecurityGroupManager.AuthorizeSGIngress call")
}

func (*fakeSecurityGroupManager) RevokeSGIngress(_ context.Context, _ string, _ []networking.IPPermissionInfo) error {
	return errors.New("unexpected SecurityGroupManager.RevokeSGIngress call")
}

type securityGroupReconcileCall struct {
	authorizeOnly bool
}

type recordingSecurityGroupReconciler struct {
	calls []securityGroupReconcileCall
}

func (r *recordingSecurityGroupReconciler) ReconcileIngress(_ context.Context, _ string, _ []networking.IPPermissionInfo, opts ...networking.SecurityGroupReconcileOption) error {
	reconcileOptions := networking.SecurityGroupReconcileOptions{}
	reconcileOptions.ApplyOptions(opts...)
	r.calls = append(r.calls, securityGroupReconcileCall{authorizeOnly: reconcileOptions.AuthorizeOnly})
	return nil
}

type fakeEndpointResolver struct {
	podEndpoints      []backend.PodEndpoint
	nodePortEndpoints []backend.NodePortEndpoint
	err               error
}

func (r *fakeEndpointResolver) ResolvePodEndpoints(_ context.Context, _ types.NamespacedName, _ intstr.IntOrString, _ discovery.AddressType) ([]backend.PodEndpoint, error) {
	return r.podEndpoints, r.err
}

func (r *fakeEndpointResolver) ResolveNodePortEndpoints(_ context.Context, _ types.NamespacedName, _ intstr.IntOrString, _ ...backend.EndpointResolveOption) ([]backend.NodePortEndpoint, error) {
	return r.nodePortEndpoints, r.err
}

type fakeNetworkingManager struct {
	calls   *[]string
	podErr  error
	nodeErr error
}

func (m *fakeNetworkingManager) ReconcileForPodEndpoints(_ context.Context, _ *elbv2api.TargetGroupBinding, _ []backend.PodEndpoint) error {
	*m.calls = append(*m.calls, "networking")
	return m.podErr
}

func (m *fakeNetworkingManager) ReconcileForNodePortEndpoints(_ context.Context, _ *elbv2api.TargetGroupBinding, _ []backend.NodePortEndpoint) error {
	*m.calls = append(*m.calls, "networking")
	return m.nodeErr
}

func (m *fakeNetworkingManager) Cleanup(_ context.Context, _ *elbv2api.TargetGroupBinding) error {
	return nil
}

func (m *fakeNetworkingManager) AttemptGarbageCollection(_ context.Context) error {
	return nil
}

type fakeTargetsManager struct {
	calls   *[]string
	targets []TargetInfo
	listErr error
}

func (m *fakeTargetsManager) RegisterTargets(_ context.Context, _ *elbv2api.TargetGroupBinding, _ []elbv2types.TargetDescription) error {
	return errors.New("unexpected RegisterTargets call")
}

func (m *fakeTargetsManager) DeregisterTargets(_ context.Context, _ *elbv2api.TargetGroupBinding, _ []elbv2types.TargetDescription) error {
	return errors.New("unexpected DeregisterTargets call")
}

func (m *fakeTargetsManager) ListTargets(_ context.Context, _ *elbv2api.TargetGroupBinding) ([]TargetInfo, error) {
	*m.calls = append(*m.calls, "list-targets")
	return m.targets, m.listErr
}
