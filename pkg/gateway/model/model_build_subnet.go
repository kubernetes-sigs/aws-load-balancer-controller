package model

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/model/subnet"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

type subnetModelBuilder interface {
	buildLoadBalancerSubnets(ctx context.Context, gwSubnetConfig *[]elbv2gw.SubnetConfiguration, gwSubnetTagSelectors *map[string][]string, scheme elbv2model.LoadBalancerScheme, ipAddressType elbv2model.IPAddressType, stack core.Stack) ([]elbv2model.SubnetMapping, bool, error)
}

type subnetModelBuilderImpl struct {
	loadBalancerType   elbv2model.LoadBalancerType
	subnetMutatorChain []subnet.Mutator

	trackingProvider    tracking.Provider
	subnetsResolver     networking.SubnetsResolver
	elbv2TaggingManager elbv2deploy.TaggingManager
}

func newSubnetModelBuilder(loadBalancerType elbv2model.LoadBalancerType, trackingProvider tracking.Provider, subnetsResolver networking.SubnetsResolver, elbv2TaggingManager elbv2deploy.TaggingManager) subnetModelBuilder {
	var subnetMutatorChain []subnet.Mutator

	if loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
		subnetMutatorChain = []subnet.Mutator{
			subnet.NewEIPMutator(),
			subnet.NewPrivateIPv4Mutator(),
			subnet.NewIPv6Mutator(),
			subnet.NewSourceNATMutator(),
		}
	}

	return &subnetModelBuilderImpl{
		loadBalancerType:   loadBalancerType,
		subnetMutatorChain: subnetMutatorChain,

		trackingProvider:    trackingProvider,
		subnetsResolver:     subnetsResolver,
		elbv2TaggingManager: elbv2TaggingManager,
	}
}

func (subnetBuilder *subnetModelBuilderImpl) buildLoadBalancerSubnets(ctx context.Context, gwSubnetConfig *[]elbv2gw.SubnetConfiguration, gwSubnetTagSelectors *map[string][]string, scheme elbv2model.LoadBalancerScheme, ipAddressType elbv2model.IPAddressType, stack core.Stack) ([]elbv2model.SubnetMapping, bool, error) {
	sourceNATEnabled, err := subnetBuilder.validateSubnetsInput(gwSubnetConfig, scheme, ipAddressType)

	if err != nil {
		return nil, false, err
	}

	resolvedEC2Subnets, err := subnetBuilder.resolveEC2Subnets(ctx, stack, gwSubnetConfig, gwSubnetTagSelectors, scheme)

	if err != nil {
		return nil, false, err
	}

	resultPtrs := make([]*elbv2model.SubnetMapping, 0)

	for _, ec2Subnet := range resolvedEC2Subnets {
		resultPtrs = append(resultPtrs, &elbv2model.SubnetMapping{
			SubnetID: *ec2Subnet.SubnetId,
		})
	}

	var subnetConfig []elbv2gw.SubnetConfiguration

	if gwSubnetConfig != nil {
		subnetConfig = *gwSubnetConfig
	}

	for _, mutator := range subnetBuilder.subnetMutatorChain {
		err := mutator.Mutate(resultPtrs, resolvedEC2Subnets, subnetConfig)
		if err != nil {
			return nil, false, err
		}
	}

	result := make([]elbv2model.SubnetMapping, 0, len(resultPtrs))

	for _, v := range resultPtrs {
		result = append(result, *v)
	}

	return result, sourceNATEnabled, nil
}

func (subnetBuilder *subnetModelBuilderImpl) validateSubnetsInput(subnetConfigsPtr *[]elbv2gw.SubnetConfiguration, scheme elbv2model.LoadBalancerScheme, ipAddressType elbv2model.IPAddressType) (bool, error) {
	if subnetConfigsPtr == nil || len(*subnetConfigsPtr) == 0 {
		return false, nil
	}

	subnetsConfig := *subnetConfigsPtr
	identifierSpecified := subnetsConfig[0].Identifier != ""
	eipAllocationSpecified := subnetsConfig[0].EIPAllocation != nil
	ipv6AllocationSpecified := subnetsConfig[0].IPv6Allocation != nil
	privateIPv4AllocationSpecified := subnetsConfig[0].PrivateIPv4Allocation != nil
	sourceNATSpecified := subnetsConfig[0].SourceNatIPv6Prefix != nil

	if eipAllocationSpecified {
		if subnetBuilder.loadBalancerType != elbv2model.LoadBalancerTypeNetwork {
			return false, errors.Errorf("EIP Allocation is only allowed for Network LoadBalancers")
		}

		if scheme != elbv2model.LoadBalancerSchemeInternetFacing {
			return false, errors.Errorf("EIPAllocation can only be set for internet facing load balancers")
		}
	}

	if ipv6AllocationSpecified {
		if subnetBuilder.loadBalancerType != elbv2model.LoadBalancerTypeNetwork {
			return false, errors.Errorf("IPv6Allocation is only supported for Network LoadBalancers")
		}

		if ipAddressType != elbv2model.IPAddressTypeDualStack {
			return false, errors.Errorf("IPv6Allocation can only be set for dualstack load balancers")
		}
	}

	if privateIPv4AllocationSpecified {
		if subnetBuilder.loadBalancerType != elbv2model.LoadBalancerTypeNetwork {
			return false, errors.Errorf("PrivateIPv4Allocation is only supported for Network LoadBalancers")
		}

		if scheme != elbv2model.LoadBalancerSchemeInternal {
			return false, errors.Errorf("PrivateIPv4Allocation can only be set for internal load balancers")
		}
	}

	if sourceNATSpecified {
		if subnetBuilder.loadBalancerType != elbv2model.LoadBalancerTypeNetwork {
			return false, errors.Errorf("SourceNatIPv6Prefix is only supported for Network LoadBalancers")
		}
	}

	for _, subnetConfig := range subnetsConfig {
		if (subnetConfig.Identifier != "") != identifierSpecified {
			return false, errors.Errorf("Either specify all subnet identifiers or none.")
		}

		if (subnetConfig.EIPAllocation != nil) != eipAllocationSpecified {
			return false, errors.Errorf("Either specify all eip allocations or none.")
		}

		if (subnetConfig.IPv6Allocation != nil) != ipv6AllocationSpecified {
			return false, errors.Errorf("Either specify all ipv6 allocations or none.")
		}

		if (subnetConfig.PrivateIPv4Allocation != nil) != privateIPv4AllocationSpecified {
			return false, errors.Errorf("Either specify all private ipv4 allocations or none.")
		}

		if (subnetConfig.SourceNatIPv6Prefix != nil) != sourceNATSpecified {
			return false, errors.Errorf("Either specify all source nat prefixes or none.")
		}
	}

	return sourceNATSpecified, nil
}

func (subnetBuilder *subnetModelBuilderImpl) resolveEC2Subnets(ctx context.Context, stack core.Stack, subnetConfigsPtr *[]elbv2gw.SubnetConfiguration, subnetTagSelector *map[string][]string, scheme elbv2model.LoadBalancerScheme) ([]ec2types.Subnet, error) {
	// if we have identifiers, query directly by them.
	// this assumes that validateSubnetsInput() was already ran on the input.
	if subnetConfigsPtr != nil && len(*subnetConfigsPtr) != 0 && (*subnetConfigsPtr)[0].Identifier != "" {
		nameOrIds := make([]string, 0)

		for _, configuredSubnet := range *subnetConfigsPtr {
			nameOrIds = append(nameOrIds, configuredSubnet.Identifier)
		}

		return subnetBuilder.subnetsResolver.ResolveViaNameOrIDSlice(ctx, nameOrIds,
			networking.WithSubnetsResolveLBType(subnetBuilder.loadBalancerType),
			networking.WithSubnetsResolveLBScheme(scheme),
		)
	}

	if subnetTagSelector != nil && len(*subnetTagSelector) != 0 {
		selectorTags := *subnetTagSelector
		selector := elbv2api.SubnetSelector{
			Tags: selectorTags,
		}

		return subnetBuilder.subnetsResolver.ResolveViaSelector(ctx, selector,
			networking.WithSubnetsResolveLBType(subnetBuilder.loadBalancerType),
			networking.WithSubnetsResolveLBScheme(scheme),
		)
	}

	stackTags := subnetBuilder.trackingProvider.StackTags(stack)

	sdkLBs, err := subnetBuilder.elbv2TaggingManager.ListLoadBalancers(ctx, tracking.TagsAsTagFilter(stackTags))
	if err != nil {
		return nil, err
	}

	if len(sdkLBs) == 0 || (string(scheme) != string(sdkLBs[0].LoadBalancer.Scheme)) {
		return subnetBuilder.subnetsResolver.ResolveViaDiscovery(ctx,
			networking.WithSubnetsResolveLBType(subnetBuilder.loadBalancerType),
			networking.WithSubnetsResolveLBScheme(scheme),
		)
	}

	storedSubnetIds := make([]string, 0)
	for _, availabilityZone := range sdkLBs[0].LoadBalancer.AvailabilityZones {
		subnetID := awssdk.ToString(availabilityZone.SubnetId)
		storedSubnetIds = append(storedSubnetIds, subnetID)
	}
	return subnetBuilder.subnetsResolver.ResolveViaNameOrIDSlice(ctx, storedSubnetIds,
		networking.WithSubnetsResolveLBType(subnetBuilder.loadBalancerType),
		networking.WithSubnetsResolveLBScheme(scheme),
	)

}
