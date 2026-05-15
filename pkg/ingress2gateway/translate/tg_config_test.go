package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	sharedconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
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
			result := buildTargetGroupConfig(serviceRef{namespace: "default", name: tt.svcName, port: tt.port}, tt.annos, "ingress/default/test")
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

func TestBuildTargetGroupConfigFromEntries(t *testing.T) {
	svcRef := serviceRef{namespace: "default", name: "my-svc", port: 8080}

	tests := []struct {
		name    string
		entries []tgcEntry
		wantNil bool
		check   func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration)
	}{
		{
			name:    "no entries returns nil",
			entries: nil,
			wantNil: true,
		},
		{
			name: "single entry with config uses DefaultConfiguration",
			entries: []tgcEntry{
				{
					svcRef:       svcRef,
					annotations:  map[string]string{"alb.ingress.kubernetes.io/target-type": "ip"},
					migrationTag: "ingress/default/ing-a",
					routeName:    "default-ing-a-route-abc123",
				},
			},
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				require.NotNil(t, tgc.Spec.DefaultConfiguration.TargetType)
				assert.Equal(t, gatewayv1beta1.TargetTypeIP, *tgc.Spec.DefaultConfiguration.TargetType)
				assert.Empty(t, tgc.Spec.RouteConfigurations)
			},
		},
		{
			name: "multiple entries with identical config",
			entries: []tgcEntry{
				{
					svcRef:       svcRef,
					annotations:  map[string]string{"alb.ingress.kubernetes.io/target-type": "ip"},
					migrationTag: "ingress/default/ing-a",
					routeName:    "default-ing-a-route-abc123",
				},
				{
					svcRef:       svcRef,
					annotations:  map[string]string{"alb.ingress.kubernetes.io/target-type": "ip"},
					migrationTag: "ingress/default/ing-b",
					routeName:    "default-ing-b-route-def456",
				},
			},
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				require.Len(t, tgc.Spec.RouteConfigurations, 2)
				require.NotNil(t, tgc.Spec.RouteConfigurations[0].TargetGroupProps.TargetType)
				assert.Equal(t, gatewayv1beta1.TargetTypeIP, *tgc.Spec.RouteConfigurations[0].TargetGroupProps.TargetType)
				require.NotNil(t, tgc.Spec.RouteConfigurations[1].TargetGroupProps.TargetType)
				assert.Equal(t, gatewayv1beta1.TargetTypeIP, *tgc.Spec.RouteConfigurations[1].TargetGroupProps.TargetType)
			},
		},
		{
			name: "multiple entries with different config",
			entries: []tgcEntry{
				{
					svcRef: svcRef,
					annotations: map[string]string{
						"alb.ingress.kubernetes.io/backend-protocol": "HTTP",
						"alb.ingress.kubernetes.io/healthcheck-path": "/healthz",
					},
					migrationTag: "ingress/default/ing-a",
					routeName:    "default-ing-a-route-abc123",
				},
				{
					svcRef: svcRef,
					annotations: map[string]string{
						"alb.ingress.kubernetes.io/backend-protocol": "HTTPS",
						"alb.ingress.kubernetes.io/healthcheck-path": "/ready",
					},
					migrationTag: "ingress/default/ing-b",
					routeName:    "default-ing-b-route-def456",
				},
			},
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				// DefaultConfiguration should be empty since configs differ.
				assert.Nil(t, tgc.Spec.DefaultConfiguration.Protocol)
				// Should have two RouteConfigurations.
				require.Len(t, tgc.Spec.RouteConfigurations, 2)

				rc0 := tgc.Spec.RouteConfigurations[0]
				assert.Equal(t, sharedconstants.HTTPRouteKind, rc0.RouteIdentifier.RouteKind)
				assert.Equal(t, "default", rc0.RouteIdentifier.RouteNamespace)
				assert.Equal(t, "default-ing-a-route-abc123", rc0.RouteIdentifier.RouteName)
				require.NotNil(t, rc0.TargetGroupProps.Protocol)
				assert.Equal(t, gatewayv1beta1.ProtocolHTTP, *rc0.TargetGroupProps.Protocol)
				require.NotNil(t, rc0.TargetGroupProps.HealthCheckConfig)
				assert.Equal(t, "/healthz", *rc0.TargetGroupProps.HealthCheckConfig.HealthCheckPath)

				rc1 := tgc.Spec.RouteConfigurations[1]
				assert.Equal(t, "default-ing-b-route-def456", rc1.RouteIdentifier.RouteName)
				require.NotNil(t, rc1.TargetGroupProps.Protocol)
				assert.Equal(t, gatewayv1beta1.ProtocolHTTPS, *rc1.TargetGroupProps.Protocol)
				require.NotNil(t, rc1.TargetGroupProps.HealthCheckConfig)
				assert.Equal(t, "/ready", *rc1.TargetGroupProps.HealthCheckConfig.HealthCheckPath)
			},
		},
		{
			name: "entries with one empty config emit RouteConfigurations for all entries",
			entries: []tgcEntry{
				{
					svcRef:       svcRef,
					annotations:  map[string]string{"alb.ingress.kubernetes.io/target-type": "ip"},
					migrationTag: "ingress/default/ing-a",
					routeName:    "default-ing-a-route-abc123",
				},
				{
					svcRef:       svcRef,
					annotations:  map[string]string{},
					migrationTag: "ingress/default/ing-b",
					routeName:    "default-ing-b-route-def456",
				},
			},
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				// Both entries get RouteConfigurations (even the empty one gets migration tag).
				require.Len(t, tgc.Spec.RouteConfigurations, 2)
				assert.Equal(t, "default-ing-a-route-abc123", tgc.Spec.RouteConfigurations[0].RouteIdentifier.RouteName)
				require.NotNil(t, tgc.Spec.RouteConfigurations[0].TargetGroupProps.TargetType)
				assert.Equal(t, gatewayv1beta1.TargetTypeIP, *tgc.Spec.RouteConfigurations[0].TargetGroupProps.TargetType)
				assert.Equal(t, "default-ing-b-route-def456", tgc.Spec.RouteConfigurations[1].RouteIdentifier.RouteName)
			},
		},
		{
			name: "all entries empty still creates TGC with migration tags",
			entries: []tgcEntry{
				{svcRef: svcRef, annotations: map[string]string{}, migrationTag: "ingress/default/ing-a", routeName: "r1"},
				{svcRef: svcRef, annotations: map[string]string{}, migrationTag: "ingress/default/ing-b", routeName: "r2"},
			},
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				require.Len(t, tgc.Spec.RouteConfigurations, 2)
				require.NotNil(t, tgc.Spec.RouteConfigurations[0].TargetGroupProps.Tags)
				assert.Equal(t, "ingress/default/ing-a", (*tgc.Spec.RouteConfigurations[0].TargetGroupProps.Tags)[utils.MigrationTagKey])
				require.NotNil(t, tgc.Spec.RouteConfigurations[1].TargetGroupProps.Tags)
				assert.Equal(t, "ingress/default/ing-b", (*tgc.Spec.RouteConfigurations[1].TargetGroupProps.Tags)[utils.MigrationTagKey])
			},
		},
		{
			name: "migration tags are set on each RouteConfiguration",
			entries: []tgcEntry{
				{
					svcRef:       svcRef,
					annotations:  map[string]string{"alb.ingress.kubernetes.io/target-type": "ip"},
					migrationTag: "ingress/default/ing-a",
					routeName:    "route-a",
				},
				{
					svcRef:       svcRef,
					annotations:  map[string]string{"alb.ingress.kubernetes.io/target-type": "instance"},
					migrationTag: "ingress/default/ing-b",
					routeName:    "route-b",
				},
			},
			check: func(t *testing.T, tgc *gatewayv1beta1.TargetGroupConfiguration) {
				require.Len(t, tgc.Spec.RouteConfigurations, 2)
				require.NotNil(t, tgc.Spec.RouteConfigurations[0].TargetGroupProps.Tags)
				assert.Equal(t, "ingress/default/ing-a", (*tgc.Spec.RouteConfigurations[0].TargetGroupProps.Tags)[utils.MigrationTagKey])
				require.NotNil(t, tgc.Spec.RouteConfigurations[1].TargetGroupProps.Tags)
				assert.Equal(t, "ingress/default/ing-b", (*tgc.Spec.RouteConfigurations[1].TargetGroupProps.Tags)[utils.MigrationTagKey])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTargetGroupConfigFromEntries(tt.entries)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.Equal(t, utils.GetTGConfigName("default", "my-svc"), result.Name)
			assert.Equal(t, "my-svc", result.Spec.TargetReference.Name)
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
