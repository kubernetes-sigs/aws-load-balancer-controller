package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_targetgroupConfigurationReconciler_updateStatus(t *testing.T) {
	k8sSchema := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(k8sSchema)
	_ = elbv2gw.AddToScheme(k8sSchema)

	tgConf := &elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-tgc",
			Namespace:  "default",
			Generation: 10,
		},
	}

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sSchema).
		WithStatusSubresource(&elbv2gw.TargetGroupConfiguration{}).
		WithObjects(tgConf).
		Build()

	r := &targetgroupConfigurationReconciler{
		k8sClient: k8sClient,
	}

	ctx := context.Background()
	// Initial update
	err := r.updateStatus(ctx, tgConf)
	assert.NoError(t, err)

	updatedTgConf := &elbv2gw.TargetGroupConfiguration{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-tgc", Namespace: "default"}, updatedTgConf)
	assert.NoError(t, err)

	assert.NotNil(t, updatedTgConf.Status.ObservedGeneration)
	assert.Equal(t, int64(10), *updatedTgConf.Status.ObservedGeneration)

	// Update again with same generation - should return nil and do nothing
	err = r.updateStatus(ctx, updatedTgConf)
	assert.NoError(t, err)
}
