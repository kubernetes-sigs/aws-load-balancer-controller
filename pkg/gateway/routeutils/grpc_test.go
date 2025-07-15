package routeutils

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_ConvertGRPCRuleToRouteRule(t *testing.T) {

	rule := &gwv1.GRPCRouteRule{
		Name:               (*gwv1.SectionName)(awssdk.String("my-name")),
		Matches:            []gwv1.GRPCRouteMatch{},
		Filters:            []gwv1.GRPCRouteFilter{},
		BackendRefs:        []gwv1.GRPCBackendRef{},
		SessionPersistence: &gwv1.SessionPersistence{},
	}

	backends := []Backend{
		{}, {},
	}

	result := convertGRPCRouteRule(rule, backends)

	assert.Equal(t, backends, result.GetBackends())
	assert.Equal(t, rule, result.GetRawRouteRule().(*gwv1.GRPCRouteRule))
}

func Test_ListGRPCRoutes(t *testing.T) {
	k8sClient := testutils.GenerateTestClient()

	k8sClient.Create(context.Background(), &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo1",
			Namespace: "bar1",
		},
		Spec: gwv1.GRPCRouteSpec{
			Hostnames: []gwv1.Hostname{
				"host1",
			},
			Rules: []gwv1.GRPCRouteRule{
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{},
						{},
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{},
				},
			},
		},
	})

	k8sClient.Create(context.Background(), &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo2",
			Namespace: "bar2",
		},
		Spec: gwv1.GRPCRouteSpec{
			Hostnames: []gwv1.Hostname{
				"host2",
			},
			Rules: nil,
		},
	})

	k8sClient.Create(context.Background(), &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo3",
			Namespace: "bar3",
		},
	})

	result, err := ListGRPCRoutes(context.Background(), k8sClient)

	assert.NoError(t, err)

	itemMap := make(map[string]string)
	for _, v := range result {
		routeNsn := v.GetRouteNamespacedName()
		itemMap[routeNsn.Namespace] = routeNsn.Name
		assert.Equal(t, GRPCRouteKind, v.GetRouteKind())
		assert.NotNil(t, v.GetRawRoute())

		if routeNsn.Name == "foo1" {
			assert.Equal(t, []gwv1.Hostname{
				"host1",
			}, v.GetHostnames())
			assert.Equal(t, 6, len(v.GetBackendRefs()))
		}

		if routeNsn.Name == "foo2" {
			assert.Equal(t, []gwv1.Hostname{
				"host2",
			}, v.GetHostnames())
			assert.Equal(t, 0, len(v.GetBackendRefs()))
		}

		if routeNsn.Name == "foo3" {
			assert.Equal(t, 0, len(v.GetHostnames()))
			assert.Equal(t, 0, len(v.GetBackendRefs()))
		}

	}

	assert.Equal(t, "foo1", itemMap["bar1"])
	assert.Equal(t, "foo2", itemMap["bar2"])
	assert.Equal(t, "foo3", itemMap["bar3"])
}

func Test_GRPC_LoadAttachedRules(t *testing.T) {
	weight := 0
	mockLoader := func(ctx context.Context, k8sClient client.Client, typeSpecificBackend interface{}, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind RouteKind) (*Backend, error, error) {
		weight++
		return &Backend{
			Weight: weight,
		}, nil, nil
	}

	routeDescription := grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			Spec: gwv1.GRPCRouteSpec{Rules: []gwv1.GRPCRouteRule{
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{},
						{},
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{},
				},
			}},
		},
		rules:         nil,
		backendLoader: mockLoader,
	}

	result, errs := routeDescription.loadAttachedRules(context.Background(), nil)
	assert.Equal(t, 0, len(errs))
	convertedRules := result.GetAttachedRules()
	assert.Equal(t, 3, len(convertedRules))

	assert.Equal(t, 2, len(convertedRules[0].GetBackends()))
	assert.Equal(t, 4, len(convertedRules[1].GetBackends()))
	assert.Equal(t, 0, len(convertedRules[2].GetBackends()))
}
