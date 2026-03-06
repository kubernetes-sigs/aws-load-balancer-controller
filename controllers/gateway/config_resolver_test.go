package gateway

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_getLoadBalancerConfigForGateway(t *testing.T) {
	mergedConfigName := "mergedConfig"
	gwClassName := "gwclass"
	gwName := "gw"
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	k8sClient := mock_client.NewMockClient(ctrl)
	k8sFinalizerManager := k8s.NewMockFinalizerManager(ctrl)

	testCases := []struct {
		name             string
		configMergeFn    func(gwClassLbConfig elbv2gw.LoadBalancerConfiguration, gwLbConfig elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfiguration
		configResolverFn func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error)

		inputGateway      *gwv1.Gateway
		inputGatewayClass *gwv1.GatewayClass

		resolvedGatewayClassConfig *elbv2gw.LoadBalancerConfiguration
		resolvedGatewayConfig      *elbv2gw.LoadBalancerConfiguration

		setupMocks func()

		expectErr bool
		expected  elbv2gw.LoadBalancerConfiguration
	}{
		{
			name:              "gw class isnt accepted",
			inputGatewayClass: &gwv1.GatewayClass{},
			setupMocks:        func() {},
			expectErr:         true,
		},
		{
			name: "gw class isnt accepted -- condition is missing",
			inputGatewayClass: &gwv1.GatewayClass{
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "foo",
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			setupMocks: func() {},
			expectErr:  true,
		},
		{
			name: "gw class isnt accepted -- condition is explicitly false",
			inputGatewayClass: &gwv1.GatewayClass{
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayClassReasonAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			setupMocks: func() {},
			expectErr:  true,
		},
		{
			name: "gw class accepted -- fail to get gw class config",
			inputGatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gwClassName,
					Annotations: map[string]string{
						"elbv2.k8s.aws/last-processed-config": "1",
					},
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayClassReasonAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
				Spec: gwv1.GatewayClassSpec{
					ParametersRef: &gwv1.ParametersReference{
						Name: gwClassName,
					},
				},
			},
			inputGateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: "ns",
				},
				Spec: gwv1.GatewaySpec{
					Infrastructure: &gwv1.GatewayInfrastructure{
						ParametersRef: &gwv1.LocalParametersReference{
							Name: gwName,
						},
					},
				},
			},
			configResolverFn: func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
				if reference.Name == gwName {
					return nil, errors.New("bad thing")
				}
				return &elbv2gw.LoadBalancerConfiguration{
					ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"},
				}, nil
			},
			setupMocks: func() {
				k8sFinalizerManager.EXPECT().
					AddFinalizers(context.Background(), gomock.Any(), shared_constants.LoadBalancerConfigurationFinalizer).
					Return(nil)
			},
			expectErr: true,
		},
		{
			name: "gw class accepted -- fail to get gw config",
			inputGatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gwClassName,
					Annotations: map[string]string{
						"elbv2.k8s.aws/last-processed-config": "1",
					},
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayClassReasonAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
				Spec: gwv1.GatewayClassSpec{
					ParametersRef: &gwv1.ParametersReference{
						Name: gwClassName,
					},
				},
			},
			inputGateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: "ns",
				},
				Spec: gwv1.GatewaySpec{
					Infrastructure: &gwv1.GatewayInfrastructure{
						ParametersRef: &gwv1.LocalParametersReference{
							Name: gwName,
						},
					},
				},
			},
			configResolverFn: func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
				if reference == nil {
					return nil, nil
				}
				if reference.Name == gwName {
					return nil, errors.New("bad thing")
				}
				return &elbv2gw.LoadBalancerConfiguration{
					ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"},
				}, nil
			},
			setupMocks: func() {
				k8sFinalizerManager.EXPECT().
					AddFinalizers(context.Background(), gomock.Any(), shared_constants.LoadBalancerConfigurationFinalizer).
					Return(nil)
			},
			expectErr: true,
		},
		{
			name: "gw class accepted -- no configs",
			inputGatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gwClassName,
					Annotations: map[string]string{
						"elbv2.k8s.aws/last-processed-config": "1",
					},
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayClassReasonAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			inputGateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: "ns",
				},
			},
			configResolverFn: func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
				if reference == nil {
					return nil, nil
				}
				return nil, errors.New("bad thing")
			},
			setupMocks: func() {},
			expected:   elbv2gw.LoadBalancerConfiguration{},
		},
		{
			name: "gw class accepted -- only gw class configs",
			inputGatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gwClassName,
					Annotations: map[string]string{
						"elbv2.k8s.aws/last-processed-config": "1",
					},
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayClassReasonAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
				Spec: gwv1.GatewayClassSpec{
					ParametersRef: &gwv1.ParametersReference{
						Name: gwClassName,
					},
				},
			},
			inputGateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: "ns",
				},
			},
			configResolverFn: func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
				if reference == nil {
					return nil, nil
				}

				if reference.Name != gwClassName {
					return nil, errors.New("bad thing")
				}
				return &elbv2gw.LoadBalancerConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "gwclass", ResourceVersion: "1"},
				}, nil
			},
			expected: elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "gwclass", ResourceVersion: "1"},
			},
			setupMocks: func() {
				k8sFinalizerManager.EXPECT().
					AddFinalizers(context.Background(), gomock.Any(), shared_constants.LoadBalancerConfigurationFinalizer).
					Return(nil)
			},
		},
		{
			name: "gw class accepted -- only gw config",
			inputGatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gwClassName,
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayClassReasonAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			inputGateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: "ns",
				},
				Spec: gwv1.GatewaySpec{
					Infrastructure: &gwv1.GatewayInfrastructure{
						ParametersRef: &gwv1.LocalParametersReference{
							Name: gwName,
						},
					},
				},
			},
			configResolverFn: func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
				if reference == nil {
					return nil, nil
				}

				if reference.Name != gwName {
					return nil, errors.New("bad thing")
				}
				return &elbv2gw.LoadBalancerConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "gw", ResourceVersion: "1"},
				}, nil
			},
			expected: elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", ResourceVersion: "1"},
			},
			setupMocks: func() {
				k8sFinalizerManager.EXPECT().
					AddFinalizers(context.Background(), gomock.Any(), shared_constants.LoadBalancerConfigurationFinalizer).
					Return(nil)
			},
		},
		{
			name: "gw class accepted -- both gw and gwclass have config - perform merge",
			inputGatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gwClassName,
					Annotations: map[string]string{
						"elbv2.k8s.aws/last-processed-config": "1",
					},
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayClassReasonAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
				Spec: gwv1.GatewayClassSpec{
					ParametersRef: &gwv1.ParametersReference{
						Name: gwClassName,
					},
				},
			},
			inputGateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: "ns",
				},
				Spec: gwv1.GatewaySpec{
					Infrastructure: &gwv1.GatewayInfrastructure{
						ParametersRef: &gwv1.LocalParametersReference{
							Name: gwName,
						},
					},
				},
			},
			configResolverFn: func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
				if reference == nil {
					return nil, nil
				}

				return &elbv2gw.LoadBalancerConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: reference.Name, ResourceVersion: "1"},
				}, nil
			},
			expected: elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: mergedConfigName},
			},
			setupMocks: func() {
				k8sFinalizerManager.EXPECT().
					AddFinalizers(context.Background(), gomock.Any(), shared_constants.LoadBalancerConfigurationFinalizer).
					Return(nil).Times(2)
			},
		},
		{
			name: "gw class accepted -- but processed config version has a mismatch",
			inputGatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: gwClassName,
					Annotations: map[string]string{
						"elbv2.k8s.aws/last-processed-config": "3",
					},
				},
				Status: gwv1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayClassReasonAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
				Spec: gwv1.GatewayClassSpec{
					ParametersRef: &gwv1.ParametersReference{
						Name: gwClassName,
					},
				},
			},
			inputGateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwName,
					Namespace: "ns",
				},
			},
			configResolverFn: func(ctx context.Context, k8sClient client.Client, reference *gwv1.ParametersReference) (*elbv2gw.LoadBalancerConfiguration, error) {
				if reference == nil {
					return nil, nil
				}

				if reference.Name != gwClassName {
					return nil, errors.New("bad thing")
				}
				return &elbv2gw.LoadBalancerConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "gwclass", ResourceVersion: "1"},
				}, nil
			},
			setupMocks: func() {
				k8sFinalizerManager.EXPECT().
					AddFinalizers(context.Background(), gomock.Any(), shared_constants.LoadBalancerConfigurationFinalizer).
					Return(nil)
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &gatewayConfigResolverImpl{
				configMergeFn: func(gwClassLbConfig elbv2gw.LoadBalancerConfiguration, gwLbConfig elbv2gw.LoadBalancerConfiguration) elbv2gw.LoadBalancerConfiguration {
					mergedConfig := elbv2gw.LoadBalancerConfiguration{
						ObjectMeta: metav1.ObjectMeta{Name: mergedConfigName},
					}
					return mergedConfig
				},
				configResolverFn:    tc.configResolverFn,
				tgConfigConstructor: gateway.NewTargetGroupConfigConstructor(),
				logger:              logr.Discard(),
			}
			tc.setupMocks()
			result, _, err := r.getLoadBalancerConfigForGateway(context.Background(), k8sClient, k8sFinalizerManager, tc.inputGateway, tc.inputGatewayClass)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_resolveAndMergeDefaultTGCs(t *testing.T) {
	ipTargetType := elbv2gw.TargetTypeIP
	instanceTargetType := elbv2gw.TargetTypeInstance
	preferGW := elbv2gw.MergeModePreferGateway
	preferGWClass := elbv2gw.MergeModePreferGatewayClass

	testCases := []struct {
		name        string
		gwClassLBC  *elbv2gw.LoadBalancerConfiguration
		gwLBC       *elbv2gw.LoadBalancerConfiguration
		storedTGCs  []elbv2gw.TargetGroupConfiguration
		expectedNil bool
		expectedTT  *elbv2gw.TargetType
		expectedHC  *string
	}{
		{
			name:        "both LBCs nil",
			gwClassLBC:  nil,
			gwLBC:       nil,
			expectedNil: true,
		},
		{
			name: "only GatewayClass LBC has default TGC",
			gwClassLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "class-tgc"},
				},
			},
			gwLBC: nil,
			storedTGCs: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "class-tgc", Namespace: "kube-system"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						DefaultConfiguration: elbv2gw.TargetGroupProps{TargetType: &ipTargetType},
					},
				},
			},
			expectedTT: &ipTargetType,
		},
		{
			name:       "only Gateway LBC has default TGC, same namespace as GW",
			gwClassLBC: nil,
			gwLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "team-a"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "gw-tgc"},
				},
			},
			storedTGCs: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-tgc", Namespace: "team-a"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						DefaultConfiguration: elbv2gw.TargetGroupProps{TargetType: &instanceTargetType},
					},
				},
			},
			expectedTT: &instanceTargetType,
		},
		{
			name:       "Gateway LBC references TGC not found — returns error",
			gwClassLBC: nil,
			gwLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-lbc", Namespace: "team-a"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "nonexistent-tgc"},
				},
			},
			storedTGCs:  []elbv2gw.TargetGroupConfiguration{},
			expectedNil: true,
		},
		{
			name: "both have default TGC, prefer-gateway — GW TGC targetType wins, class TGC healthCheck fills gap",
			gwClassLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					MergingMode:                     &preferGW,
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "class-tgc"},
				},
			},
			gwLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "team-a"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "gw-tgc"},
				},
			},
			storedTGCs: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "class-tgc", Namespace: "kube-system"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						DefaultConfiguration: elbv2gw.TargetGroupProps{
							TargetType: &ipTargetType,
							HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
								HealthCheckPath: strPtr("/class-health"),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-tgc", Namespace: "team-a"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						DefaultConfiguration: elbv2gw.TargetGroupProps{
							TargetType: &instanceTargetType,
						},
					},
				},
			},
			expectedTT: &instanceTargetType,
			expectedHC: strPtr("/class-health"),
		},
		{
			name: "both have default TGC, prefer-gateway-class — class TGC targetType wins",
			gwClassLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					MergingMode:                     &preferGWClass,
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "class-tgc"},
				},
			},
			gwLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: "team-a"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "gw-tgc"},
				},
			},
			storedTGCs: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "class-tgc", Namespace: "kube-system"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						DefaultConfiguration: elbv2gw.TargetGroupProps{
							TargetType: &ipTargetType,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-tgc", Namespace: "team-a"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						DefaultConfiguration: elbv2gw.TargetGroupProps{
							TargetType: &instanceTargetType,
							HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
								HealthCheckPath: strPtr("/gw-health"),
							},
						},
					},
				},
			},
			expectedTT: &ipTargetType,
			expectedHC: strPtr("/gw-health"),
		},
		{
			name: "TGC referenced by GatewayClass LBC does not exist — returns error",
			gwClassLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "class-lbc", Namespace: "kube-system"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "nonexistent"},
				},
			},
			gwLBC: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-lbc", Namespace: "team-a"},
				Spec: elbv2gw.LoadBalancerConfigurationSpec{
					DefaultTargetGroupConfiguration: &elbv2gw.DefaultTargetGroupConfigurationReference{Name: "gw-tgc"},
				},
			},
			storedTGCs: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-tgc", Namespace: "team-a"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						DefaultConfiguration: elbv2gw.TargetGroupProps{TargetType: &instanceTargetType},
					},
				},
			},
			expectedNil: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testutils.GenerateTestClient()

			for _, tgc := range tc.storedTGCs {
				c := tgc.DeepCopy()
				err := k8sClient.Create(context.Background(), c)
				assert.NoError(t, err)
			}

			resolver := &gatewayConfigResolverImpl{
				tgConfigConstructor: gateway.NewTargetGroupConfigConstructor(),
				logger:              logr.Discard(),
			}

			result, err := resolver.resolveAndMergeDefaultTGCs(context.Background(), k8sClient, tc.gwClassLBC, tc.gwLBC)

			if tc.expectedNil {
				if err != nil {
					assert.Nil(t, result)
					return
				}
				assert.Nil(t, result)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tc.expectedTT, result.Spec.DefaultConfiguration.TargetType)

			if tc.expectedHC != nil {
				assert.NotNil(t, result.Spec.DefaultConfiguration.HealthCheckConfig)
				assert.Equal(t, tc.expectedHC, result.Spec.DefaultConfiguration.HealthCheckConfig.HealthCheckPath)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
