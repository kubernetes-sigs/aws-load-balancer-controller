/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
)

// --- Mocks ---

type mockDeferredReconciler struct{}

func (m *mockDeferredReconciler) Enqueue(tgb *elbv2api.TargetGroupBinding)       {}
func (m *mockDeferredReconciler) MarkProcessed(tgb *elbv2api.TargetGroupBinding) {}
func (m *mockDeferredReconciler) Run()                                           {}

type mockMetricCollector struct{}

func (m *mockMetricCollector) ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration) {
}
func (m *mockMetricCollector) ObserveQUICTargetMissingServerId(namespace string, tgbName string) {}
func (m *mockMetricCollector) ObserveControllerReconcileError(controller string, errorType string) {
}
func (m *mockMetricCollector) ObserveControllerReconcileLatency(controller string, stage string, fn func()) {
	fn()
}
func (m *mockMetricCollector) ObserveWebhookValidationError(webhookName string, errorType string) {}
func (m *mockMetricCollector) ObserveWebhookMutationError(webhookName string, errorType string)   {}
func (m *mockMetricCollector) StartCollectTopTalkers(ctx context.Context)                         {}
func (m *mockMetricCollector) StartCollectCacheSize(ctx context.Context)                          {}

// --- Test ---

func TestTargetGroupBindingReconciler_Delete_Stuck(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	elbv2api.AddToScheme(scheme)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-tgb",
			Namespace:  "default",
			Finalizers: []string{"elbv2.k8s.aws/resources"},
			DeletionTimestamp: &metav1.Time{
				Time: time.Now(),
			},
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/test/123",
		},
	}

	k8sClient := testclient.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(tgb).WithRuntimeObjects(tgb).Build()

	// Setup Mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFinalizerManager := k8s.NewMockFinalizerManager(ctrl)
	mockFinalizerManager.EXPECT().RemoveFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	mockResMgr := targetgroupbinding.NewMockResourceManager(ctrl)
	mockResMgr.EXPECT().Cleanup(gomock.Any(), gomock.Any()).Return(errors.New("cleanup failed due to AWS error"))

	reconciler := &targetGroupBindingReconciler{
		k8sClient:                            k8sClient,
		eventRecorder:                        record.NewFakeRecorder(10),
		finalizerManager:                     mockFinalizerManager,
		tgbResourceManager:                   mockResMgr,
		deferredTargetGroupBindingReconciler: &mockDeferredReconciler{},
		logger:                               log.Log.WithName("controllers").WithName("TargetGroupBinding"),
		metricsCollector:                     &mockMetricCollector{},
		reconcileCounters:                    metricsutil.NewReconcileCounters(),
	}

	req := reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: "default",
			Name:      "test-tgb",
		},
	}

	// EXECUTE
	_, err := reconciler.Reconcile(context.Background(), req)

	// VERIFY
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup failed due to AWS error")

	// Verify Status
	updatedTGB := &elbv2api.TargetGroupBinding{}
	err = k8sClient.Get(context.Background(), req.NamespacedName, updatedTGB)
	assert.NoError(t, err)

	assert.NotEmpty(t, updatedTGB.Status.Conditions)
	if len(updatedTGB.Status.Conditions) > 0 {
		condition := updatedTGB.Status.Conditions[0]
		assert.Equal(t, "Ready", condition.Type)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
		assert.Equal(t, "FailedCleanup", condition.Reason)
		assert.Equal(t, "cleanup failed due to AWS error", condition.Message)
	}
}

func TestTargetGroupBindingReconciler_Reconcile_FailedReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	elbv2api.AddToScheme(scheme)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tgb",
			Namespace: "default",
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/test/123",
		},
	}

	k8sClient := testclient.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(tgb).WithRuntimeObjects(tgb).Build()

	// Setup Mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFinalizerManager := k8s.NewMockFinalizerManager(ctrl)
	mockFinalizerManager.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	mockResMgr := targetgroupbinding.NewMockResourceManager(ctrl)
	mockResMgr.EXPECT().Reconcile(gomock.Any(), gomock.Any()).Return(false, errors.New("target group not found"))

	fakeRecorder := record.NewFakeRecorder(10)

	reconciler := &targetGroupBindingReconciler{
		k8sClient:                            k8sClient,
		eventRecorder:                        fakeRecorder,
		finalizerManager:                     mockFinalizerManager,
		tgbResourceManager:                   mockResMgr,
		deferredTargetGroupBindingReconciler: &mockDeferredReconciler{},
		logger:                               log.Log.WithName("controllers").WithName("TargetGroupBinding"),
		metricsCollector:                     &mockMetricCollector{},
		reconcileCounters:                    metricsutil.NewReconcileCounters(),
	}

	req := reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: "default",
			Name:      "test-tgb",
		},
	}

	// EXECUTE
	_, err := reconciler.Reconcile(context.Background(), req)

	// VERIFY
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "target group not found")

	// Verify FailedReconcile event was emitted
	select {
	case event := <-fakeRecorder.Events:
		assert.Contains(t, event, "FailedReconcile")
		assert.Contains(t, event, "target group not found")
	default:
		t.Fatal("expected FailedReconcile event but none was emitted")
	}
}

func TestTargetGroupBindingReconciler_Reconcile_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	elbv2api.AddToScheme(scheme)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tgb",
			Namespace: "default",
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetGroupARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/test/123",
		},
	}

	k8sClient := testclient.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(tgb).WithRuntimeObjects(tgb).Build()

	// Setup Mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFinalizerManager := k8s.NewMockFinalizerManager(ctrl)
	mockFinalizerManager.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	mockResMgr := targetgroupbinding.NewMockResourceManager(ctrl)
	mockResMgr.EXPECT().Reconcile(gomock.Any(), gomock.Any()).Return(false, nil)

	fakeRecorder := record.NewFakeRecorder(10)

	reconciler := &targetGroupBindingReconciler{
		k8sClient:                            k8sClient,
		eventRecorder:                        fakeRecorder,
		finalizerManager:                     mockFinalizerManager,
		tgbResourceManager:                   mockResMgr,
		deferredTargetGroupBindingReconciler: &mockDeferredReconciler{},
		logger:                               log.Log.WithName("controllers").WithName("TargetGroupBinding"),
		metricsCollector:                     &mockMetricCollector{},
		reconcileCounters:                    metricsutil.NewReconcileCounters(),
	}

	req := reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: "default",
			Name:      "test-tgb",
		},
	}

	// EXECUTE
	_, err := reconciler.Reconcile(context.Background(), req)

	// VERIFY
	assert.NoError(t, err)

	// Verify SuccessfullyReconciled event was emitted
	select {
	case event := <-fakeRecorder.Events:
		assert.Contains(t, event, "SuccessfullyReconciled")
		assert.Contains(t, event, "Successfully reconciled")
	default:
		t.Fatal("expected SuccessfullyReconciled event but none was emitted")
	}
}
