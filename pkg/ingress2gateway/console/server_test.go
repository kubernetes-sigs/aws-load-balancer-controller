package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = gwv1.Install(s)
	_ = networking.AddToScheme(s)
	return s
}

func TestHandleGateways(t *testing.T) {
	tests := []struct {
		name           string
		objects        []runtime.Object
		wantCount      int
		wantFirstName  string
		wantFirstError string
	}{
		{
			name:      "no gateways",
			objects:   nil,
			wantCount: 0,
		},
		{
			name: "gateway with plan and matching ingress",
			objects: []runtime.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gw",
						Namespace: "ns",
						Annotations: map[string]string{
							gateway_constants.AnnotationDryRunPlan:        `{"id":"ns/my-gw","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"name":"test","tags":{"gateway.k8s.aws/migrated-from":"ingress/ns/my-ing"}}}}}}`,
							gateway_constants.AnnotationIngressPlanHolder: "ns/my-ing",
						},
					},
				},
				&networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-ing",
						Namespace: "ns",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/dry-run-plan": `{"id":"ns/my-ing","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"name":"old"}}}}}`,
						},
					},
				},
			},
			wantCount:     1,
			wantFirstName: "my-gw",
		},
		{
			name: "gateway without ingress plan holder",
			objects: []runtime.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orphan-gw",
						Namespace: "ns",
						Annotations: map[string]string{
							gateway_constants.AnnotationDryRunPlan: `{"id":"ns/orphan-gw","resources":{}}`,
						},
					},
				},
			},
			wantCount:      1,
			wantFirstName:  "orphan-gw",
			wantFirstError: "could not determine ingress plan holder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			k8sClient := builder.Build()

			server := NewConsoleServer(k8sClient, "ns")
			req := httptest.NewRequest(http.MethodGet, "/api/gateways", nil)
			w := httptest.NewRecorder()

			server.handleGateways(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var items []map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
			assert.Len(t, items, tt.wantCount)

			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirstName, items[0]["name"])
				if tt.wantFirstError != "" {
					assert.Contains(t, items[0]["error"], tt.wantFirstError)
				}
			}
		})
	}
}

func TestHandleDiff(t *testing.T) {
	ingressPlan := `{"id":"ns/my-ing","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"name":"old","scheme":"internet-facing"}}}}}`
	gatewayPlan := `{"id":"ns/my-gw","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"name":"new","scheme":"internet-facing"}}}}}`

	tests := []struct {
		name       string
		query      string
		objects    []runtime.Object
		wantStatus int
		wantSame   int
		wantChange int
	}{
		{
			name:       "missing gateway param",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "gateway not found",
			query:      "gateway=nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:  "successful diff",
			query: "gateway=my-gw",
			objects: []runtime.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-gw", Namespace: "ns",
						Annotations: map[string]string{
							gateway_constants.AnnotationDryRunPlan:        gatewayPlan,
							gateway_constants.AnnotationIngressPlanHolder: "ns/my-ing",
						},
					},
				},
				&networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ing", Namespace: "ns",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/dry-run-plan": ingressPlan,
						},
					},
				},
			},
			wantStatus: http.StatusOK,
			wantSame:   1, // scheme
			wantChange: 1, // name
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			k8sClient := builder.Build()

			_ = context.Background()
			server := NewConsoleServer(k8sClient, "ns")
			req := httptest.NewRequest(http.MethodGet, "/api/diff?"+tt.query, nil)
			w := httptest.NewRecorder()

			server.handleDiff(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				var result DiffResult
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				assert.Equal(t, tt.wantSame, result.Summary.Same)
				assert.Equal(t, tt.wantChange, result.Summary.Changed)
			}
		})
	}
}

func TestHandleIndex(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	server := NewConsoleServer(k8sClient, "test-ns")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "test-ns")
}
