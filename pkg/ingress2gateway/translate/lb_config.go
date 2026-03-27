package translate

import (
	"fmt"
	"reflect"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	annotations "sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

// buildLoadBalancerConfigResource builds a LoadBalancerConfiguration from annotations.
// Returns nil if no LB-level annotations are present.
func buildLoadBalancerConfigResource(name, namespace string, annos map[string]string, listenPorts []listenPortEntry, migrationTag string) *gatewayv1beta1.LoadBalancerConfiguration {
	spec := buildLoadBalancerConfigSpec(annos, listenPorts)

	if reflect.DeepEqual(spec, gatewayv1beta1.LoadBalancerConfigurationSpec{}) {
		return nil
	}

	// Add migration tag only when we have real config
	if migrationTag != "" {
		if spec.Tags == nil {
			tags := make(map[string]string)
			spec.Tags = &tags
		}
		(*spec.Tags)[utils.MigrationTagKey] = migrationTag
	}

	return &gatewayv1beta1.LoadBalancerConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: utils.LBConfigAPIVersion,
			Kind:       gwconstants.LoadBalancerConfiguration,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

// buildLoadBalancerConfigSpec builds a LoadBalancerConfigurationSpec from Ingress annotations.
func buildLoadBalancerConfigSpec(annos map[string]string, listenPorts []listenPortEntry) gatewayv1beta1.LoadBalancerConfigurationSpec {
	spec := gatewayv1beta1.LoadBalancerConfigurationSpec{}

	if v := getString(annos, annotations.IngressSuffixScheme); v != "" {
		scheme := gatewayv1beta1.LoadBalancerScheme(v)
		spec.Scheme = &scheme
	}

	if v := getString(annos, annotations.IngressSuffixLoadBalancerName); v != "" {
		spec.LoadBalancerName = &v
	}

	if v := getString(annos, annotations.IngressSuffixIPAddressType); v != "" {
		ipType := gatewayv1beta1.LoadBalancerIpAddressType(v)
		spec.IpAddressType = &ipType
	}

	if subnets := getStringSlice(annos, annotations.IngressSuffixSubnets); len(subnets) > 0 {
		subnetConfigs := make([]gatewayv1beta1.SubnetConfiguration, 0, len(subnets))
		for _, s := range subnets {
			subnetConfigs = append(subnetConfigs, gatewayv1beta1.SubnetConfiguration{
				Identifier: s,
			})
		}
		spec.LoadBalancerSubnets = &subnetConfigs
	}

	if v := getString(annos, annotations.IngressSuffixCustomerOwnedIPv4Pool); v != "" {
		spec.CustomerOwnedIpv4Pool = &v
	}

	if v := getString(annos, annotations.IngressSuffixIPAMIPv4PoolId); v != "" {
		spec.IPv4IPAMPoolId = &v
	}

	if sgs := getStringSlice(annos, annotations.IngressSuffixSecurityGroups); len(sgs) > 0 {
		spec.SecurityGroups = &sgs
	}

	if v := getBool(annos, annotations.IngressSuffixManageSecurityGroupRules); v != nil {
		spec.ManageBackendSecurityGroupRules = v
	}

	if cidrs := getStringSlice(annos, annotations.IngressSuffixInboundCIDRs); len(cidrs) > 0 {
		spec.SourceRanges = &cidrs
	}

	if pls := getStringSlice(annos, annotations.IngressSuffixSecurityGroupPrefixLists); len(pls) > 0 {
		spec.SecurityGroupPrefixes = &pls
	}

	if attrs := getStringMap(annos, annotations.IngressSuffixLoadBalancerAttributes); len(attrs) > 0 {
		for k, v := range attrs {
			spec.LoadBalancerAttributes = append(spec.LoadBalancerAttributes, gatewayv1beta1.LoadBalancerAttribute{
				Key:   k,
				Value: v,
			})
		}
	}

	if tags := getStringMap(annos, annotations.IngressSuffixTags); len(tags) > 0 {
		spec.Tags = &tags
	}

	if capStr := getString(annos, annotations.IngressSuffixLoadBalancerCapacityReservation); capStr != "" {
		capMap := getStringMap(annos, annotations.IngressSuffixLoadBalancerCapacityReservation)
		if cuStr, ok := capMap["CapacityUnits"]; ok {
			if cu, err := strconv.ParseInt(cuStr, 10, 32); err == nil {
				spec.MinimumLoadBalancerCapacity = &gatewayv1beta1.MinimumLoadBalancerCapacity{
					CapacityUnits: int32(cu),
				}
			}
		}
	}

	if v := getString(annos, annotations.IngressSuffixWAFv2ACLARN); v != "" && v != "none" {
		spec.WAFv2 = &gatewayv1beta1.WAFv2Configuration{ACL: v}
	} else if v := getString(annos, annotations.IngressSuffixWAFv2ACLName); v != "" && v != "none" {
		spec.WAFv2 = &gatewayv1beta1.WAFv2Configuration{ACL: v}
	}

	if v := getBool(annos, annotations.IngressSuffixShieldAdvancedProtection); v != nil {
		spec.ShieldAdvanced = &gatewayv1beta1.ShieldConfiguration{Enabled: *v}
	}

	listenerConfigs := buildListenerConfigurations(annos, listenPorts)
	if len(listenerConfigs) > 0 {
		spec.ListenerConfigurations = &listenerConfigs
	}

	return spec
}

// buildListenerConfigurations builds ListenerConfiguration entries from annotations and listen-ports.
func buildListenerConfigurations(annos map[string]string, listenPorts []listenPortEntry) []gatewayv1beta1.ListenerConfiguration {
	if len(listenPorts) == 0 {
		return nil
	}

	certARNs := getStringSlice(annos, annotations.IngressSuffixCertificateARN)
	sslPolicy := getString(annos, annotations.IngressSuffixSSLPolicy)

	var mutualAuthEntries []mutualAuthEntry
	ingressAnnotationParser.ParseJSONAnnotation(annotations.IngressSuffixMutualAuthentication, &mutualAuthEntries, annos)

	var meaningful []gatewayv1beta1.ListenerConfiguration
	for _, lp := range listenPorts {
		protocolPort := gatewayv1beta1.ProtocolPort(fmt.Sprintf("%s:%d", lp.Protocol, lp.Port))
		lc := gatewayv1beta1.ListenerConfiguration{
			ProtocolPort: protocolPort,
		}

		isSecure := lp.Protocol == utils.ProtocolHTTPS

		if isSecure && len(certARNs) > 0 {
			first := certARNs[0]
			lc.DefaultCertificate = &first
			if len(certARNs) > 1 {
				for i := 1; i < len(certARNs); i++ {
					c := certARNs[i]
					lc.Certificates = append(lc.Certificates, &c)
				}
			}
		}

		if isSecure && sslPolicy != "" {
			lc.SslPolicy = &sslPolicy
		}

		lsAttrSuffix := fmt.Sprintf("%s.%s-%d", annotations.IngressSuffixlsAttsAnnotationPrefix, lp.Protocol, lp.Port)
		if attrs := getStringMap(annos, lsAttrSuffix); len(attrs) > 0 {
			for k, v := range attrs {
				lc.ListenerAttributes = append(lc.ListenerAttributes, gatewayv1beta1.ListenerAttribute{
					Key:   k,
					Value: v,
				})
			}
		}

		for _, ma := range mutualAuthEntries {
			if ma.Port == lp.Port {
				maCfg := gatewayv1beta1.MutualAuthenticationAttributes{
					Mode: gatewayv1beta1.MutualAuthenticationMode(ma.Mode),
				}
				if ma.TrustStore != "" {
					maCfg.TrustStore = &ma.TrustStore
				}
				if ma.IgnoreClientCertificateExpiry {
					maCfg.IgnoreClientCertificateExpiry = &ma.IgnoreClientCertificateExpiry
				}
				if ma.AdvertiseTrustStoreCaNames != "" {
					v := gatewayv1beta1.AdvertiseTrustStoreCaNamesEnum(ma.AdvertiseTrustStoreCaNames)
					maCfg.AdvertiseTrustStoreCaNames = &v
				}
				lc.MutualAuthentication = &maCfg
				break
			}
		}

		if lc.DefaultCertificate != nil || lc.SslPolicy != nil || len(lc.Certificates) > 0 ||
			len(lc.ListenerAttributes) > 0 || lc.MutualAuthentication != nil {
			meaningful = append(meaningful, lc)
		}
	}
	return meaningful
}

type mutualAuthEntry struct {
	Port                          int32  `json:"port"`
	Mode                          string `json:"mode"`
	TrustStore                    string `json:"trustStore"`
	IgnoreClientCertificateExpiry bool   `json:"ignoreClientCertificateExpiry"`
	AdvertiseTrustStoreCaNames    string `json:"advertiseTrustStoreCaNames"`
}
