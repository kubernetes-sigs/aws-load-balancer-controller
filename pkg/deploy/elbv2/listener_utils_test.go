package elbv2

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
)

func Test_isListenerNotFoundError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "is ListenerNotFound error",
			args: args{
				err: &smithy.GenericAPIError{Code: "ListenerNotFound", Message: "some message"},
			},
			want: true,
		},
		{
			name: "wraps ListenerNotFound error",
			args: args{
				err: errors.Wrap(&smithy.GenericAPIError{Code: "ListenerNotFound", Message: "some message"}, "wrapped message"),
			},
			want: true,
		},
		{
			name: "isn't ListenerNotFound error",
			args: args{
				err: errors.New("some other error"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isListenerNotFoundError(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildSDKSourceIpConditionConfig(t *testing.T) {
	tests := []struct {
		name     string
		modelCfg elbv2model.SourceIPConditionConfig
		want     *elbv2types.SourceIpConditionConfig
	}{
		{
			name:     "ALB CIDR values only",
			modelCfg: elbv2model.SourceIPConditionConfig{Values: []string{"10.0.0.0/8", "192.168.0.0/16"}},
			want:     &elbv2types.SourceIpConditionConfig{Values: []string{"10.0.0.0/8", "192.168.0.0/16"}},
		},
		{
			name:     "NLB IpAddressType ipv4",
			modelCfg: elbv2model.SourceIPConditionConfig{IpAddressType: (*elbv2model.SourceIPAddressType)(awssdk.String("ipv4"))},
			want:     &elbv2types.SourceIpConditionConfig{IpAddressType: elbv2types.SourceIpAddressTypeEnum("ipv4")},
		},
		{
			name:     "NLB IpAddressType ipv6",
			modelCfg: elbv2model.SourceIPConditionConfig{IpAddressType: (*elbv2model.SourceIPAddressType)(awssdk.String("ipv6"))},
			want:     &elbv2types.SourceIpConditionConfig{IpAddressType: elbv2types.SourceIpAddressTypeEnum("ipv6")},
		},
		{
			name:     "empty config",
			modelCfg: elbv2model.SourceIPConditionConfig{},
			want:     &elbv2types.SourceIpConditionConfig{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSDKSourceIpConditionConfig(tt.modelCfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildSDKJwtValidationConfig(t *testing.T) {
	type args struct {
		jwtValidationConfig elbv2model.JwtValidationConfig
	}
	tests := []struct {
		name string
		args args
		want *elbv2types.JwtValidationActionConfig
	}{
		{
			name: "jwt validation with no additional claims",
			args: args{
				jwtValidationConfig: elbv2model.JwtValidationConfig{
					JwksEndpoint: "https://issuer.example.com/.well-known/jwks.json",
					Issuer:       "https://issuer.com",
				},
			},
			want: &elbv2types.JwtValidationActionConfig{
				JwksEndpoint: awssdk.String("https://issuer.example.com/.well-known/jwks.json"),
				Issuer:       awssdk.String("https://issuer.com"),
			},
		},
		{
			name: "jwt validation with additional claims",
			args: args{
				jwtValidationConfig: elbv2model.JwtValidationConfig{
					JwksEndpoint: "https://issuer.example.com/.well-known/jwks.json",
					Issuer:       "https://issuer.com",
					AdditionalClaims: []elbv2model.JwtAdditionalClaim{
						{
							Format: "string-array",
							Name:   "scope",
							Values: []string{"read:api", "write:api"},
						},
						{
							Format: "single-string",
							Name:   "iat",
							Values: []string{"12456"},
						},
						{
							Format: "space-separated-values",
							Name:   "aud",
							Values: []string{"https://example.com", "https://another-site.com"},
						},
					},
				},
			},
			want: &elbv2types.JwtValidationActionConfig{
				JwksEndpoint: awssdk.String("https://issuer.example.com/.well-known/jwks.json"),
				Issuer:       awssdk.String("https://issuer.com"),
				AdditionalClaims: []elbv2types.JwtValidationActionAdditionalClaim{
					{
						Format: "string-array",
						Name:   awssdk.String("scope"),
						Values: []string{"read:api", "write:api"},
					},
					{
						Format: "single-string",
						Name:   awssdk.String("iat"),
						Values: []string{"12456"},
					},
					{
						Format: "space-separated-values",
						Name:   awssdk.String("aud"),
						Values: []string{"https://example.com", "https://another-site.com"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			got := buildSDKJwtValidationConfig(tt.args.jwtValidationConfig)
			assert.Equal(t, tt.want, got)
		})
	}
}
