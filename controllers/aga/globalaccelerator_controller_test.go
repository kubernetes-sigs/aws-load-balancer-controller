package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/v3/apis/aga/v1beta1"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// recordedCondition captures the arguments of a reconcile-condition observation/deletion.
type recordedCondition struct {
	controller string
	namespace  string
	name       string
	reconciled bool
}

// recordingMetricCollector records the reconcile-condition calls so tests can assert on the
// per-resource condition metric. ObserveControllerReconcileLatency invokes the passed fn so the
// wrapped reconcile stages actually run.
type recordingMetricCollector struct {
	conditions []recordedCondition
	deletes    []recordedCondition
}

func (m *recordingMetricCollector) ObservePodReadinessGateReady(_ string, _ string, _ time.Duration) {
}
func (m *recordingMetricCollector) ObserveQUICTargetMissingServerId(_ string, _ string) {}
func (m *recordingMetricCollector) ObserveControllerReconcileError(_ string, _ string)  {}
func (m *recordingMetricCollector) ObserveControllerReconcileCondition(controller string, namespace string, name string, reconciled bool) {
	m.conditions = append(m.conditions, recordedCondition{controller, namespace, name, reconciled})
}
func (m *recordingMetricCollector) DeleteControllerReconcileCondition(controller string, namespace string, name string) {
	m.deletes = append(m.deletes, recordedCondition{controller: controller, namespace: namespace, name: name})
}
func (m *recordingMetricCollector) ObserveControllerReconcileLatency(_ string, _ string, fn func()) {
	fn()
}
func (m *recordingMetricCollector) ObserveWebhookValidationError(_ string, _ string) {}
func (m *recordingMetricCollector) ObserveWebhookMutationError(_ string, _ string)   {}
func (m *recordingMetricCollector) StartCollectTopTalkers(_ context.Context)         {}
func (m *recordingMetricCollector) StartCollectCacheSize(_ context.Context)          {}

// TestGlobalAcceleratorReconciler_Reconcile_NotFound covers the branch where the GlobalAccelerator
// has already been deleted: the fetch returns NotFound and the reconcile drops the condition series.
func TestGlobalAcceleratorReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = agaapi.AddToScheme(scheme)

	// no object seeded into the fake client, so the Get returns NotFound
	k8sClient := testclient.NewClientBuilder().WithScheme(scheme).Build()

	metricsCollector := &recordingMetricCollector{}
	r := &globalAcceleratorReconciler{
		k8sClient:        k8sClient,
		logger:           logr.Discard(),
		metricsCollector: metricsCollector,
	}

	err := r.reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "gone-aga"},
	})

	assert.NoError(t, err)
	assert.Empty(t, metricsCollector.conditions)
	assert.Equal(t, []recordedCondition{{controller: "globalAccelerator", namespace: "default", name: "gone-aga"}}, metricsCollector.deletes)
}
