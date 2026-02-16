package model

import (
	"context"
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_BuildSecurityGroups_Specified(t *testing.T) {
	const clusterName = "my-cluster"

	type resolveSgCall struct {
		securityGroups []string
		err            error
	}

	type backendSgProviderCall struct {
		sgId string
		err  error
	}

	testCases := []struct {
		name            string
		lbConf          elbv2gw.LoadBalancerConfiguration
		lbType          elbv2model.LoadBalancerType
		ipAddressType   elbv2model.IPAddressType
		expectedTags    map[string]string
		tagErr          error
		enableBackendSg bool

		resolveSg    *resolveSgCall
		providerCall *backendSgProviderCall

		expectErr              bool
		expectedBackendSgToken coremodel.StringToken
		expectedSgTokens       []coremodel.StringToken
		backendSgAllocated     bool
	}{
		{
			name: "sg specified - no backend sg",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SecurityGroups: &[]string{
						"sg1",
						"sg2",
					},
				},
			},
			resolveSg: &resolveSgCall{
				securityGroups: []string{
					"sg1",
					"sg2",
				},
			},
			expectedSgTokens: []coremodel.StringToken{
				coremodel.LiteralStringToken("sg1"),
				coremodel.LiteralStringToken("sg2"),
			},
		},
		{
			name: "sg disabled - nlb",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DisableSecurityGroup: awssdk.Bool(true),
				},
			},
			lbType: elbv2model.LoadBalancerTypeNetwork,
		},
		{
			name: "sg disabled - alb",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DisableSecurityGroup: awssdk.Bool(true),
					SecurityGroups: &[]string{
						"sg1",
						"sg2",
					},
				},
			},
			lbType: elbv2model.LoadBalancerTypeApplication,
			resolveSg: &resolveSgCall{
				securityGroups: []string{
					"sg1",
					"sg2",
				},
			},
			expectedSgTokens: []coremodel.StringToken{
				coremodel.LiteralStringToken("sg1"),
				coremodel.LiteralStringToken("sg2"),
			},
		},
		{
			name:            "sg specified - with backend sg",
			enableBackendSg: true,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
					SecurityGroups: &[]string{
						"sg1",
						"sg2",
					},
				},
			},
			resolveSg: &resolveSgCall{
				securityGroups: []string{
					"sg1",
					"sg2",
				},
			},
			providerCall: &backendSgProviderCall{
				sgId: "auto-allocated",
			},
			expectedSgTokens: []coremodel.StringToken{
				coremodel.LiteralStringToken("sg1"),
				coremodel.LiteralStringToken("sg2"),
				coremodel.LiteralStringToken("auto-allocated"),
			},
			expectedBackendSgToken: coremodel.LiteralStringToken("auto-allocated"),
			backendSgAllocated:     true,
		},
		{
			name: "sg specified - with backend sg - error - backendsg not enabled",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
					SecurityGroups: &[]string{
						"sg1",
						"sg2",
					},
				},
			},
			resolveSg: &resolveSgCall{
				securityGroups: []string{
					"sg1",
					"sg2",
				},
			},
			expectErr: true,
		},
		{
			name:            "sg specified - with backend sg - error - resolve sg error",
			enableBackendSg: true,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
					SecurityGroups: &[]string{
						"sg1",
						"sg2",
					},
				},
			},
			resolveSg: &resolveSgCall{
				err: errors.New("bad thing"),
			},
			expectErr: true,
		},
		{
			name:            "sg specified - with backend sg - error - resolve sg error",
			enableBackendSg: true,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
					SecurityGroups: &[]string{
						"sg1",
						"sg2",
					},
				},
			},
			resolveSg: &resolveSgCall{
				securityGroups: []string{
					"sg1",
					"sg2",
				},
			},
			providerCall: &backendSgProviderCall{
				err: errors.New("bad thing"),
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTagger := &mockTagHelper{
				tags: tc.expectedTags,
				err:  tc.tagErr,
			}

			gw := &gwv1.Gateway{}
			gw.Name = "my-gw"
			gw.Namespace = "my-namespace"

			mockSgProvider := networking.NewMockBackendSGProvider(ctrl)
			mockSgResolver := networking.NewMockSecurityGroupResolver(ctrl)

			if tc.resolveSg != nil {
				mockSgResolver.EXPECT().ResolveViaNameOrID(gomock.Any(), gomock.Eq(*tc.lbConf.Spec.SecurityGroups)).Return(tc.resolveSg.securityGroups, tc.resolveSg.err).Times(1)
			}

			if tc.providerCall != nil {
				mockSgProvider.EXPECT().Get(gomock.Any(), gomock.Eq(networking.ResourceType(networking.ResourceTypeGateway)), gomock.Eq([]types.NamespacedName{k8s.NamespacedName(gw)})).Return(tc.providerCall.sgId, tc.providerCall.err).Times(1)
			}

			stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
			builder := newSecurityGroupBuilder(mockTagger, clusterName, tc.lbType, tc.enableBackendSg, mockSgResolver, mockSgProvider, logr.Discard())

			out, err := builder.buildSecurityGroups(context.Background(), stack, tc.lbConf, gw, gw.Spec.Listeners, tc.ipAddressType)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedBackendSgToken, out.backendSecurityGroupToken)
			assert.Equal(t, tc.expectedSgTokens, out.securityGroupTokens)
			assert.Equal(t, tc.backendSgAllocated, out.backendSecurityGroupAllocated)
		})
	}
}

func Test_BuildSecurityGroups_Allocate(t *testing.T) {
	const clusterName = "my-cluster"

	type backendSgProviderCall struct {
		sgId string
		err  error
	}

	testCases := []struct {
		name            string
		lbConf          elbv2gw.LoadBalancerConfiguration
		ipAddressType   elbv2model.IPAddressType
		expectedTags    map[string]string
		tagErr          error
		enableBackendSg bool

		providerCall *backendSgProviderCall

		expectErr              bool
		hasBackendSg           bool
		backendSgAllocated     bool
		expectedStackResources int
	}{
		{
			name: "sg allocate - no backend sg",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
			expectedStackResources: 1,
		},
		{
			name:            "sg allocate - with backend sg",
			enableBackendSg: true,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
				},
			},
			providerCall: &backendSgProviderCall{
				sgId: "auto-allocated",
			},
			backendSgAllocated:     true,
			expectedStackResources: 1,
		},
		{
			name:            "sg allocate - provider error",
			enableBackendSg: true,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: awssdk.Bool(true),
				},
			},
			providerCall: &backendSgProviderCall{
				err: errors.New("bad thing"),
			},
			expectErr: true,
		},
		{
			name: "sg allocate - tag error",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
			expectErr: true,
			tagErr:    errors.New("bad thing"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTagger := &mockTagHelper{
				tags: tc.expectedTags,
				err:  tc.tagErr,
			}

			gw := &gwv1.Gateway{}
			gw.Name = "my-gw"
			gw.Namespace = "my-namespace"

			mockSgProvider := networking.NewMockBackendSGProvider(ctrl)
			mockSgResolver := networking.NewMockSecurityGroupResolver(ctrl)

			if tc.providerCall != nil {
				mockSgProvider.EXPECT().Get(gomock.Any(), gomock.Eq(networking.ResourceType(networking.ResourceTypeGateway)), gomock.Eq([]types.NamespacedName{k8s.NamespacedName(gw)})).Return(tc.providerCall.sgId, tc.providerCall.err).Times(1)
			}

			stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
			builder := newSecurityGroupBuilder(mockTagger, clusterName, elbv2model.LoadBalancerTypeApplication, tc.enableBackendSg, mockSgResolver, mockSgProvider, logr.Discard())

			out, err := builder.buildSecurityGroups(context.Background(), stack, tc.lbConf, gw, gw.Spec.Listeners, tc.ipAddressType)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.backendSgAllocated, out.backendSecurityGroupAllocated)
			var resSGs []*ec2model.SecurityGroup
			listErr := stack.ListResources(&resSGs)
			assert.NoError(t, listErr)
			assert.Equal(t, tc.expectedStackResources, len(resSGs))
			if tc.hasBackendSg {
				assert.NotNil(t, out.backendSecurityGroupToken)
			}
		})
	}
}

func Test_BuildSecurityGroups_BuildManagedSecurityGroupIngressPermissions(t *testing.T) {
	testCases := []struct {
		name          string
		lbConf        elbv2gw.LoadBalancerConfiguration
		ipAddressType elbv2model.IPAddressType
		gateway       *gwv1.Gateway
		expected      []ec2model.IPPermission
	}{
		{
			name: "ipv4 - tcp - with default source ranges",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
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
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "0.0.0.0/0",
						},
					},
				},
			},
		},
		{
			name: "ipv4 - tcp - with source range",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"127.0.0.1/24",
						"127.100.0.1/24",
						"127.200.0.1/24",
					},
				},
			},
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tcp",
							Port:     80,
							Protocol: gwv1.TCPProtocolType,
						},
					},
				},
			},
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.100.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.200.0.1/24",
						},
					},
				},
			},
		},
		{
			name: "ipv4 - udp - with source range",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"127.0.0.1/24",
						"127.100.0.1/24",
						"127.200.0.1/24",
					},
				},
			},
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "udp",
							Port:     80,
							Protocol: gwv1.UDPProtocolType,
						},
					},
				},
			},
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "udp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "udp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.100.0.1/24",
						},
					},
				},
				{
					IPProtocol: "udp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.200.0.1/24",
						},
					},
				},
			},
		},
		{
			name: "ipv4 - udp - with source range - icmp enabled",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"127.0.0.1/24",
					},
					EnableICMP: awssdk.Bool(true),
				},
			},
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "udp",
							Port:     80,
							Protocol: gwv1.UDPProtocolType,
						},
					},
				},
			},
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "udp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "icmp",
					FromPort:   awssdk.Int32(2),
					ToPort:     awssdk.Int32(3),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
			},
		},
		{
			name: "ipv4 - with multiple l7 listeners - with source range - multiple ports",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"127.0.0.1/24",
						"127.100.0.1/24",
						"127.200.0.1/24",
					},
				},
			},
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "http-1",
							Port:     80,
							Protocol: gwv1.HTTPProtocolType,
						},
						{
							Name:     "http-2",
							Port:     8080,
							Protocol: gwv1.HTTPSProtocolType,
						},
					},
				},
			},
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.100.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.200.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(8080),
					ToPort:     awssdk.Int32(8080),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(8080),
					ToPort:     awssdk.Int32(8080),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.100.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(8080),
					ToPort:     awssdk.Int32(8080),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.200.0.1/24",
						},
					},
				},
			},
		},
		{
			name: "ipv4 - with multiple l4 listeners type - with source range - multiple ports",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"127.0.0.1/24",
						"127.100.0.1/24",
						"127.200.0.1/24",
					},
				},
			},
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tcp-1",
							Port:     80,
							Protocol: gwv1.TCPProtocolType,
						},
						{
							Name:     "udp-1",
							Port:     85,
							Protocol: gwv1.UDPProtocolType,
						},
						{
							Name:     "tls-1",
							Port:     90,
							Protocol: gwv1.TLSProtocolType,
						},
					},
				},
			},
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.100.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.200.0.1/24",
						},
					},
				},
				{
					IPProtocol: "udp",
					FromPort:   awssdk.Int32(85),
					ToPort:     awssdk.Int32(85),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "udp",
					FromPort:   awssdk.Int32(85),
					ToPort:     awssdk.Int32(85),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.100.0.1/24",
						},
					},
				},
				{
					IPProtocol: "udp",
					FromPort:   awssdk.Int32(85),
					ToPort:     awssdk.Int32(85),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.200.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(90),
					ToPort:     awssdk.Int32(90),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(90),
					ToPort:     awssdk.Int32(90),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.100.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(90),
					ToPort:     awssdk.Int32(90),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.200.0.1/24",
						},
					},
				},
			},
		},
		{
			name:          "ipv6 - with default source ranges",
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
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
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "0.0.0.0/0",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: "::/0",
						},
					},
				},
			},
		},
		{
			name:          "ipv6 - with source range",
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"2001:db8::/32",
					},
				},
			},
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name:     "tcp",
							Port:     80,
							Protocol: gwv1.TCPProtocolType,
						},
						{
							Name:     "udp",
							Port:     85,
							Protocol: gwv1.UDPProtocolType,
						},
						{
							Name:     "tls",
							Port:     90,
							Protocol: gwv1.TLSProtocolType,
						},
					},
				},
			},
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: "2001:db8::/32",
						},
					},
				},
				{
					IPProtocol: "udp",
					FromPort:   awssdk.Int32(85),
					ToPort:     awssdk.Int32(85),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: "2001:db8::/32",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(90),
					ToPort:     awssdk.Int32(90),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: "2001:db8::/32",
						},
					},
				},
			},
		},
		{
			name:          "ipv6 + ipv4 - with source range",
			ipAddressType: elbv2model.IPAddressTypeDualStack,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"2001:db8::/32",
						"127.0.0.1/24",
					},
				},
			},
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
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: "2001:db8::/32",
						},
					},
				},
			},
		},
		{
			name:          "ipv6 + ipv4 - with source range - but lb type doesnt support ipv6",
			ipAddressType: elbv2model.IPAddressTypeIPV4,
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"2001:db8::/32",
						"127.0.0.1/24",
					},
				},
			},
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
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
			},
		},
		{
			name: "prefix list",
			lbConf: elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					SourceRanges: &[]string{
						"127.0.0.1/24",
					},
					SecurityGroupPrefixes: &[]string{"pl1", "pl2"},
				},
			},
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
			expected: []ec2model.IPPermission{
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "127.0.0.1/24",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: "pl1",
						},
					},
				},
				{
					IPProtocol: "tcp",
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(80),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: "pl2",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := &securityGroupBuilderImpl{}
			permissions := builder.buildManagedSecurityGroupIngressPermissions(tc.lbConf, tc.gateway.Spec.Listeners, tc.ipAddressType)
			assert.ElementsMatch(t, tc.expected, permissions, fmt.Sprintf("%+v", permissions))
		})
	}
}
