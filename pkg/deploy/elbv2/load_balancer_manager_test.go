package elbv2

import (
	"context"
	"testing"

	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_buildSDKCreateLoadBalancerInput(t *testing.T) {
	schemeInternetFacing := elbv2model.LoadBalancerSchemeInternetFacing
	addressTypeDualStack := elbv2model.IPAddressTypeDualStack
	enablePrefixForIpv6SourceNatOn := elbv2model.EnablePrefixForIpv6SourceNatOn
	enablePrefixForIpv6SourceNatOff := elbv2model.EnablePrefixForIpv6SourceNatOff
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
					Scheme:        schemeInternetFacing,
					IPAddressType: addressTypeDualStack,
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
				Type:          elbv2types.LoadBalancerTypeEnumApplication,
				IpAddressType: elbv2types.IpAddressTypeDualstack,
				Scheme:        elbv2types.LoadBalancerSchemeEnumInternetFacing,
				SubnetMappings: []elbv2types.SubnetMapping{
					{
						SubnetId: awssdk.String("subnet-A"),
					},
					{
						SubnetId: awssdk.String("subnet-B"),
					},
				},
				SecurityGroups: []string{"sg-A", "sg-B"},
			},
		},
		{
			name: "network loadBalancer - standard case",
			args: args{
				lbSpec: elbv2model.LoadBalancerSpec{
					Name:          "my-nlb",
					Type:          elbv2model.LoadBalancerTypeNetwork,
					Scheme:        schemeInternetFacing,
					IPAddressType: addressTypeDualStack,
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
				Type:          elbv2types.LoadBalancerTypeEnumNetwork,
				IpAddressType: elbv2types.IpAddressTypeDualstack,
				Scheme:        elbv2types.LoadBalancerSchemeEnumInternetFacing,
				SubnetMappings: []elbv2types.SubnetMapping{
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
			name: "network loadBalancer - Dualstack UDP Support over IPv6 - on",
			args: args{
				lbSpec: elbv2model.LoadBalancerSpec{
					Name:          "my-nlb",
					Type:          elbv2model.LoadBalancerTypeNetwork,
					Scheme:        schemeInternetFacing,
					IPAddressType: addressTypeDualStack,
					SubnetMappings: []elbv2model.SubnetMapping{
						{
							SubnetID: "subnet-A",
						},
						{
							SubnetID: "subnet-B",
						},
					},
					EnablePrefixForIpv6SourceNat: enablePrefixForIpv6SourceNatOn,
				},
			},
			want: &elbv2sdk.CreateLoadBalancerInput{
				Name:          awssdk.String("my-nlb"),
				Type:          elbv2types.LoadBalancerTypeEnumNetwork,
				IpAddressType: elbv2types.IpAddressTypeDualstack,
				Scheme:        elbv2types.LoadBalancerSchemeEnumInternetFacing,
				SubnetMappings: []elbv2types.SubnetMapping{
					{
						SubnetId: awssdk.String("subnet-A"),
					},
					{
						SubnetId: awssdk.String("subnet-B"),
					},
				},
				EnablePrefixForIpv6SourceNat: elbv2types.EnablePrefixForIpv6SourceNatEnumOn,
			},
		},
		{
			name: "network loadBalancer - Dualstack UDP Support over IPv6 - off",
			args: args{
				lbSpec: elbv2model.LoadBalancerSpec{
					Name:          "my-nlb",
					Type:          elbv2model.LoadBalancerTypeNetwork,
					Scheme:        elbv2model.LoadBalancerSchemeInternetFacing,
					IPAddressType: elbv2model.IPAddressTypeDualStack,
					SubnetMappings: []elbv2model.SubnetMapping{
						{
							SubnetID: "subnet-A",
						},
						{
							SubnetID: "subnet-B",
						},
					},
					EnablePrefixForIpv6SourceNat: enablePrefixForIpv6SourceNatOff,
				},
			},
			want: &elbv2sdk.CreateLoadBalancerInput{
				Name:          awssdk.String("my-nlb"),
				Type:          elbv2types.LoadBalancerTypeEnumNetwork,
				IpAddressType: elbv2types.IpAddressTypeDualstack,
				Scheme:        elbv2types.LoadBalancerSchemeEnumInternetFacing,
				SubnetMappings: []elbv2types.SubnetMapping{
					{
						SubnetId: awssdk.String("subnet-A"),
					},
					{
						SubnetId: awssdk.String("subnet-B"),
					},
				},
				EnablePrefixForIpv6SourceNat: elbv2types.EnablePrefixForIpv6SourceNatEnumOff,
			},
		},
		{
			name: "application loadBalancer - with CoIP pool",
			args: args{
				lbSpec: elbv2model.LoadBalancerSpec{
					Name:          "my-alb",
					Type:          elbv2model.LoadBalancerTypeApplication,
					Scheme:        schemeInternetFacing,
					IPAddressType: addressTypeDualStack,
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
				Type:          elbv2types.LoadBalancerTypeEnumApplication,
				IpAddressType: elbv2types.IpAddressTypeDualstack,
				Scheme:        elbv2types.LoadBalancerSchemeEnumInternetFacing,
				SubnetMappings: []elbv2types.SubnetMapping{
					{
						SubnetId: awssdk.String("subnet-A"),
					},
					{
						SubnetId: awssdk.String("subnet-B"),
					},
				},
				SecurityGroups:        []string{"sg-A", "sg-B"},
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
	sourceNatIpv6PrefixAutoAssigned := elbv2model.SourceNatIpv6PrefixAutoAssigned
	type args struct {
		modelSubnetMappings []elbv2model.SubnetMapping
	}
	tests := []struct {
		name string
		args args
		want []elbv2types.SubnetMapping
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
			want: []elbv2types.SubnetMapping{
				{
					SubnetId: awssdk.String("subnet-a"),
				},
				{
					SubnetId: awssdk.String("subnet-b"),
				},
			},
		},
		{
			name: "subnet mappings with sourceNAT prefix",
			args: args{
				modelSubnetMappings: []elbv2model.SubnetMapping{
					{
						SubnetID:            "subnet-a",
						SourceNatIpv6Prefix: &sourceNatIpv6PrefixAutoAssigned,
					},
				},
			},
			want: []elbv2types.SubnetMapping{
				{
					SubnetId:            awssdk.String("subnet-a"),
					SourceNatIpv6Prefix: awssdk.String("auto_assigned"),
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
		want    []string
		wantErr error
	}{
		{
			name: "one securityGroup",
			args: args{
				modelSecurityGroups: []coremodel.StringToken{
					coremodel.LiteralStringToken("sg-a"),
				},
			},
			want: []string{"sg-a"},
		},
		{
			name: "multiple securityGroups",
			args: args{
				modelSecurityGroups: []coremodel.StringToken{
					coremodel.LiteralStringToken("sg-a"),
					coremodel.LiteralStringToken("sg-b"),
				},
			},
			want: []string{"sg-a", "sg-b"},
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
		want elbv2types.SubnetMapping
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
			want: elbv2types.SubnetMapping{
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
			want: elbv2types.SubnetMapping{
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
					LoadBalancer: &elbv2types.LoadBalancer{
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
					LoadBalancer: &elbv2types.LoadBalancer{
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
					LoadBalancer: &elbv2types.LoadBalancer{
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
					LoadBalancer: &elbv2types.LoadBalancer{
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
					LoadBalancer: &elbv2types.LoadBalancer{
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
					LoadBalancer: &elbv2types.LoadBalancer{
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
				logger: logr.New(&log.NullLogSink{}),
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

func Test_defaultLoadBalancerManager_updateSDKLoadBalancerWithSubnetMappings(t *testing.T) {
	type setSubnetsWithContextCall struct {
		req  *elbv2sdk.SetSubnetsInput
		resp *elbv2sdk.SetSubnetsOutput
		err  error
	}
	type fields struct {
		setSubnetsWithContextCall setSubnetsWithContextCall
	}
	enablePrefixForIpv6SourceNatOn := elbv2model.EnablePrefixForIpv6SourceNatOn
	enablePrefixForIpv6SourceNatOff := elbv2model.EnablePrefixForIpv6SourceNatOff
	sourceNatIpv6PrefixAutoAssigned := elbv2model.SourceNatIpv6PrefixAutoAssigned
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		resLB *elbv2model.LoadBalancer
		sdkLB LoadBalancerWithTags
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
		fields  fields
	}{
		{
			name: "should set the updated sourceNAT SourceNatIpv6Prefix",
			fields: fields{
				setSubnetsWithContextCall: setSubnetsWithContextCall{
					req: &elbv2sdk.SetSubnetsInput{
						LoadBalancerArn:              awssdk.String("LoadBalancerArn"),
						EnablePrefixForIpv6SourceNat: elbv2types.EnablePrefixForIpv6SourceNatEnumOn,
						SubnetMappings: []elbv2types.SubnetMapping{
							{
								SubnetId:            awssdk.String("subnet-A"),
								SourceNatIpv6Prefix: &sourceNatIpv6PrefixAutoAssigned,
							},
						},
					},
					resp: &elbv2sdk.SetSubnetsOutput{},
				},
			},
			args: args{
				resLB: &elbv2model.LoadBalancer{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						Type:                         elbv2model.LoadBalancerTypeNetwork,
						EnablePrefixForIpv6SourceNat: enablePrefixForIpv6SourceNatOn,
						SubnetMappings: []elbv2model.SubnetMapping{
							{
								SubnetID:            "subnet-A",
								SourceNatIpv6Prefix: &sourceNatIpv6PrefixAutoAssigned,
							},
						},
					},
				},
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn:              awssdk.String("LoadBalancerArn"),
						EnablePrefixForIpv6SourceNat: elbv2types.EnablePrefixForIpv6SourceNatEnumOn,
						AvailabilityZones:            []elbv2types.AvailabilityZone{{SubnetId: awssdk.String("subnet-A"), SourceNatIpv6Prefixes: []string{"1024:004:003::/80"}}},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "should set the updated enablePrefixForIpv6SourceNat value",
			fields: fields{
				setSubnetsWithContextCall: setSubnetsWithContextCall{
					req: &elbv2sdk.SetSubnetsInput{
						LoadBalancerArn:              awssdk.String("LoadBalancerArn"),
						EnablePrefixForIpv6SourceNat: elbv2types.EnablePrefixForIpv6SourceNatEnumOff,
						SubnetMappings: []elbv2types.SubnetMapping{
							{
								SubnetId: awssdk.String("subnet-A"),
							},
						},
					},
					resp: &elbv2sdk.SetSubnetsOutput{},
				},
			},
			args: args{
				resLB: &elbv2model.LoadBalancer{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						Type: elbv2model.LoadBalancerTypeNetwork,
						SubnetMappings: []elbv2model.SubnetMapping{
							{
								SubnetID: "subnet-A",
							},
						},
						EnablePrefixForIpv6SourceNat: enablePrefixForIpv6SourceNatOff,
					},
				},
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2types.LoadBalancer{
						LoadBalancerArn:              awssdk.String("LoadBalancerArn"),
						EnablePrefixForIpv6SourceNat: elbv2types.EnablePrefixForIpv6SourceNatEnumOn,
						AvailabilityZones:            []elbv2types.AvailabilityZone{{SubnetId: awssdk.String("subnet-A"), SourceNatIpv6Prefixes: []string{"1024:004:003::/80"}}},
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2Client := services.NewMockELBV2(ctrl)
			m := &defaultLoadBalancerManager{
				logger:      logr.New(&log.NullLogSink{}),
				elbv2Client: elbv2Client,
			}

			elbv2Client.EXPECT().SetSubnetsWithContext(gomock.Any(), tt.fields.setSubnetsWithContextCall.req).Return(tt.fields.setSubnetsWithContextCall.resp, tt.fields.setSubnetsWithContextCall.err)

			err := m.updateSDKLoadBalancerWithSubnetMappings(context.Background(), tt.args.resLB, tt.args.sdkLB)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
