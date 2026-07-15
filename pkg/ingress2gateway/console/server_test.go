package console

import (
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

	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/constants"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = gwv1.Install(s)
	_ = networking.AddToScheme(s)
	return s
}

// minimal dry-run plan JSON snippets reused across tests
const (
	planWithMigratedFrom = `{"id":"ns/my-gw","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"name":"test","tags":{"gateway.k8s.aws/migrated-from":"ingress/ns/my-ing"}}}}}}`
	simpleIngressPlan    = `{"id":"ns/my-ing","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"name":"old"}}}}}`
)

func TestHandleNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		objects  []runtime.Object
		wantJSON []map[string]any
	}{
		{
			name:     "empty cluster",
			objects:  nil,
			wantJSON: []map[string]any{},
		},
		{
			name: "gateways in two namespaces",
			objects: []runtime.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gw-a", Namespace: "alpha",
						Annotations: map[string]string{gateway_constants.AnnotationDryRunPlan: planWithMigratedFrom},
					},
				},
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gw-a2", Namespace: "alpha",
						Annotations: map[string]string{gateway_constants.AnnotationDryRunPlan: planWithMigratedFrom},
					},
				},
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gw-b", Namespace: "beta",
						Annotations: map[string]string{gateway_constants.AnnotationDryRunPlan: planWithMigratedFrom},
					},
				},
				// Gateway without plan annotation — must be excluded.
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{Name: "no-plan", Namespace: "gamma"},
				},
			},
			wantJSON: []map[string]any{
				{"namespace": "alpha", "gatewayCount": float64(2)},
				{"namespace": "beta", "gatewayCount": float64(1)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			server := NewConsoleServer(builder.Build())

			req := httptest.NewRequest(http.MethodGet, "/api/namespaces", nil)
			w := httptest.NewRecorder()
			server.handleNamespaces(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			var got []map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, tt.wantJSON, got)
		})
	}
}

func TestHandleGateways(t *testing.T) {
	tests := []struct {
		name           string
		namespaceQuery string
		objects        []runtime.Object
		wantStatus     int
		wantCount      int
		wantFirstName  string
		wantFirstError string
	}{
		{
			name:           "missing namespace param",
			namespaceQuery: "",
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "namespace with no gateways",
			namespaceQuery: "ns",
			objects:        nil,
			wantStatus:     http.StatusOK,
			wantCount:      0,
		},
		{
			name:           "gateway with plan and matching ingress",
			namespaceQuery: "ns",
			objects: []runtime.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gw",
						Namespace: "ns",
						Annotations: map[string]string{
							gateway_constants.AnnotationDryRunPlan: planWithMigratedFrom,
						},
					},
				},
				&networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-ing",
						Namespace: "ns",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/dry-run-plan": simpleIngressPlan,
						},
					},
				},
			},
			wantStatus:    http.StatusOK,
			wantCount:     1,
			wantFirstName: "my-gw",
		},
		{
			name:           "gateway without ingress plan holder",
			namespaceQuery: "ns",
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
			wantStatus:     http.StatusOK,
			wantCount:      1,
			wantFirstName:  "orphan-gw",
			wantFirstError: "could not determine ingress plan holder",
		},
		{
			name:           "gateway in another namespace is excluded",
			namespaceQuery: "ns",
			objects: []runtime.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "other", Namespace: "other-ns",
						Annotations: map[string]string{gateway_constants.AnnotationDryRunPlan: planWithMigratedFrom},
					},
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			server := NewConsoleServer(builder.Build())

			url := "/api/gateways"
			if tt.namespaceQuery != "" {
				url += "?namespace=" + tt.namespaceQuery
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			server.handleGateways(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus != http.StatusOK {
				return
			}

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
	gatewayPlan := `{"id":"ns/my-gw","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"name":"new","scheme":"internet-facing","tags":{"gateway.k8s.aws/migrated-from":"ingress/ns/my-ing"}}}}}}`

	tests := []struct {
		name       string
		query      string
		objects    []runtime.Object
		wantStatus int
		wantSame   int
		wantChange int
	}{
		{
			name:       "missing namespace param",
			query:      "gateway=my-gw",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing gateway param",
			query:      "namespace=ns",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "gateway not found",
			query:      "namespace=ns&gateway=nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:  "successful diff",
			query: "namespace=ns&gateway=my-gw",
			objects: []runtime.Object{
				&gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-gw", Namespace: "ns",
						Annotations: map[string]string{
							gateway_constants.AnnotationDryRunPlan: gatewayPlan,
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
			server := NewConsoleServer(builder.Build())
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
	server := NewConsoleServer(fake.NewClientBuilder().WithScheme(scheme).Build())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "Migration Console")
}
