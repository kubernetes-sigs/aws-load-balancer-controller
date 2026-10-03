package gateway

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const testTargetGroupConfigurationFinalizer = "test-finalizer"

func TestTargetGroupConfigurationReconciler_handleDelete_GatewayTargetStillInUse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	k8sClient := testutils.GenerateTestClient()
	finalizerManager := k8s.NewMockFinalizerManager(ctrl)

	targetKind := targetReferenceKindGateway
	tgConf := &elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gateway-tgc",
			Namespace:  "test-ns",
			Finalizers: []string{testTargetGroupConfigurationFinalizer},
		},
		Spec: elbv2gw.TargetGroupConfigurationSpec{
			TargetReference: &elbv2gw.Reference{
				Name: "chained-gateway",
				Kind: &targetKind,
			},
		},
	}
	routeKind := gwv1.Kind(targetReferenceKindGateway)
	route := &gwv1.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-route",
			Namespace: "test-ns",
		},
		Spec: gwv1.TCPRouteSpec{
			Rules: []gwv1.TCPRouteRule{
				{
					BackendRefs: []gwv1.BackendRef{
						{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: "chained-gateway",
								Kind: &routeKind,
							},
						},
					},
				},
				{
					BackendRefs: []gwv1.BackendRef{{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name: "chained-gateway",
							Kind: &routeKind,
						},
					}},
				},
			},
		},
	}

	assert.NoError(t, k8sClient.Create(context.Background(), tgConf))
	assert.NoError(t, k8sClient.Create(context.Background(), route))

	r := &targetgroupConfigurationReconciler{
		k8sClient:        k8sClient,
		logger:           logr.Discard(),
		finalizerManager: finalizerManager,
		finalizer:        testTargetGroupConfigurationFinalizer,
		gwRetrieveFn: func(ctx context.Context, k8sClient client.Client, gwController string) ([]*gwv1.Gateway, error) {
			return nil, nil
		},
	}

	err := r.handleDelete(tgConf)
	assert.EqualError(t, err, "targetgroup configuration [test-ns/gateway-tgc] is still in use by TCPRoutes [test-ns/tcp-route]")
}

func TestTargetGroupConfigurationReconciler_handleDelete_GatewayTargetNotInUse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	k8sClient := testutils.GenerateTestClient()
	finalizerManager := k8s.NewMockFinalizerManager(ctrl)

	targetKind := targetReferenceKindGateway
	tgConf := &elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gateway-tgc",
			Namespace:  "test-ns",
			Finalizers: []string{testTargetGroupConfigurationFinalizer},
		},
		Spec: elbv2gw.TargetGroupConfigurationSpec{
			TargetReference: &elbv2gw.Reference{
				Name: "chained-gateway",
				Kind: &targetKind,
			},
		},
	}

	assert.NoError(t, k8sClient.Create(context.Background(), tgConf))

	finalizerManager.EXPECT().
		RemoveFinalizers(context.Background(), tgConf, testTargetGroupConfigurationFinalizer).
		Return(nil)

	r := &targetgroupConfigurationReconciler{
		k8sClient:        k8sClient,
		logger:           logr.Discard(),
		finalizerManager: finalizerManager,
		finalizer:        testTargetGroupConfigurationFinalizer,
		gwRetrieveFn: func(ctx context.Context, k8sClient client.Client, gwController string) ([]*gwv1.Gateway, error) {
			return nil, nil
		},
	}

	err := r.handleDelete(tgConf)
	assert.NoError(t, err)
}

func crossNamespaceGatewayTGCFixtures() (*elbv2gw.TargetGroupConfiguration, *gwv1.TCPRoute) {
	targetKind := targetReferenceKindGateway
	tgConf := &elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gateway-tgc",
			Namespace:  "alb-ns",
			Finalizers: []string{testTargetGroupConfigurationFinalizer},
		},
		Spec: elbv2gw.TargetGroupConfigurationSpec{
			TargetReference: &elbv2gw.Reference{
				Name: "chained-gateway",
				Kind: &targetKind,
			},
		},
	}

	routeKind := gwv1.Kind(targetReferenceKindGateway)
	backendNamespace := gwv1.Namespace("alb-ns")
	route := &gwv1.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-route",
			Namespace: "nlb-ns",
		},
		Spec: gwv1.TCPRouteSpec{
			Rules: []gwv1.TCPRouteRule{
				{
					BackendRefs: []gwv1.BackendRef{
						{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name:      "chained-gateway",
								Kind:      &routeKind,
								Namespace: &backendNamespace,
							},
						},
					},
				},
			},
		},
	}
	return tgConf, route
}

// A TCPRoute in another namespace that no ReferenceGrant permits provisions nothing, so it must not
// keep the finalizer. Otherwise any namespace could block deletion of this TGC and its namespace.
func TestTargetGroupConfigurationReconciler_handleDelete_GatewayTargetCrossNamespaceWithoutReferenceGrant(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	k8sClient := testutils.GenerateTestClient()
	finalizerManager := k8s.NewMockFinalizerManager(ctrl)

	tgConf, route := crossNamespaceGatewayTGCFixtures()
	assert.NoError(t, k8sClient.Create(context.Background(), tgConf))
	assert.NoError(t, k8sClient.Create(context.Background(), route))

	finalizerManager.EXPECT().
		RemoveFinalizers(context.Background(), tgConf, testTargetGroupConfigurationFinalizer).
		Return(nil)

	r := &targetgroupConfigurationReconciler{
		k8sClient:        k8sClient,
		logger:           logr.Discard(),
		finalizerManager: finalizerManager,
		finalizer:        testTargetGroupConfigurationFinalizer,
		gwRetrieveFn: func(ctx context.Context, k8sClient client.Client, gwController string) ([]*gwv1.Gateway, error) {
			return nil, nil
		},
	}

	err := r.handleDelete(tgConf)
	assert.NoError(t, err)
}

// With a permitting ReferenceGrant the reference is effective, so deletion must still be blocked.
func TestTargetGroupConfigurationReconciler_handleDelete_GatewayTargetCrossNamespaceWithReferenceGrant(t *testing.T) {
	for _, tt := range []struct {
		name    string
		toName  *gwv1.ObjectName
		blocked bool
	}{
		{
			name:    "grant naming the gateway",
			toName:  (*gwv1.ObjectName)(awssdk.String("chained-gateway")),
			blocked: true,
		},
		{
			name:    "grant with no name acts as a wildcard",
			toName:  nil,
			blocked: true,
		},
		{
			name:    "grant naming a different gateway does not apply",
			toName:  (*gwv1.ObjectName)(awssdk.String("other-gateway")),
			blocked: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sClient := testutils.GenerateTestClient()
			finalizerManager := k8s.NewMockFinalizerManager(ctrl)

			tgConf, route := crossNamespaceGatewayTGCFixtures()
			grant := &gwv1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "allow-nlb-ns",
					Namespace: "alb-ns",
				},
				Spec: gwv1.ReferenceGrantSpec{
					From: []gwv1.ReferenceGrantFrom{
						{
							Group:     gatewayAPIGroup,
							Kind:      gwv1.Kind(routeutils.TCPRouteKind),
							Namespace: "nlb-ns",
						},
					},
					To: []gwv1.ReferenceGrantTo{
						{
							Group: gatewayAPIGroup,
							Kind:  targetReferenceKindGateway,
							Name:  tt.toName,
						},
					},
				},
			}

			assert.NoError(t, k8sClient.Create(context.Background(), tgConf))
			assert.NoError(t, k8sClient.Create(context.Background(), route))
			assert.NoError(t, k8sClient.Create(context.Background(), grant))

			if !tt.blocked {
				finalizerManager.EXPECT().
					RemoveFinalizers(context.Background(), tgConf, testTargetGroupConfigurationFinalizer).
					Return(nil)
			}

			r := &targetgroupConfigurationReconciler{
				k8sClient:        k8sClient,
				logger:           logr.Discard(),
				finalizerManager: finalizerManager,
				finalizer:        testTargetGroupConfigurationFinalizer,
				gwRetrieveFn: func(ctx context.Context, k8sClient client.Client, gwController string) ([]*gwv1.Gateway, error) {
					return nil, nil
				},
			}

			err := r.handleDelete(tgConf)
			if tt.blocked {
				assert.EqualError(t, err, "targetgroup configuration [alb-ns/gateway-tgc] is still in use by TCPRoutes [nlb-ns/tcp-route]")
				return
			}
			assert.NoError(t, err)
		})
	}
}
