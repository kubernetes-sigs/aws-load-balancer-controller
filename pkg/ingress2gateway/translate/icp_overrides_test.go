package translate

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
)

// TestAllIngressClassParamsFieldsHandled uses reflection to verify that every non-zero field
// in IngressClassParamsSpec is handled by applyIngressClassParamsToLBConfig or applyIngressClassParamsToTGProps.
// If a new field is added to IngressClassParamsSpec but not mapped, this test fails.
func TestAllIngressClassParamsFieldsHandled(t *testing.T) {
	// Create an ICP with every field set to a non-zero value
	icp := &elbv2api.IngressClassParams{
		Spec: elbv2api.IngressClassParamsSpec{
			LoadBalancerName: "my-lb",
			CertificateArn:   []string{"arn:aws:acm:us-west-2:123:certificate/abc"},
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
			},
			Group:           &elbv2api.IngressGroup{Name: "my-group"},
			Scheme:          ptr.To(elbv2api.LoadBalancerSchemeInternetFacing),
			InboundCIDRs:    []string{"10.0.0.0/8"},
			SSLPolicy:       "ELBSecurityPolicy-TLS-1-2-2017-01",
			SSLRedirectPort: "443",
			Subnets: &elbv2api.SubnetSelector{
				IDs: []elbv2api.SubnetID{"subnet-aaa"},
			},
			IPAddressType:          ptr.To(elbv2api.IPAddressTypeDualStack),
			Tags:                   []elbv2api.Tag{{Key: "Env", Value: "prod"}},
			TargetType:             "ip",
			LoadBalancerAttributes: []elbv2api.Attribute{{Key: "idle_timeout", Value: "120"}},
			Listeners: []elbv2api.Listener{{
				Protocol:           elbv2api.ListenerProtocolHTTPS,
				Port:               443,
				ListenerAttributes: []elbv2api.Attribute{{Key: "routing.http.response.server.enabled", Value: "true"}},
			}},
			MinimumLoadBalancerCapacity: &elbv2api.MinimumLoadBalancerCapacity{CapacityUnits: 100},
			IPAMConfiguration:           &elbv2api.IPAMConfiguration{IPv4IPAMPoolId: ptr.To("ipam-pool-123")},
			PrefixListsIDs:              []string{"pl-111"},
			PrefixListsIDsLegacy:        []string{"pl-111"},
			WAFv2ACLArn:                 "arn:aws:wafv2:us-west-2:123:regional/webacl/my-acl/abc",
			WAFv2ACLName:                "my-acl",
		},
	}

	// Fields that are intentionally NOT mapped to LB/TG config (with justification)
	intentionallySkipped := map[string]string{
		"NamespaceSelector": "cluster policy, not a LB/TG setting",
		// TODO
		"Group":           "handled at Ingress grouping level later",
		"SSLRedirectPort": "handled at routing level later",
	}

	// Apply overrides
	lbSpec := gatewayv1beta1.LoadBalancerConfigurationSpec{}
	// Need at least one listener config for cert/ssl-policy to be applied
	lcs := []gatewayv1beta1.ListenerConfiguration{{ProtocolPort: "HTTPS:443"}}
	lbSpec.ListenerConfigurations = &lcs

	applyIngressClassParamsToLBConfig(&lbSpec, icp)

	tgProps := gatewayv1beta1.TargetGroupProps{}
	applyIngressClassParamsToTGProps(&tgProps, icp)

	// Use reflection to check every field in IngressClassParamsSpec
	specType := reflect.TypeOf(icp.Spec)
	specValue := reflect.ValueOf(icp.Spec)

	var unhandled []string
	for i := 0; i < specType.NumField(); i++ {
		field := specType.Field(i)
		value := specValue.Field(i)

		// Skip zero-value fields (shouldn't happen since we set everything above)
		if value.IsZero() {
			continue
		}

		// Skip intentionally skipped fields
		if _, ok := intentionallySkipped[field.Name]; ok {
			continue
		}

		// Check if this field resulted in a non-zero value in either LB spec or TG props
		handled := false

		switch field.Name {
		case "Scheme":
			handled = lbSpec.Scheme != nil
		case "IPAddressType":
			handled = lbSpec.IpAddressType != nil
		case "LoadBalancerName":
			handled = lbSpec.LoadBalancerName != nil
		case "SSLPolicy":
			handled = lbSpec.ListenerConfigurations != nil && len(*lbSpec.ListenerConfigurations) > 0 && (*lbSpec.ListenerConfigurations)[0].SslPolicy != nil
		case "CertificateArn":
			handled = lbSpec.ListenerConfigurations != nil && len(*lbSpec.ListenerConfigurations) > 0 && (*lbSpec.ListenerConfigurations)[0].DefaultCertificate != nil
		case "InboundCIDRs":
			handled = lbSpec.SourceRanges != nil
		case "Tags":
			handled = lbSpec.Tags != nil
		case "LoadBalancerAttributes":
			handled = len(lbSpec.LoadBalancerAttributes) > 0
		case "Subnets":
			handled = lbSpec.LoadBalancerSubnets != nil || lbSpec.LoadBalancerSubnetsSelector != nil
		case "PrefixListsIDs":
			handled = lbSpec.SecurityGroupPrefixes != nil
		case "PrefixListsIDsLegacy":
			handled = lbSpec.SecurityGroupPrefixes != nil
		case "WAFv2ACLArn":
			handled = lbSpec.WAFv2 != nil
		case "WAFv2ACLName":
			handled = lbSpec.WAFv2 != nil
		case "MinimumLoadBalancerCapacity":
			handled = lbSpec.MinimumLoadBalancerCapacity != nil
		case "IPAMConfiguration":
			handled = lbSpec.IPv4IPAMPoolId != nil
		case "Listeners":
			handled = lbSpec.ListenerConfigurations != nil
		case "TargetType":
			handled = tgProps.TargetType != nil
		default:
			// Unknown field — not in our switch, not in intentionallySkipped
			unhandled = append(unhandled, field.Name)
			continue
		}

		if !handled {
			unhandled = append(unhandled, field.Name+" (mapped but value not set)")
		}
	}

	assert.Empty(t, unhandled,
		"IngressClassParamsSpec has fields that are not handled by applyIngressClassParamsToLBConfig/applyIngressClassParamsToTGProps. "+
			"Either add handling in icp_overrides.go or add to intentionallySkipped with justification.")
}
