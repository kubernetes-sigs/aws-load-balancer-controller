package translate

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

// applyIngressClassParamsToLBConfig applies IngressClassParams overrides to a LoadBalancerConfigurationSpec.
// When called multiple times on the same spec (for multiple ICPs in a group), it detects
// per-field conflicts: if a field was already set by a previous ICP to a different value, it errors.
// Fields intentionally not mapped to LB config:
// - NamespaceSelector: cluster policy, not a LB setting
// - TargetType: TG-level, handled in applyIngressClassParamsToTGProps
// - Group: handled at Ingress grouping level
// - SSLRedirectPort: handled via resolveSSLRedirectPort
func applyIngressClassParamsToLBConfig(spec *gatewayv1beta1.LoadBalancerConfigurationSpec, icp *elbv2api.IngressClassParams) error {
	if icp == nil {
		return nil
	}

	if icp.Spec.Scheme != nil {
		scheme := gatewayv1beta1.LoadBalancerScheme(*icp.Spec.Scheme)
		if spec.Scheme != nil && *spec.Scheme != scheme {
			return fmt.Errorf("conflicting IngressClassParams scheme: %q vs %q", *spec.Scheme, scheme)
		}
		spec.Scheme = &scheme
	}

	if icp.Spec.IPAddressType != nil {
		ipType := gatewayv1beta1.LoadBalancerIpAddressType(*icp.Spec.IPAddressType)
		if spec.IpAddressType != nil && *spec.IpAddressType != ipType {
			return fmt.Errorf("conflicting IngressClassParams ip-address-type: %q vs %q", *spec.IpAddressType, ipType)
		}
		spec.IpAddressType = &ipType
	}

	if icp.Spec.LoadBalancerName != "" {
		if spec.LoadBalancerName != nil && *spec.LoadBalancerName != icp.Spec.LoadBalancerName {
			return fmt.Errorf("conflicting IngressClassParams load-balancer-name: %q vs %q", *spec.LoadBalancerName, icp.Spec.LoadBalancerName)
		}
		spec.LoadBalancerName = &icp.Spec.LoadBalancerName
	}

	if icp.Spec.SSLPolicy != "" {
		if spec.ListenerConfigurations != nil {
			lcs := *spec.ListenerConfigurations
			for i := range lcs {
				if strings.HasPrefix(string(lcs[i].ProtocolPort), utils.ProtocolHTTPS) {
					if lcs[i].SslPolicy != nil && *lcs[i].SslPolicy != icp.Spec.SSLPolicy {
						return fmt.Errorf("conflicting IngressClassParams ssl-policy: %q vs %q", *lcs[i].SslPolicy, icp.Spec.SSLPolicy)
					}
					lcs[i].SslPolicy = &icp.Spec.SSLPolicy
				}
			}
			spec.ListenerConfigurations = &lcs
		}
	}

	// CertificateArn: union across ICPs (same as annotation cert-arn merge)
	if len(icp.Spec.CertificateArn) > 0 {
		if spec.ListenerConfigurations != nil {
			first := icp.Spec.CertificateArn[0]
			lcs := *spec.ListenerConfigurations
			for i := range lcs {
				lcs[i].DefaultCertificate = &first
				if len(icp.Spec.CertificateArn) > 1 {
					lcs[i].Certificates = nil
					for j := 1; j < len(icp.Spec.CertificateArn); j++ {
						c := icp.Spec.CertificateArn[j]
						lcs[i].Certificates = append(lcs[i].Certificates, &c)
					}
				}
			}
			spec.ListenerConfigurations = &lcs
		}
	}

	if len(icp.Spec.InboundCIDRs) > 0 {
		if spec.SourceRanges != nil && !reflect.DeepEqual(*spec.SourceRanges, icp.Spec.InboundCIDRs) {
			return fmt.Errorf("conflicting IngressClassParams inbound-cidrs")
		}
		spec.SourceRanges = &icp.Spec.InboundCIDRs
	}

	// Tags: union with per-key conflict detection
	if len(icp.Spec.Tags) > 0 {
		tags := make(map[string]string)
		if spec.Tags != nil {
			for k, v := range *spec.Tags {
				tags[k] = v
			}
		}
		for _, t := range icp.Spec.Tags {
			if existing, exists := tags[t.Key]; exists && existing != t.Value {
				return fmt.Errorf("conflicting IngressClassParams tag %q: %q vs %q", t.Key, existing, t.Value)
			}
			tags[t.Key] = t.Value
		}
		spec.Tags = &tags
	}

	// LoadBalancerAttributes: union with per-key conflict detection
	if len(icp.Spec.LoadBalancerAttributes) > 0 {
		existingAttrs := make(map[string]string)
		for _, a := range spec.LoadBalancerAttributes {
			existingAttrs[a.Key] = a.Value
		}
		for _, a := range icp.Spec.LoadBalancerAttributes {
			if existing, exists := existingAttrs[a.Key]; exists && existing != a.Value {
				return fmt.Errorf("conflicting IngressClassParams load-balancer-attribute %q: %q vs %q", a.Key, existing, a.Value)
			}
			if _, exists := existingAttrs[a.Key]; !exists {
				spec.LoadBalancerAttributes = append(spec.LoadBalancerAttributes, gatewayv1beta1.LoadBalancerAttribute{
					Key:   a.Key,
					Value: a.Value,
				})
				existingAttrs[a.Key] = a.Value
			}
		}
	}

	if icp.Spec.Subnets != nil {
		if len(icp.Spec.Subnets.IDs) > 0 {
			subnetConfigs := make([]gatewayv1beta1.SubnetConfiguration, 0, len(icp.Spec.Subnets.IDs))
			for _, id := range icp.Spec.Subnets.IDs {
				subnetConfigs = append(subnetConfigs, gatewayv1beta1.SubnetConfiguration{
					Identifier: string(id),
				})
			}
			if spec.LoadBalancerSubnets != nil && !reflect.DeepEqual(*spec.LoadBalancerSubnets, subnetConfigs) {
				return fmt.Errorf("conflicting IngressClassParams subnets")
			}
			spec.LoadBalancerSubnets = &subnetConfigs
		} else if len(icp.Spec.Subnets.Tags) > 0 {
			if spec.LoadBalancerSubnetsSelector != nil && !reflect.DeepEqual(*spec.LoadBalancerSubnetsSelector, icp.Spec.Subnets.Tags) {
				return fmt.Errorf("conflicting IngressClassParams subnet selectors")
			}
			spec.LoadBalancerSubnetsSelector = &icp.Spec.Subnets.Tags
		}
	}

	if len(icp.Spec.PrefixListsIDs) > 0 {
		if spec.SecurityGroupPrefixes != nil && !reflect.DeepEqual(*spec.SecurityGroupPrefixes, icp.Spec.PrefixListsIDs) {
			return fmt.Errorf("conflicting IngressClassParams prefix-lists")
		}
		spec.SecurityGroupPrefixes = &icp.Spec.PrefixListsIDs
	} else if len(icp.Spec.PrefixListsIDsLegacy) > 0 {
		if spec.SecurityGroupPrefixes != nil && !reflect.DeepEqual(*spec.SecurityGroupPrefixes, icp.Spec.PrefixListsIDsLegacy) {
			return fmt.Errorf("conflicting IngressClassParams prefix-lists")
		}
		spec.SecurityGroupPrefixes = &icp.Spec.PrefixListsIDsLegacy
	}

	if icp.Spec.WAFv2ACLArn != "" {
		newACL := icp.Spec.WAFv2ACLArn
		if spec.WAFv2 != nil && spec.WAFv2.ACL != newACL {
			return fmt.Errorf("conflicting IngressClassParams wafv2-acl: %q vs %q", spec.WAFv2.ACL, newACL)
		}
		spec.WAFv2 = &gatewayv1beta1.WAFv2Configuration{ACL: newACL}
	} else if icp.Spec.WAFv2ACLName != "" {
		newACL := icp.Spec.WAFv2ACLName
		if spec.WAFv2 != nil && spec.WAFv2.ACL != newACL {
			return fmt.Errorf("conflicting IngressClassParams wafv2-acl: %q vs %q", spec.WAFv2.ACL, newACL)
		}
		spec.WAFv2 = &gatewayv1beta1.WAFv2Configuration{ACL: newACL}
	}

	if icp.Spec.MinimumLoadBalancerCapacity != nil {
		newCap := icp.Spec.MinimumLoadBalancerCapacity.CapacityUnits
		if spec.MinimumLoadBalancerCapacity != nil && spec.MinimumLoadBalancerCapacity.CapacityUnits != newCap {
			return fmt.Errorf("conflicting IngressClassParams minimum-load-balancer-capacity: %d vs %d",
				spec.MinimumLoadBalancerCapacity.CapacityUnits, newCap)
		}
		spec.MinimumLoadBalancerCapacity = &gatewayv1beta1.MinimumLoadBalancerCapacity{CapacityUnits: newCap}
	}

	if icp.Spec.IPAMConfiguration != nil && icp.Spec.IPAMConfiguration.IPv4IPAMPoolId != nil {
		newPool := *icp.Spec.IPAMConfiguration.IPv4IPAMPoolId
		if spec.IPv4IPAMPoolId != nil && *spec.IPv4IPAMPoolId != newPool {
			return fmt.Errorf("conflicting IngressClassParams ipam-ipv4-pool-id: %q vs %q", *spec.IPv4IPAMPoolId, newPool)
		}
		spec.IPv4IPAMPoolId = &newPool
	}

	// Listeners — apply listener attributes from ICP to matching listener configurations
	if len(icp.Spec.Listeners) > 0 && spec.ListenerConfigurations != nil {
		lcs := *spec.ListenerConfigurations
		for _, icpListener := range icp.Spec.Listeners {
			for i := range lcs {
				lcProtoPort := string(lcs[i].ProtocolPort)
				icpProtoPort := fmt.Sprintf("%s:%d", icpListener.Protocol, icpListener.Port)
				if lcProtoPort == icpProtoPort && len(icpListener.ListenerAttributes) > 0 {
					// Per-key conflict detection for listener attributes
					existingAttrs := make(map[string]string)
					for _, a := range lcs[i].ListenerAttributes {
						existingAttrs[a.Key] = a.Value
					}
					for _, attr := range icpListener.ListenerAttributes {
						if existing, exists := existingAttrs[attr.Key]; exists && existing != attr.Value {
							return fmt.Errorf("conflicting IngressClassParams listener-attribute %q on %s: %q vs %q",
								attr.Key, icpProtoPort, existing, attr.Value)
						}
						if _, exists := existingAttrs[attr.Key]; !exists {
							lcs[i].ListenerAttributes = append(lcs[i].ListenerAttributes, gatewayv1beta1.ListenerAttribute{
								Key:   attr.Key,
								Value: attr.Value,
							})
							existingAttrs[attr.Key] = attr.Value
						}
					}
				}
			}
		}
		spec.ListenerConfigurations = &lcs
	}

	return nil
}

// applyICPSpecOverride copies non-nil/non-zero fields from src to dst.
// This is used to apply the merged ICP spec on top of the annotation-derived LBConfig.
// ICP always has higher priority than annotations.
func applyICPSpecOverride(dst, src *gatewayv1beta1.LoadBalancerConfigurationSpec) {
	if src.Scheme != nil {
		dst.Scheme = src.Scheme
	}
	if src.IpAddressType != nil {
		dst.IpAddressType = src.IpAddressType
	}
	if src.LoadBalancerName != nil {
		dst.LoadBalancerName = src.LoadBalancerName
	}
	if src.SourceRanges != nil {
		dst.SourceRanges = src.SourceRanges
	}
	if src.Tags != nil {
		if dst.Tags == nil {
			dst.Tags = src.Tags
		} else {
			for k, v := range *src.Tags {
				(*dst.Tags)[k] = v
			}
		}
	}
	if len(src.LoadBalancerAttributes) > 0 {
		dst.LoadBalancerAttributes = src.LoadBalancerAttributes
	}
	if src.LoadBalancerSubnets != nil {
		dst.LoadBalancerSubnets = src.LoadBalancerSubnets
	}
	if src.LoadBalancerSubnetsSelector != nil {
		dst.LoadBalancerSubnetsSelector = src.LoadBalancerSubnetsSelector
	}
	if src.SecurityGroupPrefixes != nil {
		dst.SecurityGroupPrefixes = src.SecurityGroupPrefixes
	}
	if src.WAFv2 != nil {
		dst.WAFv2 = src.WAFv2
	}
	if src.MinimumLoadBalancerCapacity != nil {
		dst.MinimumLoadBalancerCapacity = src.MinimumLoadBalancerCapacity
	}
	if src.IPv4IPAMPoolId != nil {
		dst.IPv4IPAMPoolId = src.IPv4IPAMPoolId
	}
	if src.ListenerConfigurations != nil {
		dst.ListenerConfigurations = src.ListenerConfigurations
	}
}

// applyIngressClassParamsToTGProps applies IngressClassParams overrides directly to TargetGroupProps.
func applyIngressClassParamsToTGProps(props *gatewayv1beta1.TargetGroupProps, icp *elbv2api.IngressClassParams) {
	if icp == nil {
		return
	}
	if icp.Spec.TargetType != "" {
		tt := gatewayv1beta1.TargetType(icp.Spec.TargetType)
		props.TargetType = &tt
	}
}

// resolveSSLRedirectPort returns the SSL redirect port if configured.
// IngressClassParams.SSLRedirectPort takes priority over the Ingress annotation.
func resolveSSLRedirectPort(ingAnnotations map[string]string, icp *elbv2api.IngressClassParams) *int32 {
	if icp != nil && icp.Spec.SSLRedirectPort != "" {
		port, err := strconv.ParseInt(icp.Spec.SSLRedirectPort, 10, 32)
		if err == nil {
			p := int32(port)
			return &p
		}
	}
	return getInt32(ingAnnotations, annotations.IngressSuffixSSLRedirect)
}
