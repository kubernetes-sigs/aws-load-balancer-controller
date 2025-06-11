package gateway

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
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
				configResolverFn: tc.configResolverFn,
			}
			tc.setupMocks()
			result, err := r.getLoadBalancerConfigForGateway(context.Background(), k8sClient, k8sFinalizerManager, tc.inputGateway, tc.inputGatewayClass)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
