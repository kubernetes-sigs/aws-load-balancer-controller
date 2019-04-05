package build

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"net"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"strings"
)

func (b *defaultBuilder) buildLBSecurityGroups(ctx context.Context, stack *LoadBalancingStack,
	ingGroup ingress.Group, portsByIngress map[types.NamespacedName]sets.Int64, ipAddressType api.IPAddressType) ([]api.SecurityGroupReference, error) {

	externalSGs := sets.String{}
	for _, ing := range ingGroup.ActiveMembers {
		var sgList []string
		if exists := b.annotationParser.ParseStringSliceAnnotation(k8s.AnnotationSuffixLBSecurityGroups, &sgList, ing.Annotations); !exists {
			continue
		}
		if externalSGs.Len() == 0 {
			externalSGs = sets.NewString(sgList...)
		} else if !externalSGs.Equal(sets.NewString(sgList...)) {
			return nil, errors.Errorf("conflicting SecurityGroup: %v - %v", externalSGs.List(), sgList)
		}
	}

	if len(externalSGs) != 0 {
		sgRefs := make([]api.SecurityGroupReference, 0, len(externalSGs))
		for sg := range externalSGs {
			sgRefs = append(sgRefs, api.SecurityGroupReference{
				SecurityGroupID: sg,
			})
		}
		return sgRefs, nil
	}

	lbSG, err := b.buildManagedLBSecurityGroup(ctx, stack, ingGroup, portsByIngress, ipAddressType)
	if err != nil {
		return nil, err
	}
	return []api.SecurityGroupReference{{SecurityGroupRef: k8s.LocalObjectReference(lbSG)}}, nil
}

func (b *defaultBuilder) buildManagedLBSecurityGroup(ctx context.Context, stack *LoadBalancingStack,
	ingGroup ingress.Group, portsByIngress map[types.NamespacedName]sets.Int64, ipAddressType api.IPAddressType) (*api.SecurityGroup, error) {

	cidrIPV4ByPort := map[int64][]string{}
	cidrIPV6ByPort := map[int64][]string{}
	ports := sets.Int64{}
	for _, ing := range ingGroup.ActiveMembers {
		ingKey := k8s.NamespacedName(ing)
		ingPorts := portsByIngress[ingKey]
		ports = ports.Union(ingPorts)

		var cidrs []string
		if exists := b.annotationParser.ParseStringSliceAnnotation(k8s.AnnotationSuffixLBInboundCIDRs, &cidrs, ing.Annotations); !exists {
			continue
		}
		var IPV4CIDRs, IPV6CIDRs []string
		for _, cidr := range cidrs {
			ip, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, err
			}
			switch len(ip) {
			case net.IPv4len:
				IPV4CIDRs = append(IPV4CIDRs, cidr)
			case net.IPv6len:
				IPV6CIDRs = append(IPV6CIDRs, cidr)
			default:
				return nil, errors.Errorf("CIDR must use an IPv4 or IPv6 address: %v, Ingress: %v", cidr, ingKey.String())
			}
		}
		for port, _ := range ingPorts {
			_, existsIPV4 := cidrIPV4ByPort[port]
			_, existsIPV6 := cidrIPV6ByPort[port]
			if existsIPV4 || existsIPV6 {
				return nil, errors.Errorf("multiple Ingress defined CIDR for port %v", port)
			}
			cidrIPV4ByPort[port] = IPV4CIDRs
			cidrIPV6ByPort[port] = IPV6CIDRs
		}
	}

	var permissions []api.IPPermission
	for port := range ports {
		IPV4CIDRs := sets.NewString(cidrIPV4ByPort[port]...)
		IPV6CIDRs := sets.NewString(cidrIPV6ByPort[port]...)
		if len(IPV4CIDRs) == 0 && len(IPV6CIDRs) == 0 {
			IPV4CIDRs.Insert("0.0.0.0/0")
			if ipAddressType == api.IPAddressTypeDualstack {
				IPV4CIDRs.Insert("::/0")
			}
		}

		for _, cidr := range IPV4CIDRs.List() {
			permissions = append(permissions, api.IPPermission{
				FromPort:   port,
				ToPort:     port,
				IPProtocol: intstr.FromString("tcp"),
				CIDRIP:     cidr,
			})
		}
		for _, cidr := range IPV6CIDRs.List() {
			permissions = append(permissions, api.IPPermission{
				FromPort:   port,
				ToPort:     port,
				IPProtocol: intstr.FromString("tcp"),
				CIDRIPV6:   cidr,
			})
		}
	}

	sgName := b.nameManagedLBSecurityGroup(ingGroup.ID)
	sg := &api.SecurityGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: ResourceIDManagedLBSecurityGroup,
		},
		Spec: api.SecurityGroupSpec{
			SecurityGroupName: sgName,
			Description:       fmt.Sprintf("[k8s] Managed SecurityGroup for LoadBalancer"),
			Permissions:       permissions,
		},
	}
	stack.ManagedLBSecurityGroup = sg

	instanceSGIDs, err := b.buildInstanceSecurityGroups(ctx)
	if err != nil {
		return nil, err
	}
	stack.InstanceSecurityGroups = instanceSGIDs

	return sg, nil
}

// TODO(@M00nF1sh): optimize this!! optimize this!! optimize this!! this part smells too!!!!
func (b *defaultBuilder) buildInstanceSecurityGroups(ctx context.Context) ([]string, error) {
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", b.cloud.ClusterName())
	tagFilters := map[string][]string{clusterResourceTagKey: {"owned", "shared"}}
	resources, err := b.cloud.RGT().GetResourcesAsList(ctx, &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: aws.StringSlice([]string{cloud.ResourceTypeEC2SecurityGroup}),
		TagFilters:          cloud.NewRGTTagFiltersV2(tagFilters),
	})
	if err != nil {
		return nil, err
	}

	var sgIDs []string
	for _, resource := range resources {
		sgARN, err := arn.Parse(aws.StringValue(resource.ResourceARN))
		if err != nil {
			return nil, err
		}
		parts := strings.Split(sgARN.Resource, "/")
		sgID := parts[len(parts)-1]
		sgIDs = append(sgIDs, sgID)
	}

	instances, err := b.findInstances(ctx)
	if err != nil {
		return nil, err
	}

	instanceGroupIDs := sets.String{}
	for _, instance := range instances {
		for _, sg := range instance.SecurityGroups {
			instanceGroupIDs.Insert(aws.StringValue(sg.GroupId))
		}
	}
	return sets.NewString(sgIDs...).Intersection(instanceGroupIDs).List(), nil
}

func (b *defaultBuilder) findInstances(ctx context.Context) ([]*ec2.Instance, error) {
	nodes, err := b.getNodePool(ctx)
	if err != nil {
		return nil, err
	}
	instanceIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		instanceID, err := b.getNodeInstanceID(ctx, node)
		if err != nil {
			return nil, err
		}
		instanceIDs = append(instanceIDs, instanceID)
	}
	return b.cloud.EC2().DescribeInstancesAsList(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice(instanceIDs),
	})
}

func (b *defaultBuilder) getNodePool(ctx context.Context) ([]*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := b.cache.List(ctx, nil, nodeList); err != nil {
		return nil, err
	}
	nodes := make([]*corev1.Node, 0, len(nodeList.Items))
	for index, _ := range nodeList.Items {
		node := &nodeList.Items[index]
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		if s, ok := node.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"]; ok {
			if strings.ToUpper(s) == "TRUE" {
				continue
			}
		}
		nodes = append(nodes, node)

		// TODO(@M000nF1sh): node condition check.
	}
	return nodes, nil
}

func (b *defaultBuilder) getNodeInstanceID(ctx context.Context, node *corev1.Node) (string, error) {
	nodeVersion, _ := semver.ParseTolerant(node.Status.NodeInfo.KubeletVersion)
	if nodeVersion.Major == 1 && nodeVersion.Minor <= 10 {
		return node.Spec.DoNotUse_ExternalID, nil
	}

	providerID := node.Spec.ProviderID
	if providerID == "" {
		return "", errors.Errorf("no providerID found for node %s", node.Name)
	}

	parts := strings.Split(providerID, "/")
	return parts[len(parts)-1], nil
}
