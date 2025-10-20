package gateway

import (
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sort"
)

type LoadBalancerConfigMerger interface {
	Merge(gwClassLbConfig elbv2gw.LoadBalancerConfiguration, gwLbConfig elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfiguration
}

var _ LoadBalancerConfigMerger = &loadBalancerConfigMergerImpl{}

type loadBalancerConfigMergerImpl struct {
}

func NewLoadBalancerConfigMerger() LoadBalancerConfigMerger {
	return &loadBalancerConfigMergerImpl{}
}

func (merger *loadBalancerConfigMergerImpl) Merge(gwClassLbConfig elbv2gw.LoadBalancerConfiguration, gwLbConfig elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfiguration {
	mergeMode := elbv2gw.MergeModePreferGatewayClass

	if gwClassLbConfig.Spec.MergingMode != nil {
		mergeMode = *gwClassLbConfig.Spec.MergingMode
	}

	var highPriority elbv2gw.LoadBalancerConfiguration
	var lowPriority elbv2gw.LoadBalancerConfiguration
	if mergeMode == elbv2gw.MergeModePreferGateway {
		highPriority = gwLbConfig
		lowPriority = gwClassLbConfig
	} else {
		highPriority = gwClassLbConfig
		lowPriority = gwLbConfig
	}

	mergedSpec := merger.generateMergedSpec(highPriority, lowPriority)

	return elbv2gw.LoadBalancerConfiguration{
		Spec: mergedSpec,
	}
}

func (merger *loadBalancerConfigMergerImpl) generateMergedSpec(highPriority elbv2gw.LoadBalancerConfiguration, lowPriority elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfigurationSpec {
	res := elbv2gw.LoadBalancerConfigurationSpec{}

	merger.performTakeOneMerges(&res, highPriority, lowPriority)
	res.Tags = mergeTags(highPriority.Spec.Tags, lowPriority.Spec.Tags)
	res.LoadBalancerAttributes = mergeAttributes(highPriority.Spec.LoadBalancerAttributes, lowPriority.Spec.LoadBalancerAttributes, loadBalancerAttributeKeyFn, loadBalancerAttributeValueFn, loadBalancerAttributeConstructor)
	merger.mergeListenerConfig(&res, highPriority, lowPriority)

	return res
}

func (merger *loadBalancerConfigMergerImpl) mergeListenerConfig(merged *elbv2gw.LoadBalancerConfigurationSpec, highPriority elbv2gw.LoadBalancerConfiguration, lowPriority elbv2gw.LoadBalancerConfiguration) {
	listenerConfigurationMap := make(map[elbv2gw.ProtocolPort]elbv2gw.ListenerConfiguration)

	if highPriority.Spec.ListenerConfigurations != nil {
		for _, config := range *highPriority.Spec.ListenerConfigurations {
			listenerConfigurationMap[config.ProtocolPort] = config
		}
	}

	if lowPriority.Spec.ListenerConfigurations != nil {
		for _, config := range *lowPriority.Spec.ListenerConfigurations {
			_, found := listenerConfigurationMap[config.ProtocolPort]
			if !found {
				listenerConfigurationMap[config.ProtocolPort] = config
			}
		}
	}

	if len(listenerConfigurationMap) > 0 {
		listenerConfig := make([]elbv2gw.ListenerConfiguration, 0, len(listenerConfigurationMap))

		for _, cfg := range listenerConfigurationMap {
			listenerConfig = append(listenerConfig, cfg)
		}

		sort.Slice(listenerConfig, func(i, j int) bool {
			return listenerConfig[i].ProtocolPort < listenerConfig[j].ProtocolPort
		})

		merged.ListenerConfigurations = &listenerConfig
	}

}

func (merger *loadBalancerConfigMergerImpl) performTakeOneMerges(merged *elbv2gw.LoadBalancerConfigurationSpec, highPriority elbv2gw.LoadBalancerConfiguration, lowPriority elbv2gw.LoadBalancerConfiguration) {
	if highPriority.Spec.LoadBalancerName != nil {
		merged.LoadBalancerName = highPriority.Spec.LoadBalancerName
	} else {
		merged.LoadBalancerName = lowPriority.Spec.LoadBalancerName
	}

	if highPriority.Spec.Scheme != nil {
		merged.Scheme = highPriority.Spec.Scheme
	} else {
		merged.Scheme = lowPriority.Spec.Scheme
	}

	if highPriority.Spec.IpAddressType != nil {
		merged.IpAddressType = highPriority.Spec.IpAddressType
	} else {
		merged.IpAddressType = lowPriority.Spec.IpAddressType
	}

	if highPriority.Spec.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic != nil {
		merged.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic = highPriority.Spec.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic
	} else {
		merged.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic = lowPriority.Spec.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic
	}

	if highPriority.Spec.CustomerOwnedIpv4Pool != nil {
		merged.CustomerOwnedIpv4Pool = highPriority.Spec.CustomerOwnedIpv4Pool
	} else {
		merged.CustomerOwnedIpv4Pool = lowPriority.Spec.CustomerOwnedIpv4Pool
	}

	if highPriority.Spec.IPv4IPAMPoolId != nil {
		merged.IPv4IPAMPoolId = highPriority.Spec.IPv4IPAMPoolId
	} else {
		merged.IPv4IPAMPoolId = lowPriority.Spec.IPv4IPAMPoolId
	}

	if highPriority.Spec.LoadBalancerSubnets != nil {
		merged.LoadBalancerSubnets = highPriority.Spec.LoadBalancerSubnets
	} else {
		merged.LoadBalancerSubnets = lowPriority.Spec.LoadBalancerSubnets
	}

	if highPriority.Spec.LoadBalancerSubnetsSelector != nil {
		merged.LoadBalancerSubnetsSelector = highPriority.Spec.LoadBalancerSubnetsSelector
	} else {
		merged.LoadBalancerSubnetsSelector = lowPriority.Spec.LoadBalancerSubnetsSelector
	}

	if highPriority.Spec.SecurityGroups != nil {
		merged.SecurityGroups = highPriority.Spec.SecurityGroups
	} else {
		merged.SecurityGroups = lowPriority.Spec.SecurityGroups
	}

	if highPriority.Spec.SecurityGroupPrefixes != nil {
		merged.SecurityGroupPrefixes = highPriority.Spec.SecurityGroupPrefixes
	} else {
		merged.SecurityGroupPrefixes = lowPriority.Spec.SecurityGroupPrefixes
	}

	if highPriority.Spec.SourceRanges != nil {
		merged.SourceRanges = highPriority.Spec.SourceRanges
	} else {
		merged.SourceRanges = lowPriority.Spec.SourceRanges
	}

	if highPriority.Spec.EnableICMP != nil {
		merged.EnableICMP = highPriority.Spec.EnableICMP
	} else {
		merged.EnableICMP = lowPriority.Spec.EnableICMP
	}

	if highPriority.Spec.ManageBackendSecurityGroupRules != nil {
		merged.ManageBackendSecurityGroupRules = highPriority.Spec.ManageBackendSecurityGroupRules
	} else {
		merged.ManageBackendSecurityGroupRules = lowPriority.Spec.ManageBackendSecurityGroupRules
	}

	if highPriority.Spec.WAFv2 != nil {
		merged.WAFv2 = highPriority.Spec.WAFv2
	} else {
		merged.WAFv2 = lowPriority.Spec.WAFv2
	}

	if highPriority.Spec.ShieldAdvanced != nil {
		merged.ShieldAdvanced = highPriority.Spec.ShieldAdvanced
	} else {
		merged.ShieldAdvanced = lowPriority.Spec.ShieldAdvanced
	}

	if highPriority.Spec.DisableSecurityGroup != nil {
		merged.DisableSecurityGroup = highPriority.Spec.DisableSecurityGroup
	} else {
		merged.DisableSecurityGroup = lowPriority.Spec.DisableSecurityGroup
	}
}
