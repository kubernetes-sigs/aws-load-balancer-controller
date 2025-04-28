package model

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_mapGatewayListenerConfigsByPort(t *testing.T) {
	fooHostname := gwv1.Hostname("foo.example.com")
	barHostname := gwv1.Hostname("bar.example.com")
	tests := []struct {
		name    string
		gateway *gwv1.Gateway
		want    map[int32]*gwListenerConfig
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
			want: map[int32]*gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: []string{},
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
			want: map[int32]*gwListenerConfig{
				443: {
					protocol:  elbv2model.ProtocolTCP,
					hostnames: []string{},
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
			want: map[int32]*gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: []string{},
				},
				443: {
					protocol:  elbv2model.ProtocolHTTPS,
					hostnames: []string{},
				},
				8080: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: []string{},
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
			want: map[int32]*gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: []string{"foo.example.com"},
				},
				443: {
					protocol:  elbv2model.ProtocolHTTPS,
					hostnames: []string{"foo.example.com"},
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
			want: map[int32]*gwListenerConfig{
				80: {
					protocol:  elbv2model.ProtocolHTTP,
					hostnames: []string{"foo.example.com", "bar.example.com"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapGatewayListenerConfigsByPort(tt.gateway)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.Equal(t, tt.want, got)
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
		name  string
		lbCfg *elbv2gw.LoadBalancerConfiguration
		want  map[int32]*elbv2gw.ListenerConfiguration
	}{
		{
			name:  "nil configuration",
			lbCfg: nil,
			want:  map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name: "nil listener configurations",
			lbCfg: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: nil,
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name: "empty listener configurations",
			lbCfg: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: createListenerConfigs(),
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name: "single HTTP listener",
			lbCfg: &elbv2gw.LoadBalancerConfiguration{
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
			lbCfg: &elbv2gw.LoadBalancerConfiguration{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapLoadBalancerListenerConfigsByPort(tt.lbCfg)

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
			want:             nil,
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
