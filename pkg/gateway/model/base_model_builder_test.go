package model

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

func Test_baseModelBuilder_isDeleteProtected(t *testing.T) {
	tests := []struct {
		name   string
		lbConf elbv2gw.LoadBalancerConfiguration
		want   bool
	}{
		{
			name: "deletion protection enabled",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   shared_constants.LBAttributeDeletionProtection,
							Value: "true",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "deletion protection disabled",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   shared_constants.LBAttributeDeletionProtection,
							Value: "false",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "deletion protection not specified",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{},
				},
			},
			want: false,
		},
		{
			name: "deletion protection with invalid value",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   shared_constants.LBAttributeDeletionProtection,
							Value: "invalid",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "deletion protection among other attributes",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					LoadBalancerAttributes: []elbv2gw.LoadBalancerAttribute{
						{
							Key:   "idle_timeout.timeout_seconds",
							Value: "60",
						},
						{
							Key:   shared_constants.LBAttributeDeletionProtection,
							Value: "true",
						},
						{
							Key:   "access_logs.s3.enabled",
							Value: "false",
						},
					},
				},
			},
			want: true,
		},
		{
			name:   "empty config",
			lbConf: elbv2gw.LoadBalancerConfiguration{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &baseModelBuilder{
				logger: logr.Discard(),
			}
			got := builder.isDeleteProtected(tt.lbConf)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_baseModelBuilder_buildLoadBalancerScheme(t *testing.T) {
	internetFacing := elbv2gw.LoadBalancerScheme(elbv2model.LoadBalancerSchemeInternetFacing)
	internal := elbv2gw.LoadBalancerScheme(elbv2model.LoadBalancerSchemeInternal)
	unknown := elbv2gw.LoadBalancerScheme("unknown")

	tests := []struct {
		name          string
		lbConf        elbv2gw.LoadBalancerConfiguration
		defaultScheme elbv2model.LoadBalancerScheme
		want          elbv2model.LoadBalancerScheme
		wantErr       bool
	}{
		{
			name: "internet-facing scheme",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					Scheme: &internetFacing,
				},
			},
			defaultScheme: elbv2model.LoadBalancerSchemeInternal,
			want:          elbv2model.LoadBalancerSchemeInternetFacing,
			wantErr:       false,
		},
		{
			name: "internal scheme",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					Scheme: &internal,
				},
			},
			defaultScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			want:          elbv2model.LoadBalancerSchemeInternal,
			wantErr:       false,
		},
		{
			name: "nil scheme uses default",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					Scheme: nil,
				},
			},
			defaultScheme: elbv2model.LoadBalancerSchemeInternal,
			want:          elbv2model.LoadBalancerSchemeInternal,
			wantErr:       false,
		},
		{
			name: "unknown scheme returns error",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					Scheme: &unknown,
				},
			},
			defaultScheme: elbv2model.LoadBalancerSchemeInternal,
			want:          "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &baseModelBuilder{
				defaultLoadBalancerScheme: tt.defaultScheme,
			}
			got, err := builder.buildLoadBalancerScheme(tt.lbConf)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_baseModelBuilder_buildLoadBalancerIPAddressType(t *testing.T) {
	ipv4 := elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeIPV4)
	dualStack := elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeDualStack)
	dualStackWithoutPublicIPv4 := elbv2gw.LoadBalancerIpAddressType(elbv2model.IPAddressTypeDualStackWithoutPublicIPV4)
	unknown := elbv2gw.LoadBalancerIpAddressType("unknown")

	tests := []struct {
		name        string
		lbConf      elbv2gw.LoadBalancerConfiguration
		defaultType elbv2model.IPAddressType
		want        elbv2model.IPAddressType
		wantErr     bool
	}{
		{
			name: "ipv4 address type",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					IpAddressType: &ipv4,
				},
			},
			defaultType: elbv2model.IPAddressTypeDualStack,
			want:        elbv2model.IPAddressTypeIPV4,
			wantErr:     false,
		},
		{
			name: "dualstack address type",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					IpAddressType: &dualStack,
				},
			},
			defaultType: elbv2model.IPAddressTypeIPV4,
			want:        elbv2model.IPAddressTypeDualStack,
			wantErr:     false,
		},
		{
			name: "dualstack without public ipv4 address type",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					IpAddressType: &dualStackWithoutPublicIPv4,
				},
			},
			defaultType: elbv2model.IPAddressTypeIPV4,
			want:        elbv2model.IPAddressTypeDualStackWithoutPublicIPV4,
			wantErr:     false,
		},
		{
			name: "nil address type uses default",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					IpAddressType: nil,
				},
			},
			defaultType: elbv2model.IPAddressTypeIPV4,
			want:        elbv2model.IPAddressTypeIPV4,
			wantErr:     false,
		},
		{
			name: "unknown address type returns error",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					IpAddressType: &unknown,
				},
			},
			defaultType: elbv2model.IPAddressTypeIPV4,
			want:        "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &baseModelBuilder{
				defaultIPType: tt.defaultType,
			}
			got, err := builder.buildLoadBalancerIPAddressType(tt.lbConf)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
