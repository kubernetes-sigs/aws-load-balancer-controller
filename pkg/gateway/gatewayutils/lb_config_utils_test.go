package gatewayutils

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
	"time"
)

func Test_ResolveLoadBalancerConfig(t *testing.T) {
	testCases := []struct {
		name      string
		reference *gwv1.ParametersReference
		lbConf    *elbv2gw.LoadBalancerConfiguration
		expectErr bool
	}{
		{
			name: "lb conf found",
			reference: &gwv1.ParametersReference{
				Name:      "foo",
				Namespace: (*gwv1.Namespace)(awssdk.String("ns")),
			},
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "ns",
				},
			},
		},
		{
			name: "no lb conf",
			reference: &gwv1.ParametersReference{
				Name:      "foo",
				Namespace: (*gwv1.Namespace)(awssdk.String("ns")),
			},
			expectErr: true,
		},
		{
			name: "no namespace",
			reference: &gwv1.ParametersReference{
				Name: "foo",
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := testutils.GenerateTestClient()
			if tc.lbConf != nil {
				err := mockClient.Create(context.Background(), tc.lbConf)
				assert.NoError(t, err)
			}
			time.Sleep(1 * time.Second)

			res, err := ResolveLoadBalancerConfig(context.Background(), mockClient, tc.reference)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.lbConf, res)
		})
	}
}

func Test_AddLoadBalancerConfigurationFinalizers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	k8sClient := mock_client.NewMockClient(ctrl)
	k8sFinalizerManager := k8s.NewMockFinalizerManager(ctrl)
	defaultNamespace := gwv1.Namespace("test-ns")
	testNamespace := "test-ns"
	testName := "test-name"

	ctx := context.Background()

	tests := []struct {
		name         string
		gateway      *gwv1.Gateway
		gatewayClass *gwv1.GatewayClass
		setupMocks   func()
		wantErr      bool
	}{
		{
			name: "gateway and gatewayClass have no LB config",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{},
			},
			gatewayClass: &gwv1.GatewayClass{
				Spec: gwv1.GatewayClassSpec{},
			},
			setupMocks: func() {},
			wantErr:    false,
		},
		{
			name:    "gatewayClass has LB config",
			gateway: &gwv1.Gateway{},
			gatewayClass: &gwv1.GatewayClass{
				Spec: gwv1.GatewayClassSpec{
					ParametersRef: &gwv1.ParametersReference{
						Kind:      gwv1.Kind(constants.LoadBalancerConfiguration),
						Name:      testName,
						Namespace: &defaultNamespace,
					},
				},
			},
			setupMocks: func() {
				lbConfig := &elbv2gw.LoadBalancerConfiguration{}
				k8sClient.EXPECT().Get(ctx, types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				}, lbConfig).Return(nil)
				k8sFinalizerManager.EXPECT().AddFinalizers(ctx, lbConfig,
					shared_constants.LoadBalancerConfigurationFinalizer).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "gateway has LB config",
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
				},
				Spec: gwv1.GatewaySpec{
					Infrastructure: &gwv1.GatewayInfrastructure{
						ParametersRef: &gwv1.LocalParametersReference{
							Kind: gwv1.Kind(constants.LoadBalancerConfiguration),
							Name: testName,
						},
					},
				},
			},
			gatewayClass: &gwv1.GatewayClass{},
			setupMocks: func() {
				lbConfig := &elbv2gw.LoadBalancerConfiguration{}
				k8sClient.EXPECT().Get(ctx, types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				}, lbConfig).Return(nil)
				k8sFinalizerManager.EXPECT().AddFinalizers(ctx, lbConfig,
					shared_constants.LoadBalancerConfigurationFinalizer).Return(nil)
			},
			wantErr: false,
		},
		{
			name:    "failed in adding finalizer",
			gateway: &gwv1.Gateway{},
			gatewayClass: &gwv1.GatewayClass{
				Spec: gwv1.GatewayClassSpec{
					ParametersRef: &gwv1.ParametersReference{
						Kind:      gwv1.Kind(constants.LoadBalancerConfiguration),
						Name:      testName,
						Namespace: &defaultNamespace,
					},
				},
			},
			setupMocks: func() {
				lbConfig := &elbv2gw.LoadBalancerConfiguration{}
				k8sClient.EXPECT().Get(ctx, types.NamespacedName{
					Namespace: testNamespace,
					Name:      testName,
				}, lbConfig).Return(nil)
				k8sFinalizerManager.EXPECT().AddFinalizers(ctx, lbConfig,
					shared_constants.LoadBalancerConfigurationFinalizer).Return(fmt.Errorf("test error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()
			err := AddLoadBalancerConfigurationFinalizers(ctx, tt.gateway, tt.gatewayClass, k8sClient, k8sFinalizerManager, "controllerName")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_RemoveLoadBalancerConfigurationFinalizers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	k8sClient := mock_client.NewMockClient(ctrl)
	k8sFinalizerManager := k8s.NewMockFinalizerManager(ctrl)
	ctx := context.Background()
	controllerName := "test-controller"
	testGwName := "test-gw"
	testNamespace := "test-ns"
	testLbConfigName := "test-lb-config"

	tests := []struct {
		name         string
		gateway      *gwv1.Gateway
		gatewayClass *gwv1.GatewayClass
		setupMocks   func()
		wantErr      bool
	}{
		{
			name: "remove finalizer from gateway LB config",
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testGwName,
					Namespace: testNamespace,
				},
				Spec: gwv1.GatewaySpec{
					Infrastructure: &gwv1.GatewayInfrastructure{
						ParametersRef: &gwv1.LocalParametersReference{
							Kind: gwv1.Kind(constants.LoadBalancerConfiguration),
							Name: testLbConfigName,
						},
					},
				},
			},
			gatewayClass: nil,
			setupMocks: func() {
				k8sClient.EXPECT().
					Get(ctx, types.NamespacedName{
						Namespace: testNamespace,
						Name:      testLbConfigName,
					}, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ types.NamespacedName, obj *elbv2gw.LoadBalancerConfiguration, _ ...client.GetOption) error {
						obj.Finalizers = []string{shared_constants.LoadBalancerConfigurationFinalizer}
						return nil
					})

				k8sClient.EXPECT().
					List(ctx, &gwv1.GatewayList{}, gomock.Any()).
					Return(nil)
				k8sClient.EXPECT().
					List(ctx, &gwv1.GatewayClassList{}, gomock.Any()).
					Return(nil)

				k8sFinalizerManager.EXPECT().
					RemoveFinalizers(ctx, gomock.Any(), shared_constants.LoadBalancerConfigurationFinalizer).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "failed in remove finalizer",
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testGwName,
					Namespace: testNamespace,
				},
				Spec: gwv1.GatewaySpec{
					Infrastructure: &gwv1.GatewayInfrastructure{
						ParametersRef: &gwv1.LocalParametersReference{
							Kind: gwv1.Kind(constants.LoadBalancerConfiguration),
							Name: testLbConfigName,
						},
					},
				},
			},
			gatewayClass: nil,
			setupMocks: func() {
				k8sClient.EXPECT().
					Get(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ types.NamespacedName, obj *elbv2gw.LoadBalancerConfiguration, _ ...client.GetOption) error {
						obj.Finalizers = []string{shared_constants.LoadBalancerConfigurationFinalizer}
						return nil
					})

				k8sClient.EXPECT().
					List(ctx, &gwv1.GatewayList{}, gomock.Any()).
					Return(nil)
				k8sClient.EXPECT().
					List(ctx, &gwv1.GatewayClassList{}, gomock.Any()).
					Return(nil)

				k8sFinalizerManager.EXPECT().
					RemoveFinalizers(ctx, gomock.Any(), shared_constants.LoadBalancerConfigurationFinalizer).
					Return(fmt.Errorf("failed to remove finalizer"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()
			err := RemoveLoadBalancerConfigurationFinalizers(ctx, tt.gateway, tt.gatewayClass, k8sClient, k8sFinalizerManager, sets.New(controllerName))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

}

func Test_isLBConfigInUse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	k8sClient := mock_client.NewMockClient(ctrl)
	ctx := context.Background()
	controllerName := "test-controller"
	testNamespace := "test-ns"

	tests := []struct {
		name         string
		lbConfig     *elbv2gw.LoadBalancerConfiguration
		gateway      *gwv1.Gateway
		gatewayClass *gwv1.GatewayClass
		setupMocks   func()
		want         bool
	}{
		{
			name: "LB config not in use",
			lbConfig: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: testNamespace,
				},
			},
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: testNamespace,
				},
			},
			gatewayClass: &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gwclass",
				},
			},
			setupMocks: func() {
				k8sClient.EXPECT().
					Get(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				k8sClient.EXPECT().
					List(gomock.Any(), &gwv1.GatewayList{}, gomock.Any()).
					DoAndReturn(func(_ context.Context, list *gwv1.GatewayList, _ ...client.ListOption) error {
						list.Items = []gwv1.Gateway{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "test-gw",
									Namespace: testNamespace,
								},
							},
						}
						return nil
					}).AnyTimes()

				k8sClient.EXPECT().
					List(gomock.Any(), &gwv1.GatewayClassList{}, gomock.Any()).
					DoAndReturn(func(_ context.Context, list *gwv1.GatewayClassList, _ ...client.ListOption) error {
						list.Items = []gwv1.GatewayClass{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "test-gwclass",
								},
							},
						}
						return nil
					}).AnyTimes()
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()
			got := IsLBConfigInUse(ctx, tt.lbConfig, tt.gateway, tt.gatewayClass, k8sClient, sets.New(controllerName))
			assert.Equal(t, tt.want, got)
		})
	}
}
