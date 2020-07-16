package build

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"sort"
	"strings"
)

const (
	DefaultIPAddressType = api.IPAddressTypeIPV4
	DefaultScheme        = api.LoadBalancerSchemaInternal
)

const (
	TagKeySubnetInternalELB = "kubernetes.io/role/internal-elb"
	TagKeySubnetPublicELB   = "kubernetes.io/role/elb"
)

func (b *defaultBuilder) buildLoadBalancerSpec(ctx context.Context, ingGroup ingress.Group) (api.LoadBalancerSpec, error) {
	ipAddressType, err := b.buildIPAddressType(ctx, ingGroup)
	if err != nil {
		return api.LoadBalancerSpec{}, err
	}

	schema, err := b.buildSchema(ctx, ingGroup)
	if err != nil {
		return api.LoadBalancerSpec{}, err
	}

	subnetMappings, err := b.buildSubnetMapping(ctx, ingGroup, schema)
	if err != nil {
		return api.LoadBalancerSpec{}, err
	}

	lbAttributes, err := b.buildLBAttributes(ctx, ingGroup)
	if err != nil {
		return api.LoadBalancerSpec{}, err
	}

	lbName := b.nameLoadBalancer(ingGroup.ID, schema)
	lbSpec := api.LoadBalancerSpec{
		LoadBalancerName: lbName,
		LoadBalancerType: elbv2.LoadBalancerTypeEnumApplication,
		IPAddressType:    ipAddressType,
		Schema:           schema,
		SubnetMappings:   subnetMappings,
		Attributes:       lbAttributes,
		Tags:             b.ingConfig.DefaultTags,
	}

	return lbSpec, nil
}

func (b *defaultBuilder) buildIPAddressType(ctx context.Context, ingGroup ingress.Group) (api.IPAddressType, error) {
	explicitIPAddressTypes := sets.String{}
	for _, ing := range ingGroup.ActiveMembers {
		rawIPAddressType := ""
		if exists := b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixLBIPAddressType, &rawIPAddressType, ing.Annotations); !exists {
			continue
		}
		explicitIPAddressTypes.Insert(rawIPAddressType)
	}
	if len(explicitIPAddressTypes) == 0 {
		return DefaultIPAddressType, nil
	} else if len(explicitIPAddressTypes) > 1 {
		return DefaultIPAddressType, errors.Errorf("conflicting IPAddressType: %v", strings.Join(explicitIPAddressTypes.List(), ","))
	}
	ipAddressType, _ := explicitIPAddressTypes.PopAny()
	return api.ParseIPAddressType(ipAddressType)
}

func (b *defaultBuilder) buildSchema(ctx context.Context, ingGroup ingress.Group) (api.LoadBalancerSchema, error) {
	explicitSchema := sets.String{}
	for _, ing := range ingGroup.ActiveMembers {
		rawSchema := ""
		if exists := b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixLBSchema, &rawSchema, ing.Annotations); !exists {
			continue
		}
		explicitSchema.Insert(rawSchema)
	}
	if len(explicitSchema) == 0 {
		return DefaultScheme, nil
	} else if len(explicitSchema) > 1 {
		return DefaultScheme, errors.Errorf("conflicting Schema: %v", strings.Join(explicitSchema.List(), ","))
	}
	schema, _ := explicitSchema.PopAny()
	return api.ParseLoadBalancerSchema(schema)
}

func (b *defaultBuilder) buildSubnetMapping(ctx context.Context, ingGroup ingress.Group, schema api.LoadBalancerSchema) ([]api.SubnetMapping, error) {
	subnets := sets.String{}
	for _, ing := range ingGroup.ActiveMembers {
		var rawSubnets []string
		if exists := b.annotationParser.ParseStringSliceAnnotation(k8s.AnnotationSuffixLBSubnets, &rawSubnets, ing.Annotations); !exists {
			continue
		}

		if subnets.Len() == 0 {
			subnets = sets.NewString(rawSubnets...)
		} else if !subnets.Equal(sets.NewString(rawSubnets...)) {
			return nil, errors.Errorf("conflicting Subnets: %v - %v", subnets.List(), rawSubnets)
		}
	}

	if len(subnets) > 0 {
		chosenSubnets, err := b.chooseSubnetsForALB(ctx, subnets.List())
		if err != nil {
			return nil, err
		}
		if len(chosenSubnets) != len(subnets) {
			return nil, errors.Errorf("invalid subnet setting: %v - %v", subnets.List(), chosenSubnets)
		}
		return buildSubnetMappingFromSubnets(chosenSubnets), nil
	}

	chosenSubnets, err := b.discoverSubnets(ctx, schema)
	if err != nil {
		return nil, err
	}
	return buildSubnetMappingFromSubnets(chosenSubnets), nil
}

//TODO(@M00nF1sh): If we move subnet discovery and instance SG discovery as builder context,
// we will integration test model builder as whole easily.
func (b *defaultBuilder) discoverSubnets(ctx context.Context, schema api.LoadBalancerSchema) ([]string, error) {
	subnetRoleTagKey := ""
	switch schema {
	case api.LoadBalancerSchemaInternetFacing:
		subnetRoleTagKey = TagKeySubnetPublicELB
	case api.LoadBalancerSchemaInternal:
		subnetRoleTagKey = TagKeySubnetInternalELB
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
	subnetIDs, err = b.chooseSubnetsForALB(ctx, subnetIDs)
	if err != nil {
		logging.FromContext(ctx).Info("failed to discover subnets", "tagFilters", tagFilters)
		return nil, errors.Wrapf(err, "failed to discover subnets")
	}

	return subnetIDs, nil
}

func (b *defaultBuilder) chooseSubnetsForALB(ctx context.Context, subnetIDOrNames []string) ([]string, error) {
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
