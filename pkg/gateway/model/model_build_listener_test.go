package model

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	"reflect"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
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
		lbCfg elbv2gw.LoadBalancerConfiguration
		want  map[int32]*elbv2gw.ListenerConfiguration
	}{
		{
			name: "nil configuration",
			want: map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name: "nil listener configurations",
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: nil,
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name: "empty listener configurations",
			lbCfg: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ListenerConfigurations: createListenerConfigs(),
				},
			},
			want: map[int32]*elbv2gw.ListenerConfiguration{},
		},
		{
			name: "single HTTP listener",
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
		gwLsCfg    *gwListenerConfig
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
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"my-host-1", "my-host-2"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				DefaultCertificate: aws.String("arn:aws:acm:region:123456789012:certificate/default-cert"),
			},
			want: []elbv2model.Certificate{
				{
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/default-cert"),
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
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolTLS,
				hostnames: []string{"my-host-1", "my-host-2"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				Certificates: []*string{
					aws.String("arn:aws:acm:region:123456789012:certificate/extra-cert-1"),
					aws.String("arn:aws:acm:region:123456789012:certificate/extra-cert-2"),
				},
			},
			want: []elbv2model.Certificate{
				{
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/extra-cert-1"),
				},
				{
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/extra-cert-2"),
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
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"my-host-1", "my-host-2"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				DefaultCertificate: aws.String("arn:aws:acm:region:123456789012:certificate/default-cert"),
				Certificates: []*string{
					aws.String("arn:aws:acm:region:123456789012:certificate/extra-cert-1"),
					aws.String("arn:aws:acm:region:123456789012:certificate/extra-cert-2"),
				},
			},
			want: []elbv2model.Certificate{
				{
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/default-cert"),
				},
				{
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/extra-cert-1"),
				},
				{
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/extra-cert-2"),
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
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolTLS,
				hostnames: []string{"example.com"},
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
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/cert-1"),
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
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolTLS,
				hostnames: []string{"example.com", "*.example.org"},
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
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/cert-1"),
				},
				{
					CertificateARN: aws.String("arn:aws:acm:region:123456789012:certificate/cert-2"),
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
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
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
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTP,
				hostnames: []string{},
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
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{},
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
		name                              string
		protocol                          elbv2model.Protocol
		subnets                           buildLoadBalancerSubnetsOutput
		gwLsCfg                           *gwListenerConfig
		lbLsCfg                           *elbv2gw.ListenerConfiguration
		resolveSubnetInLocalZoneOrOutpost resolveSubnetInLocalZoneOrOutpostCall
		describeTrustStores               describeTrustStoresCall
		want                              *elbv2model.MutualAuthenticationAttributes
		wantErr                           bool
	}{
		{
			name:     "non-secure protocol should return nil",
			protocol: elbv2model.ProtocolHTTP,
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTP,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{},
			want:    nil,
			wantErr: false,
		},
		{
			name:     "subnet in local zone or outpost should return nil",
			protocol: elbv2model.ProtocolHTTPS,
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   true,
				err:      nil,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:     "subnet resolver error should return error",
			protocol: elbv2model.ProtocolHTTPS,
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      errors.New("subnet resolver error"),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:     "nil lbLsCfg should return off mode",
			protocol: elbv2model.ProtocolHTTPS,
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: nil,
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
			},
			want: &elbv2model.MutualAuthenticationAttributes{
				Mode: string(elbv2gw.MutualAuthenticationOffMode),
			},
			wantErr: false,
		},
		{
			name:     "nil mutualAuthentication should return off mode",
			protocol: elbv2model.ProtocolHTTPS,
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: nil,
			},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
			},
			want: &elbv2model.MutualAuthenticationAttributes{
				Mode: string(elbv2gw.MutualAuthenticationOffMode),
			},
			wantErr: false,
		},
		{
			name:     "verify mode with truststore name should resolve ARN",
			protocol: elbv2model.ProtocolHTTPS,
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:       verifyMode,
					TrustStore: &trustStoreNameOnly,
				},
			},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
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
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:       verifyMode,
					TrustStore: &trustStoreArn,
				},
			},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
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
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:                          verifyMode,
					TrustStore:                    &trustStoreArn,
					IgnoreClientCertificateExpiry: &trueValue,
					AdvertiseTrustStoreCaNames:    &advertiseTrustStoreCaNames,
				},
			},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
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
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:                          verifyMode,
					TrustStore:                    &trustStoreArn,
					IgnoreClientCertificateExpiry: nil,
					AdvertiseTrustStoreCaNames:    &advertiseTrustStoreCaNames,
				},
			},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
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
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode: passthroughMode,
				},
			},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
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
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode: offMode,
				},
			},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
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
			subnets: buildLoadBalancerSubnetsOutput{
				subnets: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-1",
					},
				},
			},
			gwLsCfg: &gwListenerConfig{
				protocol:  elbv2model.ProtocolHTTPS,
				hostnames: []string{"example.com"},
			},
			lbLsCfg: &elbv2gw.ListenerConfiguration{
				MutualAuthentication: &elbv2gw.MutualAuthenticationAttributes{
					Mode:       verifyMode,
					TrustStore: &nonExistentTrustStoreNameOnly,
				},
			},
			resolveSubnetInLocalZoneOrOutpost: resolveSubnetInLocalZoneOrOutpostCall{
				subnetID: "subnet-1",
				result:   false,
				err:      nil,
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

			mockSubnetsResolver := networking.NewMockSubnetsResolver(ctrl)
			mockELBV2Client := services.NewMockELBV2(ctrl)

			if tt.protocol == elbv2model.ProtocolHTTPS {
				callInfo := tt.resolveSubnetInLocalZoneOrOutpost
				mockSubnetsResolver.EXPECT().
					IsSubnetInLocalZoneOrOutpost(gomock.Any(), callInfo.subnetID).
					Return(callInfo.result, callInfo.err)

				if !callInfo.result && callInfo.err == nil &&
					tt.lbLsCfg != nil && tt.lbLsCfg.MutualAuthentication != nil &&
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
				subnetsResolver: mockSubnetsResolver,
				elbv2Client:     mockELBV2Client,
			}

			got, err := builder.buildMutualAuthenticationAttributes(context.Background(), mockSubnetsResolver, tt.subnets, tt.gwLsCfg, tt.lbLsCfg)
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
		sgOutput         securityGroupOutput
		ipAddressType    elbv2model.IPAddressType
		port             int32
		listenerProtocol elbv2model.Protocol
		routes           map[int32][]routeutils.RouteDescriptor

		expectedRules []*elbv2model.ListenerRuleSpec
		expectedTags  map[string]string
		tagErr        error
	}{
		{
			name:             "no backends should result in 503 fixed response",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTP,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			sgOutput: securityGroupOutput{
				backendSecurityGroupToken: coremodel.LiteralStringToken("sg-B"),
			},
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
								StatusCode:  "503",
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
			sgOutput: securityGroupOutput{
				backendSecurityGroupToken: coremodel.LiteralStringToken("sg-B"),
			},
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
										Service:     &corev1.Service{},
										ServicePort: &corev1.ServicePort{Name: "svcport"},
										Weight:      1,
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
			name:             "redirect filter should result in redirect action",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTP,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			sgOutput: securityGroupOutput{
				backendSecurityGroupToken: coremodel.LiteralStringToken("sg-B"),
			},
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
										Service:     &corev1.Service{},
										ServicePort: &corev1.ServicePort{Name: "svcport"},
										Weight:      1,
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
			sgOutput: securityGroupOutput{
				backendSecurityGroupToken: coremodel.LiteralStringToken("sg-B"),
			},
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
										Service:     &corev1.Service{},
										ServicePort: &corev1.ServicePort{Name: "svcport"},
										Weight:      1,
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
			sgOutput: securityGroupOutput{
				backendSecurityGroupToken: coremodel.LiteralStringToken("sg-B"),
			},
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
										Service:     &corev1.Service{},
										ServicePort: &corev1.ServicePort{Name: "svcport"},
										Weight:      1,
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
			name:             "listener rule config with authenticate-cognito and no backends should result in auth + 503 fixed response",
			port:             80,
			listenerProtocol: elbv2model.ProtocolHTTPS,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			sgOutput: securityGroupOutput{
				backendSecurityGroupToken: coremodel.LiteralStringToken("sg-B"),
			},
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
								StatusCode:  "503",
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

			mockTgBuilder := &MockTargetGroupBuilder{
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

			err := builder.buildListenerRules(stack, &elbv2model.Listener{
				Spec: elbv2model.ListenerSpec{
					Protocol: tc.listenerProtocol,
				},
			}, tc.ipAddressType, tc.sgOutput, &gwv1.Gateway{}, tc.port, elbv2gw.LoadBalancerConfiguration{}, tc.routes)
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
