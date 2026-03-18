package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

func TestBuildLoadBalancerConfigResource(t *testing.T) {
	tests := []struct {
		name    string
		annos   map[string]string
		ports   []listenPortEntry
		wantNil bool
		check   func(t *testing.T, lbc *gatewayv1beta1.LoadBalancerConfiguration)
	}{
		{
			name:    "no annotations returns nil",
			annos:   map[string]string{},
			ports:   []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantNil: true,
		},
		{
			name: "scheme annotation",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/scheme": "internet-facing",
			},
			ports: []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			check: func(t *testing.T, lbc *gatewayv1beta1.LoadBalancerConfiguration) {
				require.NotNil(t, lbc.Spec.Scheme)
				assert.Equal(t, gatewayv1beta1.LoadBalancerSchemeInternetFacing, *lbc.Spec.Scheme)
			},
		},
		{
			name: "certificate and ssl-policy on HTTPS listener",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:acm:us-west-2:123:certificate/abc",
				"alb.ingress.kubernetes.io/ssl-policy":      "ELBSecurityPolicy-TLS-1-2-2017-01",
			},
			ports: []listenPortEntry{{Protocol: "HTTPS", Port: 443}},
			check: func(t *testing.T, lbc *gatewayv1beta1.LoadBalancerConfiguration) {
				require.NotNil(t, lbc.Spec.ListenerConfigurations)
				lcs := *lbc.Spec.ListenerConfigurations
				require.Len(t, lcs, 1)
				assert.Equal(t, "arn:aws:acm:us-west-2:123:certificate/abc", *lcs[0].DefaultCertificate)
				assert.Equal(t, "ELBSecurityPolicy-TLS-1-2-2017-01", *lcs[0].SslPolicy)
			},
		},
		{
			name: "cert on HTTP listener is not added",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:acm:us-west-2:123:certificate/abc",
			},
			ports:   []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantNil: true, // cert only applies to HTTPS, so no meaningful listener config
		},
		{
			name: "load balancer attributes",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/load-balancer-attributes": "idle_timeout.timeout_seconds=120",
			},
			ports: []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			check: func(t *testing.T, lbc *gatewayv1beta1.LoadBalancerConfiguration) {
				require.Len(t, lbc.Spec.LoadBalancerAttributes, 1)
				assert.Equal(t, "idle_timeout.timeout_seconds", lbc.Spec.LoadBalancerAttributes[0].Key)
			},
		},
		{
			name: "tags include migration tag",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/tags": "Env=prod",
			},
			ports: []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			check: func(t *testing.T, lbc *gatewayv1beta1.LoadBalancerConfiguration) {
				require.NotNil(t, lbc.Spec.Tags)
				tags := *lbc.Spec.Tags
				assert.Equal(t, "prod", tags["Env"])
				assert.Contains(t, tags, utils.MigrationTagKey)
			},
		},
		{
			name: "wafv2 and shield",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/wafv2-acl-arn":              "arn:aws:wafv2:us-west-2:123:regional/webacl/my-acl/abc",
				"alb.ingress.kubernetes.io/shield-advanced-protection": "true",
			},
			ports: []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			check: func(t *testing.T, lbc *gatewayv1beta1.LoadBalancerConfiguration) {
				require.NotNil(t, lbc.Spec.WAFv2)
				assert.Equal(t, "arn:aws:wafv2:us-west-2:123:regional/webacl/my-acl/abc", lbc.Spec.WAFv2.ACL)
				require.NotNil(t, lbc.Spec.ShieldAdvanced)
				assert.True(t, lbc.Spec.ShieldAdvanced.Enabled)
			},
		},
		{
			name: "subnets and security groups",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/subnets":         "subnet-aaa,subnet-bbb",
				"alb.ingress.kubernetes.io/security-groups": "sg-111,sg-222",
			},
			ports: []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			check: func(t *testing.T, lbc *gatewayv1beta1.LoadBalancerConfiguration) {
				require.NotNil(t, lbc.Spec.LoadBalancerSubnets)
				assert.Len(t, *lbc.Spec.LoadBalancerSubnets, 2)
				require.NotNil(t, lbc.Spec.SecurityGroups)
				assert.Len(t, *lbc.Spec.SecurityGroups, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildLoadBalancerConfigResource("test-lb", "default", tt.annos, tt.ports, "ingress/default/test")
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
