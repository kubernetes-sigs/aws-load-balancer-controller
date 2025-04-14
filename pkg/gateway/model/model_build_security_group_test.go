package model

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
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
		lbConf          *elbv2gw.LoadBalancerConfiguration
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
			lbConf: &elbv2gw.LoadBalancerConfiguration{
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
			name:            "sg specified - with backend sg",
			enableBackendSg: true,
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: true,
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
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: true,
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
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: true,
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
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: true,
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
			builder := newSecurityGroupBuilder(mockTagger, clusterName, tc.enableBackendSg, mockSgResolver, mockSgProvider, logr.Discard())

			out, err := builder.buildSecurityGroups(context.Background(), stack, tc.lbConf, gw, make(map[int][]routeutils.RouteDescriptor), tc.ipAddressType)

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
		lbConf          *elbv2gw.LoadBalancerConfiguration
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
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{},
			},
			expectedStackResources: 1,
		},
		{
			name:            "sg allocate - with backend sg",
			enableBackendSg: true,
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: true,
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
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					ManageBackendSecurityGroupRules: true,
				},
			},
			providerCall: &backendSgProviderCall{
				err: errors.New("bad thing"),
			},
			expectErr: true,
		},
		{
			name: "sg allocate - tag error",
			lbConf: &elbv2gw.LoadBalancerConfiguration{
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
			builder := newSecurityGroupBuilder(mockTagger, clusterName, tc.enableBackendSg, mockSgResolver, mockSgProvider, logr.Discard())

			out, err := builder.buildSecurityGroups(context.Background(), stack, tc.lbConf, gw, make(map[int][]routeutils.RouteDescriptor), tc.ipAddressType)

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
