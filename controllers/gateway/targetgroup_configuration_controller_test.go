package gateway

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

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
			Finalizers: []string{shared_constants.TargetGroupConfigurationFinalizer},
		},
		Spec: elbv2gw.TargetGroupConfigurationSpec{
			TargetReference: &elbv2gw.Reference{
				Name: "chained-gateway",
				Kind: &targetKind,
			},
		},
	}
	routeKind := gwalpha2.Kind(targetReferenceKindGateway)
	route := &gwalpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcp-route",
			Namespace: "test-ns",
		},
		Spec: gwalpha2.TCPRouteSpec{
			Rules: []gwalpha2.TCPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{
							BackendObjectReference: gwalpha2.BackendObjectReference{
								Name: "chained-gateway",
								Kind: &routeKind,
							},
						},
					},
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
			Finalizers: []string{shared_constants.TargetGroupConfigurationFinalizer},
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
		RemoveFinalizers(context.Background(), tgConf, shared_constants.TargetGroupConfigurationFinalizer).
		Return(nil)

	r := &targetgroupConfigurationReconciler{
		k8sClient:        k8sClient,
		logger:           logr.Discard(),
		finalizerManager: finalizerManager,
		gwRetrieveFn: func(ctx context.Context, k8sClient client.Client, gwController string) ([]*gwv1.Gateway, error) {
			return nil, nil
		},
	}

	err := r.handleDelete(tgConf)
	assert.NoError(t, err)
}
