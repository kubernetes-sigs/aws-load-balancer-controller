package model

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/model/subnet"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"testing"
)

func Test_NewSubnetModelBuilder(t *testing.T) {

	trackingProvider := tracking.NewDefaultProvider("", "")
	subnetResolver := networking.NewDefaultSubnetsResolver(nil, nil, "", "", false, false, false, logr.Discard())
	taggingManager := elbv2deploy.NewDefaultTaggingManager(nil, "", nil, nil, logr.Discard())

	builderNLB := newSubnetModelBuilder(elbv2model.LoadBalancerTypeNetwork, trackingProvider, subnetResolver, taggingManager)
	subnetBuilderNLB := builderNLB.(*subnetModelBuilderImpl)

	assert.Equal(t, trackingProvider, subnetBuilderNLB.trackingProvider)
	assert.Equal(t, subnetResolver, subnetBuilderNLB.subnetsResolver)
	assert.Equal(t, taggingManager, subnetBuilderNLB.elbv2TaggingManager)
	assert.Equal(t, 4, len(subnetBuilderNLB.subnetMutatorChain))

	builderALB := newSubnetModelBuilder(elbv2model.LoadBalancerTypeApplication, trackingProvider, subnetResolver, taggingManager)
	subnetBuilderALB := builderALB.(*subnetModelBuilderImpl)

	assert.Equal(t, trackingProvider, subnetBuilderALB.trackingProvider)
	assert.Equal(t, subnetResolver, subnetBuilderALB.subnetsResolver)
	assert.Equal(t, taggingManager, subnetBuilderALB.elbv2TaggingManager)
	assert.Equal(t, 0, len(subnetBuilderALB.subnetMutatorChain))
}

type mockMutator struct {
	called int
}

func (m *mockMutator) Mutate(_ []*elbv2model.SubnetMapping, _ []ec2types.Subnet, _ []elbv2gw.SubnetConfiguration) error {
	m.called++
	return nil
}

// Test Basic flow here, in depth validation goes in helper function tests.
func Test_BuildLoadBalancerSubnets(t *testing.T) {

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	subnetsResolver := networking.NewMockSubnetsResolver(ctrl)
	subnetsResolver.EXPECT().ResolveViaNameOrIDSlice(gomock.Any(), gomock.Any(), gomock.Any()).Return([]ec2types.Subnet{
		{
			SubnetId:  awssdk.String("foo"),
			CidrBlock: awssdk.String("192.168.0.0/19"),
		},
		{
			SubnetId:  awssdk.String("bar"),
			CidrBlock: awssdk.String("192.168.0.0/19"),
		},
	}, nil)

	mm := &mockMutator{}

	builder := subnetModelBuilderImpl{
		loadBalancerType: elbv2model.LoadBalancerTypeNetwork,
		subnetMutatorChain: []subnet.Mutator{
			mm,
		},
		subnetsResolver: subnetsResolver,
	}

	gwSubnetConfig := []elbv2gw.SubnetConfiguration{
		{
			Identifier: "foo",
		},
		{
			Identifier: "bar",
		},
	}

	expectedMappings := []elbv2model.SubnetMapping{
		{
			SubnetID: "foo",
		},
		{
			SubnetID: "bar",
		},
	}

	output, err := builder.buildLoadBalancerSubnets(context.Background(), &gwSubnetConfig, nil, elbv2model.LoadBalancerSchemeInternal, elbv2model.IPAddressTypeIPV4, nil)

	assert.NoError(t, err)
	assert.Equal(t, expectedMappings, output.subnets)
	assert.False(t, output.sourceIPv6NatEnabled)
	assert.Equal(t, 1, mm.called)
}

func Test_ValidateSubnetsInput(t *testing.T) {
	testCases := []struct {
		name          string
		lbType        elbv2model.LoadBalancerType
		lbScheme      elbv2model.LoadBalancerScheme
		ipAddressType elbv2model.IPAddressType

		subnetConfig []elbv2gw.SubnetConfiguration

		sourceNatEnabled bool
		expectErr        bool
	}{
		{
			name:          "no config to validate",
			lbType:        elbv2model.LoadBalancerTypeApplication,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeIPV4,
		},
		{
			name: "EIPAllocation specified",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					EIPAllocation: awssdk.String("foo"),
				},
				{
					EIPAllocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeIPV4,
		},
		{
			name: "IPv6 allocation specified",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					IPv6Allocation: awssdk.String("foo"),
				},
				{
					IPv6Allocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeDualStack,
		},
		{
			name: "PrivateIPv4Allocation specified",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					PrivateIPv4Allocation: awssdk.String("foo"),
				},
				{
					PrivateIPv4Allocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternal,
			ipAddressType: elbv2model.IPAddressTypeDualStack,
		},
		{
			name: "SourceNatIPv6Prefix not specified in all",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					SourceNatIPv6Prefix: awssdk.String("foo"),
				},
				{
					SourceNatIPv6Prefix: awssdk.String("bar"),
				},
			},
			lbType:           elbv2model.LoadBalancerTypeNetwork,
			lbScheme:         elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType:    elbv2model.IPAddressTypeIPV4,
			sourceNatEnabled: true,
		},
		{
			name: "EIPAllocation not specified in all",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					EIPAllocation: awssdk.String("foo"),
				},
				{},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeIPV4,
			expectErr:     true,
		},
		{
			name: "IPv6 allocation not specified in all",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					IPv6Allocation: awssdk.String("foo"),
				},
				{},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			expectErr:     true,
		},
		{
			name: "PrivateIPv4Allocation not specified in all",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					PrivateIPv4Allocation: awssdk.String("foo"),
				},
				{},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternal,
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			expectErr:     true,
		},
		{
			name: "SourceNatIPv6Prefix not specified in all",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					SourceNatIPv6Prefix: awssdk.String("foo"),
				},
				{},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeIPV4,
			expectErr:     true,
		},
		{
			name: "EIPAllocation specified for alb",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					EIPAllocation: awssdk.String("foo"),
				},
				{
					EIPAllocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeApplication,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeIPV4,
			expectErr:     true,
		},
		{
			name: "EIPAllocation specified for non-internet facing nlb",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					EIPAllocation: awssdk.String("foo"),
				},
				{
					EIPAllocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternal,
			ipAddressType: elbv2model.IPAddressTypeIPV4,
			expectErr:     true,
		},
		{
			name: "IPv6 allocation specified for alb",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					IPv6Allocation: awssdk.String("foo"),
				},
				{
					IPv6Allocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeApplication,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			expectErr:     true,
		},
		{
			name: "IPv6 allocation specified for non-dualstack nlb",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					IPv6Allocation: awssdk.String("foo"),
				},
				{
					IPv6Allocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeIPV4,
			expectErr:     true,
		},
		{
			name: "PrivateIPv4Allocation specified for internet facing nlb",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					PrivateIPv4Allocation: awssdk.String("foo"),
				},
				{
					PrivateIPv4Allocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeNetwork,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			expectErr:     true,
		},
		{
			name: "PrivateIPv4Allocation specified for alb",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					PrivateIPv4Allocation: awssdk.String("foo"),
				},
				{
					PrivateIPv4Allocation: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeApplication,
			lbScheme:      elbv2model.LoadBalancerSchemeInternal,
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			expectErr:     true,
		},
		{
			name: "SourceNatIPv6Prefix specified for alb",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					SourceNatIPv6Prefix: awssdk.String("foo"),
				},
				{
					SourceNatIPv6Prefix: awssdk.String("bar"),
				},
			},
			lbType:        elbv2model.LoadBalancerTypeApplication,
			lbScheme:      elbv2model.LoadBalancerSchemeInternetFacing,
			ipAddressType: elbv2model.IPAddressTypeIPV4,
			expectErr:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := subnetModelBuilderImpl{
				loadBalancerType: tc.lbType,
			}

			ipv6SourceNatEnabled, err := builder.validateSubnetsInput(&tc.subnetConfig, tc.lbScheme, tc.ipAddressType)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.sourceNatEnabled, ipv6SourceNatEnabled)
		})
	}
}

type mockProvider struct {
}

func (m *mockProvider) ResourceIDTagKey() string {
	//TODO implement me
	panic("implement me")
}

func (m *mockProvider) StackTags(stack core.Stack) map[string]string {
	return make(map[string]string)
}

func (m *mockProvider) ResourceTags(stack core.Stack, res core.Resource, additionalTags map[string]string) map[string]string {
	//TODO implement me
	panic("implement me")
}

func (m *mockProvider) StackLabels(stack core.Stack) map[string]string {
	//TODO implement me
	panic("implement me")
}

func (m *mockProvider) StackTagsLegacy(stack core.Stack) map[string]string {
	//TODO implement me
	panic("implement me")
}

func (m *mockProvider) LegacyTagKeys() []string {
	//TODO implement me
	panic("implement me")
}

func Test_ResolveEC2Subnets(t *testing.T) {

	type subnetCall struct {
		subnets []ec2types.Subnet
		err     error
	}

	type listLoadBalancersCall struct {
		sdkLBs []elbv2deploy.LoadBalancerWithTags
		err    error
	}

	testCases := []struct {
		name string

		subnetConfig           *[]elbv2gw.SubnetConfiguration
		selector               *map[string][]string
		idOrNameResolutionCall *subnetCall
		selectorCall           *subnetCall
		discoveryCall          *subnetCall

		listLoadBalancersCall *listLoadBalancersCall

		expected  []ec2types.Subnet
		expectErr bool
	}{
		{
			name: "resolve static ids",
			idOrNameResolutionCall: &subnetCall{
				subnets: []ec2types.Subnet{
					{
						SubnetId:  awssdk.String("foo"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
					{
						SubnetId:  awssdk.String("bar"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
				},
			},
			subnetConfig: &[]elbv2gw.SubnetConfiguration{
				{
					Identifier: "foo",
				},
				{
					Identifier: "bar",
				},
			},
			expected: []ec2types.Subnet{
				{
					SubnetId:  awssdk.String("foo"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
				{
					SubnetId:  awssdk.String("bar"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
			},
		},
		{
			name: "resolve static ids - fail",
			idOrNameResolutionCall: &subnetCall{
				err: errors.New("bad thing"),
			},
			subnetConfig: &[]elbv2gw.SubnetConfiguration{
				{
					Identifier: "foo",
				},
				{
					Identifier: "bar",
				},
			},
			expectErr: true,
		},
		{
			name: "resolve via selector",
			selectorCall: &subnetCall{
				subnets: []ec2types.Subnet{
					{
						SubnetId:  awssdk.String("foo"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
					{
						SubnetId:  awssdk.String("bar"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
				},
			},
			selector: &map[string][]string{
				"key1": {"v1", "v2"},
			},
			expected: []ec2types.Subnet{
				{
					SubnetId:  awssdk.String("foo"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
				{
					SubnetId:  awssdk.String("bar"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
			},
		},
		{
			name: "resolve via selector - fail",
			selectorCall: &subnetCall{
				err: errors.New("bad thing"),
			},
			selector: &map[string][]string{
				"key1": {"v1", "v2"},
			},
			expectErr: true,
		},
		{
			name: "no lbs triggers discovery",
			discoveryCall: &subnetCall{
				subnets: []ec2types.Subnet{
					{
						SubnetId:  awssdk.String("foo"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
					{
						SubnetId:  awssdk.String("bar"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
				},
			},
			listLoadBalancersCall: &listLoadBalancersCall{
				sdkLBs: []elbv2deploy.LoadBalancerWithTags{},
			},
			expected: []ec2types.Subnet{
				{
					SubnetId:  awssdk.String("foo"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
				{
					SubnetId:  awssdk.String("bar"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
			},
		},
		{
			name: "no lbs triggers discovery - fail",
			discoveryCall: &subnetCall{
				err: errors.New("bad thing"),
			},
			listLoadBalancersCall: &listLoadBalancersCall{
				sdkLBs: []elbv2deploy.LoadBalancerWithTags{},
			},
			expectErr: true,
		},
		{
			name: "wrong scheme triggers discovery",
			discoveryCall: &subnetCall{
				subnets: []ec2types.Subnet{
					{
						SubnetId:  awssdk.String("foo"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
					{
						SubnetId:  awssdk.String("bar"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
				},
			},
			listLoadBalancersCall: &listLoadBalancersCall{
				sdkLBs: []elbv2deploy.LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							Scheme: elbv2types.LoadBalancerSchemeEnumInternetFacing,
						},
					},
				},
			},
			expected: []ec2types.Subnet{
				{
					SubnetId:  awssdk.String("foo"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
				{
					SubnetId:  awssdk.String("bar"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
			},
		},
		{
			name: "reuse lb subnet settings for static ids",
			idOrNameResolutionCall: &subnetCall{
				subnets: []ec2types.Subnet{
					{
						SubnetId:  awssdk.String("foo"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
					{
						SubnetId:  awssdk.String("bar"),
						CidrBlock: awssdk.String("192.168.0.0/19"),
					},
				},
			},
			listLoadBalancersCall: &listLoadBalancersCall{
				sdkLBs: []elbv2deploy.LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2types.LoadBalancer{
							Scheme: elbv2types.LoadBalancerSchemeEnumInternal,
							AvailabilityZones: []elbv2types.AvailabilityZone{
								{
									SubnetId: awssdk.String("foo"),
								},
								{
									SubnetId: awssdk.String("bar"),
								},
							},
						},
					},
				},
			},
			expected: []ec2types.Subnet{
				{
					SubnetId:  awssdk.String("foo"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
				{
					SubnetId:  awssdk.String("bar"),
					CidrBlock: awssdk.String("192.168.0.0/19"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			subnetsResolver := networking.NewMockSubnetsResolver(ctrl)

			if tc.idOrNameResolutionCall != nil {
				subnetsResolver.EXPECT().ResolveViaNameOrIDSlice(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.idOrNameResolutionCall.subnets, tc.idOrNameResolutionCall.err)
			}

			if tc.discoveryCall != nil {
				subnetsResolver.EXPECT().ResolveViaDiscovery(gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.discoveryCall.subnets, tc.discoveryCall.err)
			}

			if tc.selectorCall != nil {
				subnetsResolver.EXPECT().ResolveViaSelector(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(tc.selectorCall.subnets, tc.selectorCall.err)
			}

			elbv2TaggingManager := elbv2deploy.NewMockTaggingManager(ctrl)
			if tc.listLoadBalancersCall != nil {
				elbv2TaggingManager.EXPECT().ListLoadBalancers(gomock.Any(), gomock.Any()).Return(tc.listLoadBalancersCall.sdkLBs, tc.listLoadBalancersCall.err)
			}

			builder := subnetModelBuilderImpl{
				loadBalancerType:    elbv2model.LoadBalancerTypeNetwork,
				subnetsResolver:     subnetsResolver,
				trackingProvider:    &mockProvider{},
				elbv2TaggingManager: elbv2TaggingManager,
			}

			subnets, err := builder.resolveEC2Subnets(context.Background(), nil, tc.subnetConfig, tc.selector, elbv2model.LoadBalancerSchemeInternal)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, subnets)

		})
	}
}
