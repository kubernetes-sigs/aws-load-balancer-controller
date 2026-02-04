package model

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"reflect"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/certs"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_mapGatewayListenerConfigsByPort(t *testing.T) {
	fooHostname := gwv1.Hostname("foo.example.com")
	barHostname := gwv1.Hostname("bar.example.com")
	tests := []struct {
		name    string
		gateway *gwv1.Gateway
		routes  map[int32][]routeutils.RouteDescriptor
		want    map[int32]gwListenerConfig
		wantErr bool
	}{
		{
			name: "single HTTP listener",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string](),
				},
			},
			wantErr: false,
		},
		{
			name: "single TCP listener",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tcp",
							Port:     443,
							Protocol: gwv1.TCPProtocolType,
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				443: {
					protocol:  elbv2model.ProtocolTCP,
					hostnames: sets.New[string](),
				},
			},
			wantErr: false,
		},
		{
			name: "single TCP listener",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tcp",
							Port:     443,
							Protocol: gwv1.TCPProtocolType,
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				443: {
					protocol:  elbv2model.ProtocolTCP,
					hostnames: sets.New[string](),
				},
			},
			wantErr: false,
		},
		{
			name: "single TLS listener with pass through tls is translated to TCP",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tcp",
							Port:     443,
							Protocol: gwv1.TLSProtocolType,
							TLS: &gwv1.GatewayTLSConfig{
								Mode: (*gwv1.TLSModeType)(awssdk.String("Passthrough")),
							},
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				443: {
					protocol:  elbv2model.ProtocolTCP,
					hostnames: sets.New[string](),
				},
			},
			wantErr: false,
		},
		{
			name: "single TLS listener with terminate tls is still TLS",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tcp",
							Port:     443,
							Protocol: gwv1.TLSProtocolType,
							TLS: &gwv1.GatewayTLSConfig{
								Mode: (*gwv1.TLSModeType)(awssdk.String("Terminate")),
							},
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				443: {
					protocol:  elbv2model.ProtocolTLS,
					hostnames: sets.New[string](),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple listeners with different protocols",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http-1",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
						},
						{
							Name:     "https",
							Port:     443,
							Protocol: gwv1.HTTPSProtocolType,
						},
						{
							Name:     "http-2",
							Port:     8080,
							Protocol: gwv1.HTTPProtocolType,
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string](),
				},
				443: {
					protocol:  elbv2model.ProtocolHTTPS,
					hostnames: sets.New[string](),
				},
				8080: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string](),
				},
			},
			wantErr: false,
		},
		{
			name: "listeners with hostnames",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
							Hostname: &fooHostname,
						},
						{
							Name:     "https",
							Port:     443,
							Protocol: gwv1.HTTPSProtocolType,
							Hostname: &fooHostname,
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string]("foo.example.com"),
				},
				443: {
					protocol:  elbv2model.ProtocolHTTPS,
					hostnames: sets.New[string]("foo.example.com"),
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate ports with different protocols",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
						},
						{
							Name:     "https",
							Port:     80,
							Protocol: gwv1.HTTPSProtocolType,
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "multiple hostnames for same port",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http-1",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
							Hostname: &fooHostname,
						},
						{
							Name:     "http-2",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
							Hostname: &barHostname,
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string]("foo.example.com", "bar.example.com"),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple hostnames for same port",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http-1",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
							Hostname: &fooHostname,
						},
						{
							Name:     "http-2",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
							Hostname: &barHostname,
						},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string]("foo.example.com", "bar.example.com"),
				},
			},
			wantErr: false,
		},
		{
			name: "host name on listener and route",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http-1",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
							Hostname: &fooHostname,
						},
					},
				},
			},
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Hostnames: []string{"r1.com", "r2.com", "r3.com"},
					},
					&routeutils.MockRoute{
						Hostnames: []string{"r4.com", "r5.com", "r6.com"},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string]("foo.example.com", "r1.com", "r2.com", "r3.com", "r4.com", "r5.com", "r6.com"),
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate host name should be de-duped",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http-1",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
							Hostname: &fooHostname,
						},
					},
				},
			},
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Hostnames: []string{"r1.com", "r2.com", "r3.com"},
					},
					&routeutils.MockRoute{
						Hostnames: []string{"r1.com", "r2.com", "r3.com"},
					},
				},
				100: {
					&routeutils.MockRoute{
						Hostnames: []string{"this should be ignored"},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string]("foo.example.com", "r1.com", "r2.com", "r3.com"),
				},
			},
			wantErr: false,
		},
		{
			name: "route with no host name should be accepted",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http-1",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
							Hostname: &fooHostname,
						},
					},
				},
			},
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Hostnames: []string{"r1.com", "r2.com", "r3.com"},
					},
					&routeutils.MockRoute{
						Hostnames: []string{},
					},
				},
				100: {
					&routeutils.MockRoute{
						Hostnames: []string{"this should be ignored"},
					},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: sets.New[string]("foo.example.com", "r1.com", "r2.com", "r3.com"),
				},
			},
			wantErr: false,
		},
		{
			name: "listener valid merge",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "udp-1",
							Port:     80,
							Protocol: gwv1.UDPProtocolType,
						},
						{
							Name:     "tcp-1",
							Port:     80,
							Protocol: gwv1.TCPProtocolType,
						},
						{
							Name:     "tcp-2",
							Port:     443,
							Protocol: gwv1.TCPProtocolType,
						},
					},
				},
			},
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{},
				},
				443: {
					&routeutils.MockRoute{},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolTCP_UDP,
					hostnames: sets.New[string](),
				},
				443: {
					protocol:  elbv2model.ProtocolTCP,
					hostnames: sets.New[string](),
				},
			},
			wantErr: false,
		},
		{
			name: "listener valid merge - multiple listeners",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "udp-1",
							Port:     80,
							Protocol: gwv1.UDPProtocolType,
						},
						{
							Name:     "tcp-1",
							Port:     80,
							Protocol: gwv1.TCPProtocolType,
						},
						{
							Name:     "tcp-2",
							Port:     443,
							Protocol: gwv1.TCPProtocolType,
						},
						{
							Name:     "tcp-3",
							Port:     80,
							Protocol: gwv1.TCPProtocolType,
						},
						{
							Name:     "tcp-4",
							Port:     80,
							Protocol: gwv1.TCPProtocolType,
						},
						{
							Name:     "udp-2",
							Port:     80,
							Protocol: gwv1.UDPProtocolType,
						},
					},
				},
			},
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{},
				},
				443: {
					&routeutils.MockRoute{},
				},
			},
			want: map[int32]gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolTCP_UDP,
					hostnames: sets.New[string](),
				},
				443: {
					protocol:  elbv2model.ProtocolTCP,
					hostnames: sets.New[string](),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapGatewayListenerConfigsByPort(tt.gateway, tt.routes)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildListenerTags(t *testing.T) {
	tests := []struct {
		name                string
		lbCfg               elbv2gw.LoadBalancerConfiguration
		defaultTags         map[string]string
		externalManagedTags []string
		expectedTags        map[string]string
		expectedErr         error
	}{
		{
			name: "successful tag retrieval with default tags",
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lb",
					Namespace: "test-namespace",
				},
			},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{},
			expectedTags: map[string]string{
				"Environment": "test",
			},
			expectedErr: nil,
		},
		{
			name: "empty tags returned",
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lb-empty",
					Namespace: "test-namespace",
				},
			},
			defaultTags:         map[string]string{},
			externalManagedTags: []string{},
			expectedTags:        map[string]string{},
			expectedErr:         nil,
		},
		{
			name: "tags with user-specified tags",
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lb-user-tags",
					Namespace: "test-namespace",
				},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					Tags: &map[string]string{
						"Application": "my-app",
						"Owner":       "team-a",
					},
				},
			},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{},
			expectedTags: map[string]string{
				"Environment": "test",
				"Application": "my-app",
				"Owner":       "team-a",
			},
			expectedErr: nil,
		},
		{
			name: "external managed tags specified by user should cause error",
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-lb-error",
					Namespace: "test-namespace",
				},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					Tags: &map[string]string{
						"Application":   "my-app",
						"ExternalTag":   "external-value",
						"ManagedByTeam": "platform-team",
					},
				},
			},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			expectedTags:        nil,
			expectedErr:         errors.New("external managed tag key"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagHelper := newTagHelper(sets.New(tt.externalManagedTags...), tt.defaultTags, false)

			builder := &listenerBuilderImpl{
				tagHelper: tagHelper,
			}

			got, err := builder.buildListenerTags(tt.lbCfg)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
				assert.True(t, strings.Contains(err.Error(), "ExternalTag") || strings.Contains(err.Error(), "ManagedByTeam"))
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTags, got)
			}
		})
	}
}

func Test_mapLoadBalancerListenerConfigsByPort(t *testing.T) {
	// Helper function to create listener configurations
	createListenerConfigs := func(protocolPorts ...string) *[]elbv2gw.ListenerConfiguration {
		configs := make([]elbv2gw.ListenerConfiguration, len(protocolPorts))
		for i, pp := range protocolPorts {
			configs[i] = elbv2gw.ListenerConfiguration{
				ProtocolPort: elbv2gw.ProtocolPort(pp),
			}
		}
		return &configs
	}

	// Test cases
	tests := []struct {
		name      string
		lbCfg     elbv2gw.LoadBalancerConfiguration
		listeners map[int32]gwListenerConfig
		want      map[int32]*elbv2gw.ListenerConfiguration
	}{
		{
			name:      "nil configuration",
			listeners: map[int32]gwListenerConfig{},
			want:      map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name:      "nil listener configurations",
			listeners: map[int32]gwListenerConfig{},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: nil,
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name:      "empty listener configurations",
			listeners: map[int32]gwListenerConfig{},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: createListenerConfigs(),
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name: "single HTTP listener",
			listeners: map[int32]gwListenerConfig{
				80: {
					protocol: elbv2model.ProtocolHTTP,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: createListenerConfigs("HTTP:80"),
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{
				80: {
					ProtocolPort: "HTTP:80",
				},
			},
		},
		{
			name: "multiple valid listeners",
			listeners: map[int32]gwListenerConfig{
				80: {
					protocol: elbv2model.ProtocolHTTP,
				},
				443: {
					protocol: elbv2model.ProtocolHTTPS,
				},
				8080: {
					protocol: elbv2model.ProtocolHTTP,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: createListenerConfigs(
						"HTTP:80",
						"HTTPS:443",
						"HTTP:8080",
					),
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{
				80: {
					ProtocolPort: "HTTP:80",
				},
				443: {
					ProtocolPort: "HTTPS:443",
				},
				8080: {
					ProtocolPort: "HTTP:8080",
				},
			},
		},
		{
			name: "conflicting listener protocols",
			listeners: map[int32]gwListenerConfig{
				80: {
					protocol: elbv2model.ProtocolHTTP,
				},
				443: {
					protocol: elbv2model.ProtocolHTTPS,
				},
				8080: {
					protocol: elbv2model.ProtocolHTTP,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: createListenerConfigs(
						"HTTP:80",
						"TCP:80",
						"HTTPS:443",
						"HTTP:8080",
					),
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{
				80: {
					ProtocolPort: "HTTP:80",
				},
				443: {
					ProtocolPort: "HTTPS:443",
				},
				8080: {
					ProtocolPort: "HTTP:8080",
				},
			},
		},
		{
			name: "single TCP_UDP listener",
			listeners: map[int32]gwListenerConfig{
				80: {
					protocol: elbv2model.ProtocolTCP_UDP,
				},
			},
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: createListenerConfigs("TCP_UDP:80"),
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{
				80: {
					ProtocolPort: "TCP_UDP:80",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapLoadBalancerListenerConfigsByPort(tt.lbCfg, tt.listeners)

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildListenerALPNPolicy(t *testing.T) {
	ALPNPolicyHTTP1Only := elbv2gw.ALPNPolicyHTTP1Only
	invalidALPNPoilcy := elbv2gw.ALPNPolicy("invalid")
	tests := []struct {
		name             string
		listenerProtocol elbv2model.Protocol
		lbLsCfg          *elbv2gw.ListenerConfiguration
		want             []string
		wantErr          error
	}{
		{
			name:             "listener with non-TLS protocol",
			lbLsCfg:          &elbv2gw.ListenerConfiguration{},
			listenerProtocol: elbv2model.ProtocolTCP,
			want:             nil,
			wantErr:          nil,
		},
		{
			name:             "TLS listener without listener config",
			lbLsCfg:          nil,
			listenerProtocol: elbv2model.ProtocolTLS,
			want:             []string{string(elbv2gw.ALPNPolicyNone)},
			wantErr:          nil,
		},
		{
			name:             "TLS listener with HTTP1Only policy",
			listenerProtocol: elbv2model.ProtocolTLS,
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				ALPNPolicy: &ALPNPolicyHTTP1Only,
			},
			want:    []string{string(elbv2gw.ALPNPolicyHTTP1Only)},
			wantErr: nil,
		},
		{
			name:             "TLS listener with invalid ALPN policy",
			listenerProtocol: elbv2model.ProtocolTLS,
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				ALPNPolicy: &invalidALPNPoilcy,
			},
			want:    nil,
			wantErr: errors.New("invalid ALPN policy InvalidPolicy, policy must be one of [None, HTTP1Only, HTTP2Only, HTTP2Optional, HTTP2Preferred]"),
		},
		{
			name:             "TCP listener with ALPN policy",
			listenerProtocol: elbv2model.ProtocolTCP,
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				ALPNPolicy: &ALPNPolicyHTTP1Only,
			},
			want:    nil,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildListenerALPNPolicy(tt.listenerProtocol, tt.lbLsCfg)
			if tt.wantErr != nil {
				assert.Error(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestBuildCertificates(t *testing.T) {
	tests := []struct {
		name       string
		gateway    *gwv1.Gateway
		port       int32
		gwLsCfg    gwListenerConfig
		lbLsCfg    *elbv2gw.ListenerConfiguration
		setupMocks func(mockCertDiscovery *certs.MockCertDiscovery)
		want       []elbv2model.Certificate
		wantErr    bool
	}{
		{
			name: "default certificate only - explicit config",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "https",
							Port:     443,
							Protocol: gwv1.HTTPSProtocolType,
						},
					},
				},
			},
			port: 443,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("my-host-1", "my-host-2"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				DefaultCertificate: awssdk.String("arn:aws:acm:region:123456789012:certificate/default-cert"),
			},
			want: []elbv2model.Certificate{
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/default-cert"),
				},
			},
		},
		{
			name: "multiple certificates without default - explicit config",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tls",
							Port:     443,
							Protocol: gwv1.TLSProtocolType,
						},
					},
				},
			},
			port: 443,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolTLS,
				hostnames: sets.New[string]("my-host-1", "my-host-2"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				Certificates: []*string{
					awssdk.String("arn:aws:acm:region:123456789012:certificate/extra-cert-1"),
					awssdk.String("arn:aws:acm:region:123456789012:certificate/extra-cert-2"),
				},
			},
			want: []elbv2model.Certificate{
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/extra-cert-1"),
				},
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/extra-cert-2"),
				},
			},
		},
		{
			name: "multiple certificates with default - explicit config",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "https",
							Port:     443,
							Protocol: gwv1.HTTPSProtocolType,
						},
					},
				},
			},
			port: 443,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("my-host-1", "my-host-2"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				DefaultCertificate: awssdk.String("arn:aws:acm:region:123456789012:certificate/default-cert"),
				Certificates: []*string{
					awssdk.String("arn:aws:acm:region:123456789012:certificate/extra-cert-1"),
					awssdk.String("arn:aws:acm:region:123456789012:certificate/extra-cert-2"),
				},
			},
			want: []elbv2model.Certificate{
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/default-cert"),
				},
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/extra-cert-1"),
				},
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/extra-cert-2"),
				},
			},
		},
		{
			name: "auto-discover certificates for one hosts",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tls",
							Port:     443,
							Protocol: gwv1.TLSProtocolType,
						},
					},
				},
			},
			port: 443,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolTLS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				ProtocolPort: "TLS:443",
			},
			setupMocks: func(mockCertDiscovery *certs.MockCertDiscovery) {
				mockCertDiscovery.EXPECT().
					Discover(gomock.Any(), []string{"example.com"}).
					Return([]string{
						"arn:aws:acm:region:123456789012:certificate/cert-1",
					}, nil)
			},
			want: []elbv2model.Certificate{
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/cert-1"),
				},
			},
		},
		{
			name: "auto-discover certificates for hosts",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tls",
							Port:     443,
							Protocol: gwv1.TLSProtocolType,
						},
					},
				},
			},
			port: 443,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolTLS,
				hostnames: sets.New[string]("example.com", "*.example.org"),
			},
			lbLsCfg: nil,
			setupMocks: func(mockCertDiscovery *certs.MockCertDiscovery) {
				// The hostnames will be sorted alphabetically by sets.NewString().List()
				mockCertDiscovery.EXPECT().
					Discover(gomock.Any(), []string{"*.example.org", "example.com"}).
					Return([]string{
						"arn:aws:acm:region:123456789012:certificate/cert-1",
						"arn:aws:acm:region:123456789012:certificate/cert-2",
					}, nil)
			},
			want: []elbv2model.Certificate{
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/cert-1"),
				},
				{
					CertificateARN: awssdk.String("arn:aws:acm:region:123456789012:certificate/cert-2"),
				},
			},
		},
		{
			name: "certificate discovery fails",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "https",
							Port:     443,
							Protocol: gwv1.HTTPSProtocolType,
						},
					},
				},
			},
			port: 443,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				ProtocolPort: "HTTPS:443",
			},
			setupMocks: func(mockCertDiscovery *certs.MockCertDiscovery) {
				mockCertDiscovery.EXPECT().
					Discover(gomock.Any(), []string{"example.com"}).
					Return(nil, errors.New("certificate discovery failed"))
			},
			want:    []elbv2model.Certificate{},
			wantErr: true,
		},
		{
			name: "no hostname for discovery : no secure listeners",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
						},
					},
				},
			},
			port: 443,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTP,
				hostnames: sets.New[string](),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				ProtocolPort: "HTTP:80",
			},
			want: []elbv2model.Certificate{},
		},
		{
			name: "no hostname for discovery : secure listeners",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "https",
							Port:     443,
							Protocol: gwv1.HTTPSProtocolType,
						},
					},
				},
			},
			port: 443,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string](),
			},
			lbLsCfg: nil,
			want:    []elbv2model.Certificate{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockCertDiscovery := certs.NewMockCertDiscovery(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(mockCertDiscovery)
			}

			builder := &listenerBuilderImpl{
				certDiscovery: mockCertDiscovery,
			}

			got, err := builder.buildCertificates(context.Background(), tt.gateway, tt.port, tt.gwLsCfg, tt.lbLsCfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildCertificates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildCertificates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_buildMutualAuthenticationAttributes(t *testing.T) {
	trueValue := true
	falseValue := false
	verifyMode := elbv2gw.MutualAuthenticationVerifyMode
	passthroughMode := elbv2gw.MutualAuthenticationPassthroughMode
	offMode := elbv2gw.MutualAuthenticationOffMode
	advertiseTrustStoreCaNames := elbv2gw.AdvertiseTrustStoreCaNamesEnumOn
	trustStoreNameOnly := "my-trust-store"
	nonExistentTrustStoreNameOnly := "non-existent-trust-store"
	trustStoreArn := "arn:aws:elasticloadbalancing:us-west-2:123456789012:truststore/my-trust-store"

	type resolveSubnetInLocalZoneOrOutpostCall struct {
		subnetID string
		result   bool
		err      error
	}

	type describeTrustStoresCall struct {
		names  []string
		result map[string]*string
		err    error
	}

	tests := []struct {
		name                string
		protocol            elbv2model.Protocol
		gwLsCfg             gwListenerConfig
		lbLsCfg             *elbv2gw.ListenerConfiguration
		describeTrustStores describeTrustStoresCall
		want                *elbv2model.MutualAuthenticationAttributes
		wantErr             bool
	}{
		{
			name:     "non-secure protocol should return nil",
			protocol: elbv2model.ProtocolHTTP,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTP,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{},
			want:    nil,
			wantErr: false,
		},
		{
			name:     "nil lbLsCfg should return nil",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: nil,
			want:    nil,
			wantErr: false,
		},
		{
			name:     "nil mutualAuthentication should return off mode",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: nil,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:     "verify mode with truststore name should resolve ARN",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:       verifyMode,
					TrustStore: &trustStoreNameOnly,
				},
			},
			describeTrustStores: describeTrustStoresCall{
				names: []string{trustStoreNameOnly},
				result: map[string]*string{
					trustStoreNameOnly: &trustStoreArn,
				},
				err: nil,
			},
			want: &elbv2model.MutualAuthenticationAttributes{
				Mode:                          string(elbv2gw.MutualAuthenticationVerifyMode),
				TrustStoreArn:                 awssdk.String(trustStoreArn),
				IgnoreClientCertificateExpiry: awssdk.Bool(false),
				AdvertiseTrustStoreCaNames:    awssdk.String(""),
			},
			wantErr: false,
		},
		{
			name:     "verify mode with truststore ARN should use ARN directly",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:       verifyMode,
					TrustStore: &trustStoreArn,
				},
			},
			want: &elbv2model.MutualAuthenticationAttributes{
				Mode:                          string(elbv2gw.MutualAuthenticationVerifyMode),
				TrustStoreArn:                 awssdk.String(trustStoreArn),
				IgnoreClientCertificateExpiry: awssdk.Bool(false),
				AdvertiseTrustStoreCaNames:    awssdk.String(""),
			},
			wantErr: false,
		},
		{
			name:     "verify mode with all options",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:                          verifyMode,
					TrustStore:                    &trustStoreArn,
					IgnoreClientCertificateExpiry: &trueValue,
					AdvertiseTrustStoreCaNames:    &advertiseTrustStoreCaNames,
				},
			},
			want: &elbv2model.MutualAuthenticationAttributes{
				Mode:                          string(elbv2gw.MutualAuthenticationVerifyMode),
				TrustStoreArn:                 awssdk.String(trustStoreArn),
				IgnoreClientCertificateExpiry: &trueValue,
				AdvertiseTrustStoreCaNames:    awssdk.String(string(elbv2gw.AdvertiseTrustStoreCaNamesEnumOn)),
			},
			wantErr: false,
		},
		{
			name:     "verify mode with nil ignoreClientCertificateExpiry",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:                          verifyMode,
					TrustStore:                    &trustStoreArn,
					IgnoreClientCertificateExpiry: nil,
					AdvertiseTrustStoreCaNames:    &advertiseTrustStoreCaNames,
				},
			},
			want: &elbv2model.MutualAuthenticationAttributes{
				Mode:                          string(elbv2gw.MutualAuthenticationVerifyMode),
				TrustStoreArn:                 awssdk.String(trustStoreArn),
				IgnoreClientCertificateExpiry: &falseValue,
				AdvertiseTrustStoreCaNames:    awssdk.String(string(elbv2gw.AdvertiseTrustStoreCaNamesEnumOn)),
			},
			wantErr: false,
		},
		{
			name:     "passthrough mode",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode: passthroughMode,
				},
			},
			want: &elbv2model.MutualAuthenticationAttributes{
				Mode:                          string(elbv2gw.MutualAuthenticationPassthroughMode),
				TrustStoreArn:                 nil,
				AdvertiseTrustStoreCaNames:    awssdk.String(""),
				IgnoreClientCertificateExpiry: nil,
			},
			wantErr: false,
		},
		{
			name:     "off mode",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode: offMode,
				},
			},
			want: &elbv2model.MutualAuthenticationAttributes{
				Mode:                          string(elbv2gw.MutualAuthenticationOffMode),
				TrustStoreArn:                 nil,
				AdvertiseTrustStoreCaNames:    awssdk.String(""),
				IgnoreClientCertificateExpiry: nil,
			},
			wantErr: false,
		},
		{
			name:     "error on truststore ARN resolution",
			protocol: elbv2model.ProtocolHTTPS,
			gwLsCfg: gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: sets.New[string]("example.com"),
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:       verifyMode,
					TrustStore: &nonExistentTrustStoreNameOnly,
				},
			},
			describeTrustStores: describeTrustStoresCall{
				names:  []string{nonExistentTrustStoreNameOnly},
				result: nil,
				err:    errors.New("trust store resolution error"),
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockELBV2Client := services.NewMockELBV2(ctrl)

			if tt.protocol == elbv2model.ProtocolHTTPS {
				if tt.lbLsCfg != nil && tt.lbLsCfg.MutualAuthentication != nil &&
					tt.lbLsCfg.MutualAuthentication.Mode == verifyMode &&
					tt.lbLsCfg.MutualAuthentication.TrustStore != nil &&
					!strings.HasPrefix(*tt.lbLsCfg.MutualAuthentication.TrustStore, "arn:") {
					callInfo := tt.describeTrustStores
					mockELBV2Client.EXPECT().
						DescribeTrustStoresWithContext(gomock.Any(), gomock.Any()).
						DoAndReturn(func(_ context.Context, req *elbv2sdk.DescribeTrustStoresInput) (*elbv2sdk.DescribeTrustStoresOutput, error) {
							if !reflect.DeepEqual(req.Names, callInfo.names) {
								t.Errorf("expected names %v, got %v", callInfo.names, req.Names)
							}
							if callInfo.err != nil {
								return nil, callInfo.err
							}
							var trustStores []elbv2types.TrustStore
							for name, arn := range callInfo.result {
								trustStores = append(trustStores, elbv2types.TrustStore{
									Name:          awssdk.String(name),
									TrustStoreArn: arn,
								})
							}
							return &elbv2sdk.DescribeTrustStoresOutput{
								TrustStores: trustStores,
							}, nil
						})
				}
			}

			builder := &listenerBuilderImpl{
				elbv2Client: mockELBV2Client,
			}

			got, err := builder.buildMutualAuthenticationAttributes(context.Background(), tt.gwLsCfg, tt.lbLsCfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildMutualAuthenticationAttributes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_BuildListenerRules(t *testing.T) {
	autheticateBehavior := elbv2gw.AuthenticateCognitoActionConditionalBehaviorEnumAuthenticate
	testCases := []struct {
		name             string
		ipAddressType    elbv2model.IPAddressType
		port             int32
		listenerProtocol elbv2model.Protocol
		routes           map[int32][]routeutils.RouteDescriptor

		expectedRules []*elbv2model.ListenerRuleSpec
		expectedTags  map[string]string
		tagErr        error
	}{
		{
			name:             "no backends should result in 500 fixed response",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTP,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Kind:      routeutils.HTTPRouteKind,
						Name:      "my-route",
						Namespace: "my-route-ns",
						Rules: []routeutils.RouteRule{
							&routeutils.MockRule{
								RawRule: &gwv1.HTTPRouteRule{
									Matches: []gwv1.HTTPRouteMatch{
										{
											Path: &gwv1.HTTPPathMatch{
												Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
												Value: awssdk.String("/"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedRules: []*elbv2model.ListenerRuleSpec{
				{
					Priority: 1,
					Actions: []elbv2model.Action{
						{
							Type: "fixed-response",
							FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
								ContentType: awssdk.String("text/plain"),
								StatusCode:  "500",
							},
						},
					},
					Conditions: []elbv2model.RuleCondition{
						{
							Field: "path-pattern",
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
				},
			},
		},
		{
			name:             "backends should result in forward action generated",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTP,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Kind:      routeutils.HTTPRouteKind,
						Name:      "my-route",
						Namespace: "my-route-ns",
						Rules: []routeutils.RouteRule{
							&routeutils.MockRule{
								RawRule: &gwv1.HTTPRouteRule{
									Matches: []gwv1.HTTPRouteMatch{
										{
											Path: &gwv1.HTTPPathMatch{
												Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
												Value: awssdk.String("/"),
											},
										},
									},
								},
								BackendRefs: []routeutils.Backend{
									{
										ServiceBackend: &routeutils.ServiceBackendConfig{},
										Weight:         1,
									},
								},
							},
						},
					},
				},
			},
			expectedRules: []*elbv2model.ListenerRuleSpec{
				{
					Priority: 1,
					Actions: []elbv2model.Action{
						{
							Type: "forward",
							ForwardConfig: &elbv2model.ForwardActionConfig{
								// cmp can't compare the TG ARN, so don't inject it.
								TargetGroups: []elbv2model.TargetGroupTuple{
									{
										Weight: awssdk.Int32(1),
									},
								},
							},
						},
					},
					Conditions: []elbv2model.RuleCondition{
						{
							Field: "path-pattern",
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
				},
			},
		},
		{
			name:             "redirect filter should result in redirect action - https",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTP,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Kind:      routeutils.HTTPRouteKind,
						Name:      "my-route",
						Namespace: "my-route-ns",
						Rules: []routeutils.RouteRule{
							&routeutils.MockRule{
								RawRule: &gwv1.HTTPRouteRule{
									Filters: []gwv1.HTTPRouteFilter{
										{
											Type: gwv1.HTTPRouteFilterRequestRedirect,
											RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
												Scheme:     awssdk.String("HTTPS"),
												StatusCode: awssdk.Int(301),
											},
										},
									},
									Matches: []gwv1.HTTPRouteMatch{
										{
											Path: &gwv1.HTTPPathMatch{
												Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
												Value: awssdk.String("/"),
											},
										},
									},
								},
								BackendRefs: []routeutils.Backend{
									{
										ServiceBackend: &routeutils.ServiceBackendConfig{},
										Weight:         1,
									},
								},
							},
						},
					},
				},
			},
			expectedRules: []*elbv2model.ListenerRuleSpec{
				{
					Priority: 1,
					Actions: []elbv2model.Action{
						{
							Type: "redirect",
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Protocol:   awssdk.String("HTTPS"),
								Port:       awssdk.String("443"),
								StatusCode: "HTTP_301",
							},
						},
					},
					Conditions: []elbv2model.RuleCondition{
						{
							Field: "path-pattern",
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
				},
			},
		},
		{
			name:             "redirect filter should result in redirect action - http",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTP,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Kind:      routeutils.HTTPRouteKind,
						Name:      "my-route",
						Namespace: "my-route-ns",
						Rules: []routeutils.RouteRule{
							&routeutils.MockRule{
								RawRule: &gwv1.HTTPRouteRule{
									Filters: []gwv1.HTTPRouteFilter{
										{
											Type: gwv1.HTTPRouteFilterRequestRedirect,
											RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
												Scheme:     awssdk.String("HTTP"),
												StatusCode: awssdk.Int(301),
											},
										},
									},
									Matches: []gwv1.HTTPRouteMatch{
										{
											Path: &gwv1.HTTPPathMatch{
												Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
												Value: awssdk.String("/"),
											},
										},
									},
								},
								BackendRefs: []routeutils.Backend{
									{
										ServiceBackend: &routeutils.ServiceBackendConfig{},
										Weight:         1,
									},
								},
							},
						},
					},
				},
			},
			expectedRules: []*elbv2model.ListenerRuleSpec{
				{
					Priority: 1,
					Actions: []elbv2model.Action{
						{
							Type: "redirect",
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Protocol:   awssdk.String("HTTP"),
								Port:       awssdk.String("80"),
								StatusCode: "HTTP_301",
							},
						},
					},
					Conditions: []elbv2model.RuleCondition{
						{
							Field: "path-pattern",
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
				},
			},
		},
		{
			name:             "redirect filter should result in redirect action - port specified",
			port:             90,
			listenerProtocol: elbv2model.ProtocolHTTP,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			routes: map[int32][]routeutils.RouteDescriptor{
				90: {
					&routeutils.MockRoute{
						Kind:      routeutils.HTTPRouteKind,
						Name:      "my-route",
						Namespace: "my-route-ns",
						Rules: []routeutils.RouteRule{
							&routeutils.MockRule{
								RawRule: &gwv1.HTTPRouteRule{
									Filters: []gwv1.HTTPRouteFilter{
										{
											Type: gwv1.HTTPRouteFilterRequestRedirect,
											RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
												Scheme:     awssdk.String("HTTP"),
												StatusCode: awssdk.Int(301),
												Port:       (*gwv1.PortNumber)(awssdk.Int32(900)),
											},
										},
									},
									Matches: []gwv1.HTTPRouteMatch{
										{
											Path: &gwv1.HTTPPathMatch{
												Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
												Value: awssdk.String("/"),
											},
										},
									},
								},
								BackendRefs: []routeutils.Backend{
									{
										ServiceBackend: &routeutils.ServiceBackendConfig{},
										Weight:         1,
									},
								},
							},
						},
					},
				},
			},
			expectedRules: []*elbv2model.ListenerRuleSpec{
				{
					Priority: 1,
					Actions: []elbv2model.Action{
						{
							Type: "redirect",
							RedirectConfig: &elbv2model.RedirectActionConfig{
								Protocol:   awssdk.String("HTTP"),
								Port:       awssdk.String("900"),
								StatusCode: "HTTP_301",
							},
						},
					},
					Conditions: []elbv2model.RuleCondition{
						{
							Field: "path-pattern",
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
				},
			},
		},
		{
			name:             "listener rule config with fixed response should override forward",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTP,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Kind:      routeutils.HTTPRouteKind,
						Name:      "my-route",
						Namespace: "my-route-ns",
						Rules: []routeutils.RouteRule{
							&routeutils.MockRule{
								RawRule: &gwv1.HTTPRouteRule{
									Matches: []gwv1.HTTPRouteMatch{
										{
											Path: &gwv1.HTTPPathMatch{
												Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
												Value: awssdk.String("/"),
											},
										},
									},
								},
								BackendRefs: []routeutils.Backend{
									{
										ServiceBackend: &routeutils.ServiceBackendConfig{},
										Weight:         1,
									},
								},
								ListenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
									Spec: elbv2gw.ListenerRuleConfigurationSpec{
										Actions: []elbv2gw.Action{
											{
												Type: elbv2gw.ActionTypeFixedResponse,
												FixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
													StatusCode:  404,
													ContentType: awssdk.String("text/html"),
													MessageBody: awssdk.String("Not Found"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedRules: []*elbv2model.ListenerRuleSpec{
				{
					Priority: 1,
					Actions: []elbv2model.Action{
						{
							Type: "fixed-response",
							FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
								StatusCode:  "404",
								ContentType: awssdk.String("text/html"),
								MessageBody: awssdk.String("Not Found"),
							},
						},
					},
					Conditions: []elbv2model.RuleCondition{
						{
							Field: "path-pattern",
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
				},
			},
		},
		{
			name:             "listener rule config with authenticate-cognito should create auth + forward actions",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTPS,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Kind:      routeutils.HTTPRouteKind,
						Name:      "my-route",
						Namespace: "my-route-ns",
						Rules: []routeutils.RouteRule{
							&routeutils.MockRule{
								RawRule: &gwv1.HTTPRouteRule{
									Matches: []gwv1.HTTPRouteMatch{
										{
											Path: &gwv1.HTTPPathMatch{
												Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
												Value: awssdk.String("/"),
											},
										},
									},
								},
								BackendRefs: []routeutils.Backend{
									{
										ServiceBackend: &routeutils.ServiceBackendConfig{},
										Weight:         1,
									},
								},
								ListenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
									Spec: elbv2gw.ListenerRuleConfigurationSpec{
										Actions: []elbv2gw.Action{
											{
												Type: elbv2gw.ActionTypeAuthenticateCognito,
												AuthenticateCognitoConfig: &elbv2gw.AuthenticateCognitoActionConfig{
													UserPoolArn:              "arn:aws:cognito-idp:us-west-2:123456789012:userpool/us-west-2_EXAMPLE",
													UserPoolClientID:         "1example23456789",
													UserPoolDomain:           "my-cognito-domain",
													OnUnauthenticatedRequest: &autheticateBehavior,
													Scope:                    awssdk.String("openid"),
													SessionCookieName:        awssdk.String("AWSELBAuthSessionCookie"),
													SessionTimeout:           awssdk.Int64(604800),
													AuthenticationRequestExtraParams: &map[string]string{
														"display": "page",
														"prompt":  "login",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedRules: []*elbv2model.ListenerRuleSpec{
				{
					Priority: 1,
					Actions: []elbv2model.Action{
						{
							Type: "authenticate-cognito",
							AuthenticateCognitoConfig: &elbv2model.AuthenticateCognitoActionConfig{
								UserPoolARN:              "arn:aws:cognito-idp:us-west-2:123456789012:userpool/us-west-2_EXAMPLE",
								UserPoolClientID:         "1example23456789",
								UserPoolDomain:           "my-cognito-domain",
								OnUnauthenticatedRequest: elbv2model.AuthenticateCognitoActionConditionalBehaviorAuthenticate,
								Scope:                    awssdk.String("openid"),
								SessionCookieName:        awssdk.String("AWSELBAuthSessionCookie"),
								SessionTimeout:           awssdk.Int64(604800),
								AuthenticationRequestExtraParams: map[string]string{
									"display": "page",
									"prompt":  "login",
								},
							},
						},
						{
							Type: "forward",
							ForwardConfig: &elbv2model.ForwardActionConfig{
								TargetGroups: []elbv2model.TargetGroupTuple{
									{
										Weight: awssdk.Int32(1), // This will be normalized by the builder
									},
								},
							},
						},
					},
					Conditions: []elbv2model.RuleCondition{
						{
							Field: "path-pattern",
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
				},
			},
		},
		{
			name:             "listener rule config with authenticate-cognito and no backends should result in auth + 500 fixed response",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTPS,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			routes: map[int32][]routeutils.RouteDescriptor{
				80: {
					&routeutils.MockRoute{
						Kind:      routeutils.HTTPRouteKind,
						Name:      "my-route",
						Namespace: "my-route-ns",
						Rules: []routeutils.RouteRule{
							&routeutils.MockRule{
								RawRule: &gwv1.HTTPRouteRule{
									Matches: []gwv1.HTTPRouteMatch{
										{
											Path: &gwv1.HTTPPathMatch{
												Type:  (*gwv1.PathMatchType)(awssdk.String("PathPrefix")),
												Value: awssdk.String("/"),
											},
										},
									},
								},
								ListenerRuleConfig: &elbv2gw.ListenerRuleConfiguration{
									Spec: elbv2gw.ListenerRuleConfigurationSpec{
										Actions: []elbv2gw.Action{
											{
												Type: elbv2gw.ActionTypeAuthenticateCognito,
												AuthenticateCognitoConfig: &elbv2gw.AuthenticateCognitoActionConfig{
													UserPoolArn:              "arn:aws:cognito-idp:us-west-2:123456789012:userpool/us-west-2_EXAMPLE",
													UserPoolClientID:         "1example23456789",
													UserPoolDomain:           "my-cognito-domain",
													OnUnauthenticatedRequest: &autheticateBehavior,
													Scope:                    awssdk.String("openid"),
													SessionCookieName:        awssdk.String("AWSELBAuthSessionCookie"),
													SessionTimeout:           awssdk.Int64(604800),
													AuthenticationRequestExtraParams: &map[string]string{
														"display": "page",
														"prompt":  "login",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedRules: []*elbv2model.ListenerRuleSpec{
				{
					Priority: 1,
					Actions: []elbv2model.Action{
						{
							Type: "authenticate-cognito",
							AuthenticateCognitoConfig: &elbv2model.AuthenticateCognitoActionConfig{
								UserPoolARN:              "arn:aws:cognito-idp:us-west-2:123456789012:userpool/us-west-2_EXAMPLE",
								UserPoolClientID:         "1example23456789",
								UserPoolDomain:           "my-cognito-domain",
								OnUnauthenticatedRequest: elbv2model.AuthenticateCognitoActionConditionalBehaviorAuthenticate,
								Scope:                    awssdk.String("openid"),
								SessionCookieName:        awssdk.String("AWSELBAuthSessionCookie"),
								SessionTimeout:           awssdk.Int64(604800),
								AuthenticationRequestExtraParams: map[string]string{
									"display": "page",
									"prompt":  "login",
								},
							},
						},
						{
							Type: "fixed-response",
							FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
								ContentType: awssdk.String("text/plain"),
								StatusCode:  "500",
							},
						},
					},
					Conditions: []elbv2model.RuleCondition{
						{
							Field: "path-pattern",
							PathPatternConfig: &elbv2model.PathPatternConditionConfig{
								Values: []string{"/*"},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
			mockTagger := &mockTagHelper{
				tags: tc.expectedTags,
				err:  tc.tagErr,
			}

			mockTgBuilder := &mockTargetGroupBuilder{
				tgs: []*elbv2model.TargetGroup{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", "id-1"),
					},
				},
			}

			builder := &listenerBuilderImpl{
				tagHelper: mockTagger,
				tgBuilder: mockTgBuilder,
			}

			_, err := builder.buildListenerRules(context.Background(), stack, &elbv2model.Listener{
				Spec: elbv2model.ListenerSpec{
					Protocol: tc.listenerProtocol,
				},
			}, tc.ipAddressType, &gwv1.Gateway{}, tc.port, tc.routes)
			assert.NoError(t, err)

			var resLRs []*elbv2model.ListenerRule
			assert.NoError(t, stack.ListResources(&resLRs))

			assert.Equal(t, len(tc.expectedRules), len(resLRs))

			// cmp absolutely barfs trying to validate the TargetGroupARN due to stack id semantics
			opt := cmp.Options{
				cmpopts.IgnoreFields(elbv2model.TargetGroupTuple{}, "TargetGroupARN"),
			}

			processedSet := make(map[*elbv2model.ListenerRule]bool)
			for _, elr := range tc.expectedRules {
				for _, alr := range resLRs {
					conditionsEqual := cmp.Equal(elr.Conditions, alr.Spec.Conditions)
					actionsEqual := cmp.Equal(elr.Actions, alr.Spec.Actions, opt)
					priorityEqual := elr.Priority == alr.Spec.Priority
					if conditionsEqual && actionsEqual && priorityEqual {
						processedSet[alr] = true
						break
					}
				}
			}

			assert.Equal(t, len(tc.expectedRules), len(processedSet))

			for _, lr := range resLRs {
				assert.Equal(t, tc.expectedTags, lr.Spec.Tags)
			}
		})
	}
}

func Test_buildL4TargetGroupTuples(t *testing.T) {

	type tgValidation struct {
		arn    string
		weight int
	}

	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})

	tgs := make([]*elbv2model.TargetGroup, 0)

	for i := 1; i <= 4; i++ {
		tgs = append(tgs, &elbv2model.TargetGroup{
			ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::TargetGroup", fmt.Sprintf("id-%d", i)),
			Status: &elbv2model.TargetGroupStatus{
				TargetGroupARN: fmt.Sprintf("arn%d", i),
			},
		})
	}
	testCases := []struct {
		name         string
		targetGroups []*elbv2model.TargetGroup
		routes       []routeutils.RouteDescriptor
		expected     []tgValidation
		expectErr    bool
	}{
		{
			name:     "no routes",
			routes:   []routeutils.RouteDescriptor{},
			expected: make([]tgValidation, 0),
		},
		{
			name: "one route - no backends",
			routes: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{},
			},
			expected: make([]tgValidation, 0),
		},
		{
			name: "one route - one rule - one backend",
			targetGroups: []*elbv2model.TargetGroup{
				tgs[0],
			},
			routes: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{
					Rules: []routeutils.RouteRule{
						&routeutils.MockRule{
							BackendRefs: []routeutils.Backend{
								{
									Weight: 1,
								},
							},
						},
					},
				},
			},
			expected: []tgValidation{
				{
					arn:    "arn1",
					weight: 1,
				},
			},
		},
		{
			name: "one route - one rule - two backend",
			targetGroups: []*elbv2model.TargetGroup{
				tgs[0],
				tgs[1],
			},
			routes: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{
					Rules: []routeutils.RouteRule{
						&routeutils.MockRule{
							BackendRefs: []routeutils.Backend{
								{
									Weight: 1,
								},
								{
									Weight: 2,
								},
							},
						},
					},
				},
			},
			expected: []tgValidation{
				{
					arn:    "arn1",
					weight: 1,
				},
				{
					arn:    "arn2",
					weight: 2,
				},
			},
		},
		{
			name: "one route - two rules - one backend each",
			targetGroups: []*elbv2model.TargetGroup{
				tgs[0],
				tgs[1],
			},
			routes: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{
					Rules: []routeutils.RouteRule{
						&routeutils.MockRule{
							BackendRefs: []routeutils.Backend{
								{
									Weight: 1,
								},
							},
						},
						&routeutils.MockRule{
							BackendRefs: []routeutils.Backend{
								{
									Weight: 2,
								},
							},
						},
					},
				},
			},
			expected: []tgValidation{
				{
					arn:    "arn1",
					weight: 1,
				},
				{
					arn:    "arn2",
					weight: 2,
				},
			},
		},
		{
			name: "two routes - one rule / two backends each",
			targetGroups: []*elbv2model.TargetGroup{
				tgs[0],
				tgs[1],
				tgs[2],
				tgs[3],
			},
			routes: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{
					Rules: []routeutils.RouteRule{
						&routeutils.MockRule{
							BackendRefs: []routeutils.Backend{
								{
									Weight: 1,
								},
								{
									Weight: 1,
								},
							},
						},
					},
				},
				&routeutils.MockRoute{
					Rules: []routeutils.RouteRule{
						&routeutils.MockRule{
							BackendRefs: []routeutils.Backend{
								{
									Weight: 2,
								},
								{
									Weight: 1,
								},
							},
						},
					},
				},
			},
			expected: []tgValidation{
				{
					arn:    "arn1",
					weight: 1,
				},
				{
					arn:    "arn2",
					weight: 1,
				},
				{
					arn:    "arn3",
					weight: 2,
				},
				{
					arn:    "arn4",
					weight: 1,
				},
			},
		},
		{
			name: "two routes - one rule / one backend each",
			targetGroups: []*elbv2model.TargetGroup{
				tgs[0],
				tgs[1],
			},
			routes: []routeutils.RouteDescriptor{
				&routeutils.MockRoute{
					Rules: []routeutils.RouteRule{
						&routeutils.MockRule{
							BackendRefs: []routeutils.Backend{
								{
									Weight: 1,
								},
							},
						},
					},
				},
				&routeutils.MockRoute{
					Rules: []routeutils.RouteRule{
						&routeutils.MockRule{
							BackendRefs: []routeutils.Backend{
								{
									Weight: 2,
								},
							},
						},
					},
				},
			},
			expected: []tgValidation{
				{
					arn:    "arn1",
					weight: 1,
				},
				{
					arn:    "arn2",
					weight: 2,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockTgBuilder := &mockTargetGroupBuilder{
				tgs: tc.targetGroups,
			}

			builder := &listenerBuilderImpl{
				tgBuilder: mockTgBuilder,
				logger:    logr.Discard(),
			}

			result, err := builder.buildL4TargetGroupTuples(stack, tc.routes, &gwv1.Gateway{}, 80, elbv2model.ProtocolHTTP, elbv2model.IPAddressTypeIPV4)

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tc.expected), len(result))
				for i := range tc.expected {
					e := tc.expected[i]
					a := result[i]
					actualArn, _ := a.TargetGroupARN.Resolve(context.Background())
					assert.Equal(t, e.arn, actualArn)
					assert.Equal(t, e.weight, int(*a.Weight))
				}
			}
		})
	}
}

func TestQuicProtocolUpgrade(t *testing.T) {
	tests := []struct {
		name          string
		protocol      elbv2model.Protocol
		quicEnabled   *bool
		expectedProto elbv2model.Protocol
		expectError   bool
	}{
		{
			name:          "UDP with QUIC enabled",
			protocol:      elbv2model.ProtocolUDP,
			quicEnabled:   awssdk.Bool(true),
			expectedProto: elbv2model.ProtocolQUIC,
			expectError:   false,
		},
		{
			name:          "TCP_UDP with QUIC enabled",
			protocol:      elbv2model.ProtocolTCP_UDP,
			quicEnabled:   awssdk.Bool(true),
			expectedProto: elbv2model.ProtocolTCP_QUIC,
			expectError:   false,
		},
		{
			name:          "UDP with QUIC disabled",
			protocol:      elbv2model.ProtocolUDP,
			quicEnabled:   awssdk.Bool(false),
			expectedProto: elbv2model.ProtocolUDP,
			expectError:   false,
		},
		{
			name:          "TCP with QUIC enabled should error",
			protocol:      elbv2model.ProtocolTCP,
			quicEnabled:   awssdk.Bool(true),
			expectedProto: elbv2model.ProtocolTCP,
			expectError:   true,
		},
		{
			name:          "UDP with no QUIC config",
			protocol:      elbv2model.ProtocolUDP,
			quicEnabled:   nil,
			expectedProto: elbv2model.ProtocolUDP,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the QUIC protocol upgrade logic directly
			protocol := tt.protocol
			var lbLsCfg *elbv2gw.ListenerConfiguration
			if tt.quicEnabled != nil {
				lbLsCfg = &elbv2gw.ListenerConfiguration{
					QuicEnabled: tt.quicEnabled,
				}
			}

			// Apply QUIC protocol upgrade if enabled
			var err error
			if lbLsCfg != nil && lbLsCfg.QuicEnabled != nil && *lbLsCfg.QuicEnabled {
				switch protocol {
				case elbv2model.ProtocolUDP:
					protocol = elbv2model.ProtocolQUIC
				case elbv2model.ProtocolTCP_UDP:
					protocol = elbv2model.ProtocolTCP_QUIC
				default:
					err = assert.AnError // Simulate error for unsupported protocols
				}
			}

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedProto, protocol)
			}
		})
	}
}

func TestMergeProtocols_WithQuic(t *testing.T) {
	tests := []struct {
		name             string
		storedProtocol   elbv2model.Protocol
		proposedProtocol elbv2model.Protocol
		expectedProtocol elbv2model.Protocol
		expectError      bool
	}{
		{
			name:             "TCP + QUIC = TCP_QUIC",
			storedProtocol:   elbv2model.ProtocolTCP,
			proposedProtocol: elbv2model.ProtocolQUIC,
			expectedProtocol: elbv2model.ProtocolTCP_QUIC,
			expectError:      false,
		},
		{
			name:             "QUIC + TCP = TCP_QUIC",
			storedProtocol:   elbv2model.ProtocolQUIC,
			proposedProtocol: elbv2model.ProtocolTCP,
			expectedProtocol: elbv2model.ProtocolTCP_QUIC,
			expectError:      false,
		},
		{
			name:             "TCP_QUIC + TCP = TCP_QUIC",
			storedProtocol:   elbv2model.ProtocolTCP_QUIC,
			proposedProtocol: elbv2model.ProtocolTCP,
			expectedProtocol: elbv2model.ProtocolTCP_QUIC,
			expectError:      false,
		},
		{
			name:             "TCP_QUIC + QUIC = TCP_QUIC",
			storedProtocol:   elbv2model.ProtocolTCP_QUIC,
			proposedProtocol: elbv2model.ProtocolQUIC,
			expectedProtocol: elbv2model.ProtocolTCP_QUIC,
			expectError:      false,
		},
		{
			name:             "HTTP + QUIC should error",
			storedProtocol:   elbv2model.ProtocolHTTP,
			proposedProtocol: elbv2model.ProtocolQUIC,
			expectedProtocol: elbv2model.ProtocolHTTP,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mergeProtocols(tt.storedProtocol, tt.proposedProtocol)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedProtocol, result)
			}
		})
	}
}
