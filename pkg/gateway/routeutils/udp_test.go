package routeutils

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func Test_ConvertUDPRuleToRouteRule(t *testing.T) {

	rule := &gwv1.UDPRouteRule{
		Name:        (*gwv1.SectionName)(awssdk.String("my-name")),
		BackendRefs: []gwv1.BackendRef{},
	}

	backends := []Backend{
		{}, {},
	}

	result := convertUDPRouteRule(rule, backends)

	assert.Equal(t, backends, result.GetBackends())
	assert.Equal(t, rule, result.GetRawRouteRule().(*gwv1.UDPRouteRule))
}

func Test_ListUDPRoutes(t *testing.T) {
	k8sClient := testutils.GenerateTestClient()

	k8sClient.Create(context.Background(), &gwv1.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo1",
			Namespace: "bar1",
		},
		Spec: gwv1.UDPRouteSpec{
			Rules: []gwv1.UDPRouteRule{
				{
					BackendRefs: []gwv1.BackendRef{
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.BackendRef{
						{},
						{},
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.BackendRef{},
				},
			},
		},
	})

	k8sClient.Create(context.Background(), &gwv1.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo2",
			Namespace: "bar2",
		},
		Spec: gwv1.UDPRouteSpec{
			Rules: nil,
		},
	})

	k8sClient.Create(context.Background(), &gwv1.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo3",
			Namespace: "bar3",
		},
	})

	result, err := ListUDPRoutes(context.Background(), v1RouteVersions, k8sClient)

	assert.NoError(t, err)

	itemMap := make(map[string]string)
	for _, v := range result {
		routeNsn := v.GetRouteNamespacedName()
		itemMap[routeNsn.Namespace] = routeNsn.Name
		assert.Equal(t, UDPRouteKind, v.GetRouteKind())
		assert.NotNil(t, v.GetRawRoute())
		assert.Equal(t, 0, len(v.GetHostnames()))
		if routeNsn.Name == "foo1" {
			assert.Equal(t, 6, len(v.GetBackendRefs()))
		}

		if routeNsn.Name == "foo2" {
			assert.Equal(t, 0, len(v.GetBackendRefs()))
		}

		if routeNsn.Name == "foo3" {
			assert.Equal(t, 0, len(v.GetBackendRefs()))
		}
	}

	assert.Equal(t, "foo1", itemMap["bar1"])
	assert.Equal(t, "foo2", itemMap["bar2"])
	assert.Equal(t, "foo3", itemMap["bar3"])
}

// Test_ListUDPRoutes_Alpha2Fallback exercises the v1alpha2 fallback path
// (Gateway API < 1.6): v1alpha2 UDPRoutes are listed and converted to their v1
// representation at the list boundary.
func Test_ListUDPRoutes_Alpha2Fallback(t *testing.T) {
	k8sClient := testutils.GenerateTestClient()

	k8sClient.Create(context.Background(), &gwalpha2.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo1",
			Namespace: "bar1",
		},
		Spec: gwalpha2.UDPRouteSpec{
			Rules: []gwalpha2.UDPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{},
						{},
					},
				},
			},
		},
	})

	result, err := ListUDPRoutes(context.Background(), alpha2RouteVersions, k8sClient)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result))
	assert.Equal(t, UDPRouteKind, result[0].GetRouteKind())
	assert.Equal(t, "foo1", result[0].GetRouteNamespacedName().Name)
	assert.Equal(t, "bar1", result[0].GetRouteNamespacedName().Namespace)
	assert.Equal(t, 2, len(result[0].GetBackendRefs()))
	// The raw route is the v1 representation even though the source was v1alpha2.
	_, ok := result[0].GetRawRoute().(*gwv1.UDPRoute)
	assert.True(t, ok)
}

func Test_UDP_LoadAttachedRules(t *testing.T) {
	weight := 0
	mockLoader := func(ctx context.Context, k8sClient client.Client, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind RouteKind, gatewayDefaultTGConfig *elbv2gw.TargetGroupConfiguration) (*Backend, error, error) {
		weight++
		return &Backend{
			Weight: weight,
		}, nil, nil
	}
	mockListenerRuleConfigLoader := func(ctx context.Context, k8sClient client.Client, routeIdentifier types.NamespacedName, routeKind RouteKind, listenerRuleConfigRefs []gwv1.LocalObjectReference) (*elbv2gw.ListenerRuleConfiguration, error, error) {
		return nil, nil, nil
	}

	routeDescription := udpRouteDescription{
		route: &gwv1.UDPRoute{
			Spec: gwv1.UDPRouteSpec{Rules: []gwv1.UDPRouteRule{
				{
					BackendRefs: []gwv1.BackendRef{
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.BackendRef{
						{},
						{},
						{},
						{},
					},
				},
				{
					BackendRefs: []gwv1.BackendRef{},
				},
			}},
		},
		rules:           nil,
		ruleAccumulator: newAttachedRuleAccumulator[gwv1.UDPRouteRule](mockLoader, mockListenerRuleConfigLoader),
	}

	result, errs := routeDescription.loadAttachedRules(context.Background(), nil, nil)
	assert.Equal(t, 0, len(errs))
	convertedRules := result.GetAttachedRules()
	assert.Equal(t, 3, len(convertedRules))

	assert.Equal(t, 2, len(convertedRules[0].GetBackends()))
	assert.Equal(t, 4, len(convertedRules[1].GetBackends()))
	assert.Equal(t, 0, len(convertedRules[2].GetBackends()))

	assert.Nil(t, convertedRules[0].GetListenerRuleConfig())
	assert.Nil(t, convertedRules[1].GetListenerRuleConfig())
	assert.Nil(t, convertedRules[2].GetListenerRuleConfig())
}
