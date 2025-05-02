package gateway

import (
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sort"
)

type ConfigMerger interface {
	Merge(gwClassLbConfig elbv2gw.LoadBalancerConfiguration, gwLbConfig elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfiguration
}

var _ ConfigMerger = &configMergerImpl{}

type configMergerImpl struct {
}

func NewConfigMerger() ConfigMerger {
	return &configMergerImpl{}
}

func (merger *configMergerImpl) Merge(gwClassLbConfig elbv2gw.LoadBalancerConfiguration, gwLbConfig elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfiguration {
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

func (merger *configMergerImpl) generateMergedSpec(highPriority elbv2gw.LoadBalancerConfiguration, lowPriority elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfigurationSpec {
	res := elbv2gw.LoadBalancerConfigurationSpec{}

	merger.performTakeOneMerges(&res, highPriority, lowPriority)
	merger.mergeTags(&res, highPriority, lowPriority)
	merger.mergeLoadBalancerAttributes(&res, highPriority, lowPriority)
	merger.mergeListenerConfig(&res, highPriority, lowPriority)

	return res
}

func (merger *configMergerImpl) mergeLoadBalancerAttributes(merged *elbv2gw.LoadBalancerConfigurationSpec, highPriority elbv2gw.LoadBalancerConfiguration, lowPriority elbv2gw.LoadBalancerConfiguration) {
	baseAttributesMap := make(map[string]string)

	for _, attr := range highPriority.Spec.LoadBalancerAttributes {
		baseAttributesMap[attr.Key] = attr.Value
	}

	for _, attr := range lowPriority.Spec.LoadBalancerAttributes {
		_, found := baseAttributesMap[attr.Key]
		if !found {
			baseAttributesMap[attr.Key] = attr.Value
		}
	}

	if len(baseAttributesMap) > 0 {
		mergedAttributes := make([]elbv2gw.LoadBalancerAttribute, 0, len(baseAttributesMap))

		for k, v := range baseAttributesMap {
			mergedAttributes = append(mergedAttributes, elbv2gw.LoadBalancerAttribute{
				Key:   k,
				Value: v,
			})
		}

		sort.Slice(mergedAttributes, func(i, j int) bool {
			return mergedAttributes[i].Key < mergedAttributes[j].Key
		})

		merged.LoadBalancerAttributes = mergedAttributes

	}
}

func (merger *configMergerImpl) mergeListenerConfig(merged *elbv2gw.LoadBalancerConfigurationSpec, highPriority elbv2gw.LoadBalancerConfiguration, lowPriority elbv2gw.LoadBalancerConfiguration) {
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

func (merger *configMergerImpl) mergeTags(merged *elbv2gw.LoadBalancerConfigurationSpec, highPriority elbv2gw.LoadBalancerConfiguration, lowPriority elbv2gw.LoadBalancerConfiguration) {
	baseTags := make(map[string]string)

	if highPriority.Spec.Tags != nil {
		baseTags = algorithm.MergeStringMap(baseTags, *highPriority.Spec.Tags)
	}

	if lowPriority.Spec.Tags != nil {
		baseTags = algorithm.MergeStringMap(baseTags, *lowPriority.Spec.Tags)
	}

	if len(baseTags) > 0 {
		merged.Tags = &baseTags
	}
}

func (merger *configMergerImpl) performTakeOneMerges(merged *elbv2gw.LoadBalancerConfigurationSpec, highPriority elbv2gw.LoadBalancerConfiguration, lowPriority elbv2gw.LoadBalancerConfiguration) {
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

	if highPriority.Spec.VpcId != nil {
		merged.VpcId = highPriority.Spec.VpcId
	} else {
		merged.VpcId = lowPriority.Spec.VpcId
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
}
