package translate

import (
	"fmt"
	"strconv"
	"strings"

	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

// applyIngressClassParamsToLBConfig applies IngressClassParams overrides directly to a LoadBalancerConfigurationSpec.
// ICP fields take highest priority — they override any annotation-derived values.
// Fields intentionally not mapped to LB config:
// - NamespaceSelector: cluster policy, not a LB setting
// - TargetType: TG-level, handled in applyIngressClassParamsToTGProps
// - Group: handled at Ingress grouping level task (TO-DO)
// - SSLRedirectPort: handled in buildHTTPRoutes via resolveSSLRedirectPort
func applyIngressClassParamsToLBConfig(spec *gatewayv1beta1.LoadBalancerConfigurationSpec, icp *elbv2api.IngressClassParams) {
	if icp == nil {
		return
	}

	if icp.Spec.Scheme != nil {
		scheme := gatewayv1beta1.LoadBalancerScheme(*icp.Spec.Scheme)
		spec.Scheme = &scheme
	}

	if icp.Spec.IPAddressType != nil {
		ipType := gatewayv1beta1.LoadBalancerIpAddressType(*icp.Spec.IPAddressType)
		spec.IpAddressType = &ipType
	}

	if icp.Spec.LoadBalancerName != "" {
		spec.LoadBalancerName = &icp.Spec.LoadBalancerName
	}

	if icp.Spec.SSLPolicy != "" {
		// SSL policy only applies to secure listeners
		if spec.ListenerConfigurations != nil {
			lcs := *spec.ListenerConfigurations
			for i := range lcs {
				if strings.HasPrefix(string(lcs[i].ProtocolPort), utils.ProtocolHTTPS) {
					lcs[i].SslPolicy = &icp.Spec.SSLPolicy
				}
			}
			spec.ListenerConfigurations = &lcs
		}
	}

	if len(icp.Spec.CertificateArn) > 0 {
		// Certificate ARNs apply to secure listener configs
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
		spec.SourceRanges = &icp.Spec.InboundCIDRs
	}

	if len(icp.Spec.Tags) > 0 {
		tags := make(map[string]string)
		if spec.Tags != nil {
			for k, v := range *spec.Tags {
				tags[k] = v
			}
		}
		for _, t := range icp.Spec.Tags {
			tags[t.Key] = t.Value
		}
		spec.Tags = &tags
	}

	if len(icp.Spec.LoadBalancerAttributes) > 0 {
		// ICP attributes override — replace any existing
		var attrs []gatewayv1beta1.LoadBalancerAttribute
		for _, a := range icp.Spec.LoadBalancerAttributes {
			attrs = append(attrs, gatewayv1beta1.LoadBalancerAttribute{
				Key:   a.Key,
				Value: a.Value,
			})
		}
		spec.LoadBalancerAttributes = attrs
	}

	if icp.Spec.Subnets != nil {
		if len(icp.Spec.Subnets.IDs) > 0 {
			subnetConfigs := make([]gatewayv1beta1.SubnetConfiguration, 0, len(icp.Spec.Subnets.IDs))
			for _, id := range icp.Spec.Subnets.IDs {
				subnetConfigs = append(subnetConfigs, gatewayv1beta1.SubnetConfiguration{
					Identifier: string(id),
				})
			}
			spec.LoadBalancerSubnets = &subnetConfigs
		} else if len(icp.Spec.Subnets.Tags) > 0 {
			spec.LoadBalancerSubnetsSelector = &icp.Spec.Subnets.Tags
		}
	}

	if len(icp.Spec.PrefixListsIDs) > 0 {
		spec.SecurityGroupPrefixes = &icp.Spec.PrefixListsIDs
	} else if len(icp.Spec.PrefixListsIDsLegacy) > 0 {
		spec.SecurityGroupPrefixes = &icp.Spec.PrefixListsIDsLegacy
	}

	if icp.Spec.WAFv2ACLArn != "" {
		spec.WAFv2 = &gatewayv1beta1.WAFv2Configuration{ACL: icp.Spec.WAFv2ACLArn}
	} else if icp.Spec.WAFv2ACLName != "" {
		spec.WAFv2 = &gatewayv1beta1.WAFv2Configuration{ACL: icp.Spec.WAFv2ACLName}
	}

	if icp.Spec.MinimumLoadBalancerCapacity != nil {
		spec.MinimumLoadBalancerCapacity = &gatewayv1beta1.MinimumLoadBalancerCapacity{
			CapacityUnits: icp.Spec.MinimumLoadBalancerCapacity.CapacityUnits,
		}
	}

	if icp.Spec.IPAMConfiguration != nil && icp.Spec.IPAMConfiguration.IPv4IPAMPoolId != nil {
		spec.IPv4IPAMPoolId = icp.Spec.IPAMConfiguration.IPv4IPAMPoolId
	}

	// Listeners — apply listener attributes from ICP to matching listener configurations
	if len(icp.Spec.Listeners) > 0 && spec.ListenerConfigurations != nil {
		lcs := *spec.ListenerConfigurations
		for _, icpListener := range icp.Spec.Listeners {
			for i := range lcs {
				lcProtoPort := string(lcs[i].ProtocolPort)
				icpProtoPort := fmt.Sprintf("%s:%d", icpListener.Protocol, icpListener.Port)
				if lcProtoPort == icpProtoPort && len(icpListener.ListenerAttributes) > 0 {
					// ICP listener attributes override
					lcs[i].ListenerAttributes = nil
					for _, attr := range icpListener.ListenerAttributes {
						lcs[i].ListenerAttributes = append(lcs[i].ListenerAttributes, gatewayv1beta1.ListenerAttribute{
							Key:   attr.Key,
							Value: attr.Value,
						})
					}
				}
			}
		}
		spec.ListenerConfigurations = &lcs
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
