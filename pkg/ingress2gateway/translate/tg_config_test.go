package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

func TestBuildTargetGroupConfig(t *testing.T) {
	tests := []struct {
		name    string
		svcName string
		annos   map[string]string
		port    int32
		wantNil bool
		check   func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration)
	}{
		{
			name:    "no TG annotations returns nil",
			svcName: "svc",
			annos:   map[string]string{},
			port:    80,
			wantNil: true,
		},
		{
			name:    "target-type ip",
			svcName: "api-svc",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/target-type": "ip",
			},
			port: 80,
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				assert.Contains(t, tgc.Name, "api-svc-tg-confi-")
				assert.Equal(t, "api-svc", tgc.Spec.TargetReference.Name)
				require.NotNil(t, tgc.Spec.DefaultConfiguration.TargetType)
				assert.Equal(t, gatewayv1beta1.TargetTypeIP, *tgc.Spec.DefaultConfiguration.TargetType)
			},
		},
		{
			name:    "health check annotations",
			svcName: "hc-svc",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/healthcheck-path":             "/health",
				"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "30",
				"alb.ingress.kubernetes.io/healthcheck-timeout-seconds":  "10",
				"alb.ingress.kubernetes.io/healthy-threshold-count":      "3",
				"alb.ingress.kubernetes.io/unhealthy-threshold-count":    "2",
				"alb.ingress.kubernetes.io/success-codes":                "200",
				"alb.ingress.kubernetes.io/healthcheck-protocol":         "HTTPS",
				"alb.ingress.kubernetes.io/healthcheck-port":             "8080",
			},
			port: 80,
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				hc := tgc.Spec.DefaultConfiguration.HealthCheckConfig
				require.NotNil(t, hc)
				assert.Equal(t, "/health", *hc.HealthCheckPath)
				assert.Equal(t, int32(30), *hc.HealthCheckInterval)
				assert.Equal(t, int32(10), *hc.HealthCheckTimeout)
				assert.Equal(t, int32(3), *hc.HealthyThresholdCount)
				assert.Equal(t, int32(2), *hc.UnhealthyThresholdCount)
				assert.Equal(t, "200", *hc.Matcher.HTTPCode)
				assert.Equal(t, gatewayv1beta1.TargetGroupHealthCheckProtocolHTTPS, *hc.HealthCheckProtocol)
				assert.Equal(t, "8080", *hc.HealthCheckPort)
			},
		},
		{
			name:    "backend protocol and version",
			svcName: "proto-svc",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/backend-protocol":         "HTTPS",
				"alb.ingress.kubernetes.io/backend-protocol-version": "HTTP2",
			},
			port: 443,
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				require.NotNil(t, tgc.Spec.DefaultConfiguration.Protocol)
				assert.Equal(t, gatewayv1beta1.ProtocolHTTPS, *tgc.Spec.DefaultConfiguration.Protocol)
				require.NotNil(t, tgc.Spec.DefaultConfiguration.ProtocolVersion)
				assert.Equal(t, gatewayv1beta1.ProtocolVersionHTTP2, *tgc.Spec.DefaultConfiguration.ProtocolVersion)
			},
		},
		{
			name:    "target group attributes",
			svcName: "attr-svc",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/target-group-attributes": "deregistration_delay.timeout_seconds=30,stickiness.enabled=true",
			},
			port: 80,
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				assert.Len(t, tgc.Spec.DefaultConfiguration.TargetGroupAttributes, 2)
			},
		},
		{
			name:    "migration tag is added",
			svcName: "tag-svc",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/target-type": "ip",
			},
			port: 80,
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				require.NotNil(t, tgc.Spec.DefaultConfiguration.Tags)
				assert.Contains(t, *tgc.Spec.DefaultConfiguration.Tags, utils.MigrationTagKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTargetGroupConfig(tt.svcName, "default", tt.annos, tt.port, "ingress/default/test")
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
