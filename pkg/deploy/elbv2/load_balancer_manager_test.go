package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/stretchr/testify/assert"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_buildSDKCreateLoadBalancerInput(t *testing.T) {
	schemeInternetFacing := elbv2model.LoadBalancerSchemeInternetFacing
	addressTypeDualStack := elbv2model.IPAddressTypeDualStack
	type args struct {
		lbSpec elbv2model.LoadBalancerSpec
	}
	tests := []struct {
		name    string
		args    args
		want    *elbv2sdk.CreateLoadBalancerInput
		wantErr error
	}{
		{
			name: "application loadBalancer - standard case",
			args: args{
				lbSpec: elbv2model.LoadBalancerSpec{
					Name:          "my-alb",
					Type:          elbv2model.LoadBalancerTypeApplication,
					Scheme:        &schemeInternetFacing,
					IPAddressType: &addressTypeDualStack,
					SubnetMappings: []elbv2model.SubnetMapping{
						{
							SubnetID: "subnet-A",
						},
						{
							SubnetID: "subnet-B",
						},
					},
					SecurityGroups: []coremodel.StringToken{
						coremodel.LiteralStringToken("sg-A"),
						coremodel.LiteralStringToken("sg-B"),
					},
				},
			},
			want: &elbv2sdk.CreateLoadBalancerInput{
				Name:          awssdk.String("my-alb"),
				Type:          awssdk.String("application"),
				IpAddressType: awssdk.String("dualstack"),
				Scheme:        awssdk.String("internet-facing"),
				SubnetMappings: []*elbv2sdk.SubnetMapping{
					{
						SubnetId: awssdk.String("subnet-A"),
					},
					{
						SubnetId: awssdk.String("subnet-B"),
					},
				},
				SecurityGroups: awssdk.StringSlice([]string{"sg-A", "sg-B"}),
			},
		},
		{
			name: "network loadBalancer - standard case",
			args: args{
				lbSpec: elbv2model.LoadBalancerSpec{
					Name:          "my-nlb",
					Type:          elbv2model.LoadBalancerTypeNetwork,
					Scheme:        &schemeInternetFacing,
					IPAddressType: &addressTypeDualStack,
					SubnetMappings: []elbv2model.SubnetMapping{
						{
							SubnetID: "subnet-A",
						},
						{
							SubnetID: "subnet-B",
						},
					},
				},
			},
			want: &elbv2sdk.CreateLoadBalancerInput{
				Name:          awssdk.String("my-nlb"),
				Type:          awssdk.String("network"),
				IpAddressType: awssdk.String("dualstack"),
				Scheme:        awssdk.String("internet-facing"),
				SubnetMappings: []*elbv2sdk.SubnetMapping{
					{
						SubnetId: awssdk.String("subnet-A"),
					},
					{
						SubnetId: awssdk.String("subnet-B"),
					},
				},
			},
		},
		{
			name: "application loadBalancer - with CoIP pool",
			args: args{
				lbSpec: elbv2model.LoadBalancerSpec{
					Name:          "my-alb",
					Type:          elbv2model.LoadBalancerTypeApplication,
					Scheme:        &schemeInternetFacing,
					IPAddressType: &addressTypeDualStack,
					SubnetMappings: []elbv2model.SubnetMapping{
						{
							SubnetID: "subnet-A",
						},
						{
							SubnetID: "subnet-B",
						},
					},
					SecurityGroups: []coremodel.StringToken{
						coremodel.LiteralStringToken("sg-A"),
						coremodel.LiteralStringToken("sg-B"),
					},
					CustomerOwnedIPv4Pool: awssdk.String("coIP-pool-x"),
				},
			},
			want: &elbv2sdk.CreateLoadBalancerInput{
				Name:          awssdk.String("my-alb"),
				Type:          awssdk.String("application"),
				IpAddressType: awssdk.String("dualstack"),
				Scheme:        awssdk.String("internet-facing"),
				SubnetMappings: []*elbv2sdk.SubnetMapping{
					{
						SubnetId: awssdk.String("subnet-A"),
					},
					{
						SubnetId: awssdk.String("subnet-B"),
					},
				},
				SecurityGroups:        awssdk.StringSlice([]string{"sg-A", "sg-B"}),
				CustomerOwnedIpv4Pool: awssdk.String("coIP-pool-x"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSDKCreateLoadBalancerInput(tt.args.lbSpec)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_buildSDKSubnetMappings(t *testing.T) {
	type args struct {
		modelSubnetMappings []elbv2model.SubnetMapping
	}
	tests := []struct {
		name string
		args args
		want []*elbv2sdk.SubnetMapping
	}{
		{
			name: "standard case",
			args: args{
				modelSubnetMappings: []elbv2model.SubnetMapping{
					{
						SubnetID: "subnet-a",
					},
					{
						SubnetID: "subnet-b",
					},
				},
			},
			want: []*elbv2sdk.SubnetMapping{
				{
					SubnetId: awssdk.String("subnet-a"),
				},
				{
					SubnetId: awssdk.String("subnet-b"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSDKSubnetMappings(tt.args.modelSubnetMappings)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildSDKSecurityGroups(t *testing.T) {
	type args struct {
		modelSecurityGroups []coremodel.StringToken
	}
	tests := []struct {
		name    string
		args    args
		want    []*string
		wantErr error
	}{
		{
			name: "one securityGroup",
			args: args{
				modelSecurityGroups: []coremodel.StringToken{
					coremodel.LiteralStringToken("sg-a"),
				},
			},
			want: awssdk.StringSlice([]string{"sg-a"}),
		},
		{
			name: "multiple securityGroups",
			args: args{
				modelSecurityGroups: []coremodel.StringToken{
					coremodel.LiteralStringToken("sg-a"),
					coremodel.LiteralStringToken("sg-b"),
				},
			},
			want: awssdk.StringSlice([]string{"sg-a", "sg-b"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSDKSecurityGroups(tt.args.modelSecurityGroups)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_buildSDKSubnetMapping(t *testing.T) {
	type args struct {
		modelSubnetMapping elbv2model.SubnetMapping
	}
	tests := []struct {
		name string
		args args
		want *elbv2sdk.SubnetMapping
	}{
		{
			name: "stand case",
			args: args{
				modelSubnetMapping: elbv2model.SubnetMapping{
					AllocationID:       awssdk.String("some-id"),
					PrivateIPv4Address: awssdk.String("192.168.100.0"),
					SubnetID:           "subnet-abc",
				},
			},
			want: &elbv2sdk.SubnetMapping{
				AllocationId:       awssdk.String("some-id"),
				PrivateIPv4Address: awssdk.String("192.168.100.0"),
				SubnetId:           awssdk.String("subnet-abc"),
			},
		},
		{
			name: "only-subnet specified",
			args: args{
				modelSubnetMapping: elbv2model.SubnetMapping{
					SubnetID: "subnet-abc",
				},
			},
			want: &elbv2sdk.SubnetMapping{
				SubnetId: awssdk.String("subnet-abc"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSDKSubnetMapping(tt.args.modelSubnetMapping)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildResLoadBalancerStatus(t *testing.T) {
	type args struct {
		sdkLB LoadBalancerWithTags
	}
	tests := []struct {
		name string
		args args
		want elbv2model.LoadBalancerStatus
	}{
		{
			name: "standard case",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						LoadBalancerArn: awssdk.String("my-arn"),
						DNSName:         awssdk.String("www.example.com"),
					},
				},
			},
			want: elbv2model.LoadBalancerStatus{
				LoadBalancerARN: "my-arn",
				DNSName:         "www.example.com",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildResLoadBalancerStatus(tt.args.sdkLB)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultLoadBalancerManager_checkSDKLoadBalancerWithCOIPv4Pool(t *testing.T) {
	type args struct {
		resLB *elbv2model.LoadBalancer
		sdkLB LoadBalancerWithTags
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "both resLB and sdkLB don't have CustomerOwnedIPv4Pool setting",
			args: args{
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						CustomerOwnedIPv4Pool: nil,
					},
				},
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						CustomerOwnedIpv4Pool: nil,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "both resLB and sdkLB have same CustomerOwnedIPv4Pool setting",
			args: args{
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						CustomerOwnedIPv4Pool: awssdk.String("ipv4pool-coip-abc"),
					},
				},
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						CustomerOwnedIpv4Pool: awssdk.String("ipv4pool-coip-abc"),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "both resLB and sdkLB have different CustomerOwnedIPv4Pool setting",
			args: args{
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						CustomerOwnedIPv4Pool: awssdk.String("ipv4pool-coip-abc"),
					},
				},
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						CustomerOwnedIpv4Pool: awssdk.String("ipv4pool-coip-def"),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "only resLB have CustomerOwnedIPv4Pool setting",
			args: args{
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						CustomerOwnedIPv4Pool: awssdk.String("ipv4pool-coip-abc"),
					},
				},
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						CustomerOwnedIpv4Pool: nil,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "only sdkLB have CustomerOwnedIPv4Pool setting",
			args: args{
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						CustomerOwnedIPv4Pool: nil,
					},
				},
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						CustomerOwnedIpv4Pool: awssdk.String("ipv4pool-coip-abc"),
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultLoadBalancerManager{
				logger: &log.NullLogger{},
			}
			err := m.checkSDKLoadBalancerWithCOIPv4Pool(context.Background(), tt.args.resLB, tt.args.sdkLB)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
