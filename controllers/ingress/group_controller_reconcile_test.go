package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/lbc"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/util"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	networkingpkg "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// --- mocks ---

type recordedCondition struct {
	controller string
	namespace  string
	name       string
	reconciled bool
}

type recordingMetricsCollector struct {
	conditions []recordedCondition
	deletes    []recordedCondition
}

func (m *recordingMetricsCollector) ObservePodReadinessGateReady(_ string, _ string, _ time.Duration) {
}
func (m *recordingMetricsCollector) ObserveQUICTargetMissingServerId(_ string, _ string) {}
func (m *recordingMetricsCollector) ObserveControllerReconcileError(_ string, _ string)  {}
func (m *recordingMetricsCollector) ObserveControllerReconcileCondition(controller string, namespace string, name string, reconciled bool) {
	m.conditions = append(m.conditions, recordedCondition{controller, namespace, name, reconciled})
}
func (m *recordingMetricsCollector) DeleteControllerReconcileCondition(controller string, namespace string, name string) {
	m.deletes = append(m.deletes, recordedCondition{controller: controller, namespace: namespace, name: name})
}
func (m *recordingMetricsCollector) ObserveControllerReconcileLatency(_ string, _ string, fn func()) {
	fn()
}
func (m *recordingMetricsCollector) ObserveWebhookValidationError(_ string, _ string) {}
func (m *recordingMetricsCollector) ObserveWebhookMutationError(_ string, _ string)   {}
func (m *recordingMetricsCollector) StartCollectTopTalkers(_ context.Context)         {}
func (m *recordingMetricsCollector) StartCollectCacheSize(_ context.Context)          {}

type mockGroupLoader struct {
	group ingress.Group
	err   error
}

func (m *mockGroupLoader) Load(_ context.Context, _ ingress.GroupID) (ingress.Group, error) {
	return m.group, m.err
}
func (m *mockGroupLoader) LoadGroupIDIfAny(_ context.Context, _ *networking.Ingress) (*ingress.GroupID, error) {
	return nil, nil
}
func (m *mockGroupLoader) LoadGroupIDsPendingFinalization(_ context.Context, _ *networking.Ingress) []ingress.GroupID {
	return nil
}

type mockGroupFinalizerManager struct{}

func (m *mockGroupFinalizerManager) AddGroupFinalizer(_ context.Context, _ ingress.GroupID, _ []ingress.ClassifiedIngress) error {
	return nil
}
func (m *mockGroupFinalizerManager) RemoveGroupFinalizer(_ context.Context, _ ingress.GroupID, _ []*networking.Ingress) error {
	return nil
}

type mockModelBuilder struct {
	lb *elbv2model.LoadBalancer
}

func (m *mockModelBuilder) Build(_ context.Context, ingGroup ingress.Group, _ lbcmetrics.MetricCollector) (core.Stack, *elbv2model.LoadBalancer, []types.NamespacedName, bool, *elbv2model.LoadBalancer, []int32, error) {
	stack := core.NewDefaultStack(core.StackID(types.NamespacedName(ingGroup.ID)))
	return stack, m.lb, nil, false, nil, nil, nil
}

type mockStackMarshaller struct{}

func (m *mockStackMarshaller) Marshal(_ core.Stack) (string, error) { return "{}", nil }

type mockStackDeployer struct{}

func (m *mockStackDeployer) Deploy(_ context.Context, _ core.Stack, _ lbcmetrics.MetricCollector, _ string) error {
	return nil
}

type mockSecretsManager struct{}

func (m *mockSecretsManager) MonitorSecrets(_ string, _ []types.NamespacedName) {}
func (m *mockSecretsManager) GetSecret(_ context.Context, _ client.Client, _ types.NamespacedName) (*corev1.Secret, error) {
	return nil, nil
}

type mockBackendSGProvider struct{}

func (m *mockBackendSGProvider) Get(_ context.Context, _ networkingpkg.ResourceType, _ []types.NamespacedName) (string, error) {
	return "", nil
}
func (m *mockBackendSGProvider) Release(_ context.Context, _ networkingpkg.ResourceType, _ []types.NamespacedName) error {
	return nil
}

func buildTestGroupReconciler(loader *mockGroupLoader, mb *mockModelBuilder) *groupReconciler {
	return &groupReconciler{
		eventRecorder:         record.NewFakeRecorder(10),
		modelBuilder:          mb,
		stackMarshaller:       &mockStackMarshaller{},
		stackDeployer:         &mockStackDeployer{},
		secretsManager:        &mockSecretsManager{},
		backendSGProvider:     &mockBackendSGProvider{},
		groupLoader:           loader,
		groupFinalizerManager: &mockGroupFinalizerManager{},
		featureGates:          config.NewFeatureGates(),
		logger:                logr.Discard(),
		metricsCollector:      &recordingMetricsCollector{},
		reconcileCounters:     metricsutil.NewReconcileCounters(),
	}
}

// --- tests ---

// TestGroupReconciler_reconcile_membersPresent covers the branch where the group still has member
// Ingresses: the reconcile marks the group's condition healthy.
func TestGroupReconciler_reconcile_membersPresent(t *testing.T) {
	group := ingress.Group{
		ID: ingress.GroupID{Namespace: "default", Name: "my-ing"},
		Members: []ingress.ClassifiedIngress{
			{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{Name: "my-ing", Namespace: "default"},
				},
			},
		},
	}
	// lb nil so the reconcile skips DNS resolution and status update
	r := buildTestGroupReconciler(&mockGroupLoader{group: group}, &mockModelBuilder{lb: nil})

	err := r.reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-ing"},
	})

	assert.NoError(t, err)
	mc := r.metricsCollector.(*recordingMetricsCollector)
	assert.Equal(t, []recordedCondition{{controller: "ingress", namespace: "default", name: "my-ing", reconciled: true}}, mc.conditions)
	assert.Empty(t, mc.deletes)
}

// TestGroupReconciler_reconcile_noMembers covers the branch where the group no longer contains any
// member Ingress: its condition series must be removed so it does not linger.
func TestGroupReconciler_reconcile_noMembers(t *testing.T) {
	group := ingress.Group{
		ID: ingress.GroupID{Namespace: "default", Name: "empty-ing"},
	}
	r := buildTestGroupReconciler(&mockGroupLoader{group: group}, &mockModelBuilder{lb: nil})

	err := r.reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "empty-ing"},
	})

	assert.NoError(t, err)
	mc := r.metricsCollector.(*recordingMetricsCollector)
	assert.Empty(t, mc.conditions)
	assert.Equal(t, []recordedCondition{{controller: "ingress", namespace: "default", name: "empty-ing"}}, mc.deletes)
}
