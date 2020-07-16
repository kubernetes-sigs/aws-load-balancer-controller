package nlb

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sort"
	"strings"
)

type Builder interface {
	Build(ctx context.Context) (build.LoadBalancingStack, error)
}

type ServiceBuilder struct {
	cloud   cloud.Cloud
	cache   cache.Cache
	service *corev1.Service
	key     types.NamespacedName
}

func NewServiceBuilder(cloud cloud.Cloud, cache cache.Cache, service *corev1.Service, key types.NamespacedName) Builder {
	return &ServiceBuilder{
		cloud:   cloud,
		cache:   cache,
		service: service,
		key:     key,
	}
}

func (b *ServiceBuilder) Build(ctx context.Context) (build.LoadBalancingStack, error) {
	stack := build.LoadBalancingStack{
		ID: b.key.String(),
	}
	if !b.service.DeletionTimestamp.IsZero() {
		return stack, nil
	}
	lbSpec, err := b.loadBalancerSpec(ctx, b.service)
	if err != nil {
		return build.LoadBalancingStack{}, err
	}

	listeners, err := b.loadBalancerListeners(ctx, &stack)
	if err != nil {
		return build.LoadBalancingStack{}, err
	}

	lbSpec.Listeners = listeners

	lb := &api.LoadBalancer{
		ObjectMeta: v1.ObjectMeta{
			Name: build.ResourceIDLoadBalancer,
		},
		Spec: lbSpec,
	}

	stack.LoadBalancer = lb
	return stack, nil
}

func (b *ServiceBuilder) loadBalancerSpec(ctx context.Context, svc *corev1.Service) (api.LoadBalancerSpec, error) {
	ipAddressType := api.IPAddressTypeIPV4
	schema := api.LoadBalancerSchemaInternetFacing
	subnets, err := b.discoverSubnets(ctx, api.LoadBalancerSchema(schema))
	if err != nil {
		return api.LoadBalancerSpec{}, err
	}
	// TODO: construct LB attributes based on the NLB service annotations
	lbSpec := api.LoadBalancerSpec{
		LoadBalancerName: b.loadbalancerName(svc),
		LoadBalancerType: elbv2.LoadBalancerTypeEnumNetwork,
		IPAddressType:    ipAddressType,
		Schema:           api.LoadBalancerSchema(schema),
		SubnetMappings:   buildSubnetMappingFromSubnets(subnets),
	}
	return lbSpec, nil
}

func (b *ServiceBuilder) loadBalancerListeners(ctx context.Context, stack *build.LoadBalancingStack) ([]api.Listener, error) {
	listeners := make([]api.Listener, 0, len(b.service.Spec.Ports))
	for _, port := range b.service.Spec.Ports {
		// Build target group, then add it to the listener
		// TODO: Search stack for existing target group
		tgName := b.targetGroupName(b.service, b.key, port.TargetPort)
		// TODO: Read from annotations
		hc := api.HealthCheckConfig{
			Port:                    port.TargetPort,
			IntervalSeconds:         10,
			TimeoutSeconds:          10,
			HealthyThresholdCount:   3,
			UnhealthyThresholdCount: 3,
			Protocol:                "TCP",
		}
		tgSpec := api.TargetGroupSpec{
			TargetGroupName:   tgName,
			TargetType:        elbv2.TargetTypeEnumIp,
			Port:              int64(port.TargetPort.IntVal),
			Protocol:          "TCP",
			HealthCheckConfig: hc,
			Attributes: api.TargetGroupAttributes{
				Stickiness:      api.TargetGroupStickinessAttributes{Enabled: false, Type: api.TargetGroupStickinessTypeSourceIP},
				ProxyProtocolV2: api.TargetGroupProxyProtocolV2{Enabled: false},
			},
		}
		tg := &api.TargetGroup{
			ObjectMeta: v1.ObjectMeta{
				Name: tgName,
			},
			Spec: tgSpec,
		}
		stack.AddTargetGroup(tg)

		eb := &api.EndpointBinding{
			ObjectMeta: v1.ObjectMeta{
				Name: stack.ID + ":" + tgName,
			},
			Spec: api.EndpointBindingSpec{
				TargetGroup: api.TargetGroupReference{
					TargetGroupRef: k8s.LocalObjectReference(tg),
				},
				TargetType:  tgSpec.TargetType,
				ServiceRef:  k8s.ObjectReference(b.service),
				ServicePort: port.TargetPort,
			},
		}
		stack.AddEndpointBinding(eb)

		defaultActions := []api.ListenerAction{b.buildForwardAction(api.TargetGroupReference{TargetGroupRef: k8s.LocalObjectReference(tg)})}

		ls := api.Listener{
			Port:           int64(port.TargetPort.IntVal),
			Protocol:       "TCP",
			DefaultActions: defaultActions,
		}
		listeners = append(listeners, ls)
	}

	return listeners, nil
}

func (b *ServiceBuilder) loadbalancerName(svc *corev1.Service) string {
	name := "a" + strings.Replace(string(svc.UID), "-", "", -1)
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

func (b *ServiceBuilder) targetGroupName(svc *corev1.Service, id types.NamespacedName, port intstr.IntOrString) string {
	uuidHash := md5.New()
	_, _ = uuidHash.Write([]byte(svc.UID))
	_, _ = uuidHash.Write([]byte(port.String()))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", id.Name, id.Namespace, uuid)
}

func (b *ServiceBuilder) buildForwardAction(tgRef api.TargetGroupReference) api.ListenerAction {
	return api.ListenerAction{
		Type: api.ListenerActionTypeForward,
		Forward: &api.ForwardConfig{
			TargetGroup: tgRef,
		},
	}
}

func (b *ServiceBuilder) discoverSubnets(ctx context.Context, schema api.LoadBalancerSchema) ([]string, error) {
	subnetRoleTagKey := ""
	switch schema {
	case api.LoadBalancerSchemaInternetFacing:
		subnetRoleTagKey = build.TagKeySubnetPublicELB
	case api.LoadBalancerSchemaInternal:
		subnetRoleTagKey = build.TagKeySubnetInternalELB
	}
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", b.cloud.ClusterName())
	tagFilters := map[string][]string{subnetRoleTagKey: {"", "1"}, clusterResourceTagKey: {"owned", "shared"}}
	resources, err := b.cloud.RGT().GetResourcesAsList(ctx, &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: aws.StringSlice([]string{cloud.ResourceTypeEC2Subnet}),
		TagFilters:          cloud.NewRGTTagFiltersV2(tagFilters),
	})
	if err != nil {
		return nil, err
	}

	subnetIDs := make([]string, 0, len(resources))
	for _, resource := range resources {
		subnetARN, _ := arn.Parse(aws.StringValue(resource.ResourceARN))
		parts := strings.Split(subnetARN.Resource, "/")
		subnetID := parts[len(parts)-1]
		subnetIDs = append(subnetIDs, subnetID)
	}
	subnetIDs, err = b.chooseSubnets(ctx, subnetIDs)
	if err != nil {
		logging.FromContext(ctx).Info("failed to discover subnets", "tagFilters", tagFilters)
		return nil, errors.Wrapf(err, "failed to discover subnets")
	}

	return subnetIDs, nil
}

func (b *ServiceBuilder) chooseSubnets(ctx context.Context, subnetIDOrNames []string) ([]string, error) {
	subnets, err := b.cloud.EC2().GetSubnetsByNameOrID(ctx, subnetIDOrNames)
	if err != nil {
		return nil, err
	}
	subnetsByAZ := map[string][]string{}
	for _, subnet := range subnets {
		subnetVpcID := aws.StringValue(subnet.VpcId)
		clusterVpcID := b.cloud.VpcID()
		if subnetVpcID != clusterVpcID {
			logging.FromContext(ctx).Info("ignore subnet", "subnetVPC", subnetVpcID, "clusterVPC", clusterVpcID)
			continue
		}
		subnetAZ := aws.StringValue(subnet.AvailabilityZone)
		subnetsByAZ[subnetAZ] = append(subnetsByAZ[subnetAZ], aws.StringValue(subnet.SubnetId))
	}

	chosenSubnets := make([]string, 0, len(subnetsByAZ))
	for az, subnets := range subnetsByAZ {
		if len(subnets) == 1 {
			chosenSubnets = append(chosenSubnets, subnets[0])
		} else if len(subnets) > 1 {
			sort.Strings(subnets)
			logging.FromContext(ctx).Info("multiple subnet same AvailabilityZone", "AvailabilityZone", az,
				"chosen", subnets[0], "ignored", subnets[1:])
			chosenSubnets = append(chosenSubnets, subnets[0])
		}
	}
	if len(chosenSubnets) < 2 {
		return nil, errors.Errorf("requires at least 2 subnet in different AvailabilityZone")
	}

	return chosenSubnets, nil
}

func buildSubnetMappingFromSubnets(subnets []string) []api.SubnetMapping {
	var subnetMappings []api.SubnetMapping
	for _, subnet := range subnets {
		subnetMappings = append(subnetMappings, api.SubnetMapping{
			SubnetID: subnet,
		})
	}
	return subnetMappings
}
