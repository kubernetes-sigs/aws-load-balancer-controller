package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// --- mocks ---

type mockModelBuilder struct {
	stack     core.Stack
	lb        *elbv2model.LoadBalancer
	backendSG bool
	err       error
}

func (m *mockModelBuilder) Build(_ context.Context, _ *corev1.Service, _ lbcmetrics.MetricCollector) (core.Stack, *elbv2model.LoadBalancer, bool, error) {
	return m.stack, m.lb, m.backendSG, m.err
}

type mockStackDeployer struct {
	err           error
	deployedCount int
}

func (m *mockStackDeployer) Deploy(_ context.Context, _ core.Stack, _ lbcmetrics.MetricCollector, _ string) error {
	m.deployedCount++
	return m.err
}

type mockStackMarshaller struct{}

func (m *mockStackMarshaller) Marshal(_ core.Stack) (string, error) {
	return "{}", nil
}

type mockFinalizerManager struct{}

func (m *mockFinalizerManager) AddFinalizers(_ context.Context, _ client.Object, _ ...string) error {
	return nil
}

func (m *mockFinalizerManager) RemoveFinalizers(_ context.Context, _ client.Object, _ ...string) error {
	return nil
}

type mockMetricsCollector struct{}

func (m *mockMetricsCollector) ObservePodReadinessGateReady(_ string, _ string, _ time.Duration) {}
func (m *mockMetricsCollector) ObserveQUICTargetMissingServerId(_ string, _ string)              {}
func (m *mockMetricsCollector) ObserveControllerReconcileError(_ string, _ string)               {}
func (m *mockMetricsCollector) ObserveControllerReconcileLatency(_ string, _ string, fn func())  { fn() }
func (m *mockMetricsCollector) ObserveWebhookValidationError(_ string, _ string)                 {}
func (m *mockMetricsCollector) ObserveWebhookMutationError(_ string, _ string)                   {}
func (m *mockMetricsCollector) StartCollectTopTalkers(_ context.Context)                         {}
func (m *mockMetricsCollector) StartCollectCacheSize(_ context.Context)                          {}

// buildTestReconciler wires up a serviceReconciler with the given mocks and a real fake k8s client
// pre-populated with svc.
func buildTestReconciler(svc *corev1.Service, mb *mockModelBuilder, sd *mockStackDeployer) *serviceReconciler {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	k8sClient := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()

	return &serviceReconciler{
		k8sClient:        k8sClient,
		eventRecorder:    record.NewFakeRecorder(10),
		finalizerManager: &mockFinalizerManager{},
		modelBuilder:     mb,
		stackMarshaller:  &mockStackMarshaller{},
		stackDeployer:    sd,
		logger:           logr.Discard(),
		metricsCollector: &mockMetricsCollector{},
	}
}

// --- reconcile tests ---

func TestReconcile(t *testing.T) {
	stack := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "default", Name: "my-svc"}))
	tests := []struct {
		name         string
		svc          *corev1.Service
		lb           *elbv2model.LoadBalancer
		deployErr    error
		wantErr      bool
		wantDeployed int
	}{
		{
			name: "lb nil, no finalizer: cleanup is no-op, returns nil",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
			},
			lb:           nil,
			wantErr:      false,
			wantDeployed: 0,
		},
		{
			name: "lb nil, has finalizer, cleanup deploy fails: returns error",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "my-svc",
					Namespace:  "default",
					Finalizers: []string{"service.k8s.aws/resources"},
				},
			},
			lb:           nil,
			deployErr:    errors.New("deploy failed"),
			wantErr:      true,
			wantDeployed: 1,
		},
		{
			name: "lb not nil: reconcileLoadBalancerResources is called",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "my-svc",
					Namespace:  "default",
					Finalizers: []string{"service.k8s.aws/resources"},
				},
			},
			lb:           &elbv2model.LoadBalancer{},
			wantErr:      true,
			wantDeployed: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb := &mockModelBuilder{stack: stack, lb: tt.lb}
			sd := &mockStackDeployer{err: tt.deployErr}
			r := buildTestReconciler(tt.svc, mb, sd)

			err := r.reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-svc"},
			})

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantDeployed, sd.deployedCount)
		})
	}
}

func TestBuildPortsForStatus(t *testing.T) {
	tests := []struct {
		name     string
		service  *corev1.Service
		expected []corev1.PortStatus
	}{
		{
			name: "service with single port",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
					},
				},
			},
			expected: []corev1.PortStatus{
				{
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
		{
			name: "service with multiple ports",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
						{
							Name:     "https",
							Protocol: corev1.ProtocolTCP,
							Port:     443,
						},
						{
							Name:     "dns",
							Protocol: corev1.ProtocolUDP,
							Port:     53,
						},
					},
				},
			},
			expected: []corev1.PortStatus{
				{
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Port:     443,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Port:     53,
					Protocol: corev1.ProtocolUDP,
				},
			},
		},
		{
			name: "service with no ports",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &serviceReconciler{}
			result := reconciler.buildPortsForStatus(tt.service)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldUpdatePorts(t *testing.T) {
	tests := []struct {
		name     string
		service  *corev1.Service
		expected bool
	}{
		{
			name: "no existing ingress entry",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{},
					},
				},
			},
			expected: true,
		},
		{
			name: "different port count",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
						{
							Name:     "https",
							Protocol: corev1.ProtocolTCP,
							Port:     443,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								Hostname: "test-nlb.elb.amazonaws.com",
								Ports: []corev1.PortStatus{
									{
										Port:     80,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "missing port",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
						{
							Name:     "https",
							Protocol: corev1.ProtocolTCP,
							Port:     443,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								Hostname: "test-nlb.elb.amazonaws.com",
								Ports: []corev1.PortStatus{
									{
										Port:     80,
										Protocol: corev1.ProtocolTCP,
									},
									{
										Port:     8080, // Different from spec
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "matching ports - no update needed",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
						{
							Name:     "https",
							Protocol: corev1.ProtocolTCP,
							Port:     443,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								Hostname: "test-nlb.elb.amazonaws.com",
								Ports: []corev1.PortStatus{
									{
										Port:     80,
										Protocol: corev1.ProtocolTCP,
									},
									{
										Port:     443,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "matching ports, order changed- no update needed",
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Protocol: corev1.ProtocolTCP,
							Port:     80,
						},
						{
							Name:     "https",
							Protocol: corev1.ProtocolTCP,
							Port:     443,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								Hostname: "test-nlb.elb.amazonaws.com",
								Ports: []corev1.PortStatus{
									{
										Port:     443,
										Protocol: corev1.ProtocolTCP,
									},
									{
										Port:     80,
										Protocol: corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &serviceReconciler{}
			result := reconciler.shouldUpdatePorts(tt.service)
			assert.Equal(t, tt.expected, result)
		})
	}
}
