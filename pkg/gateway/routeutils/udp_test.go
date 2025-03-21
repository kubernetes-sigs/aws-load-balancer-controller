package routeutils

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"testing"
)

func Test_ConvertUDPRuleToRouteRule(t *testing.T) {

	rule := &gwalpha2.UDPRouteRule{
		Name:        (*gwv1.SectionName)(awssdk.String("my-name")),
		BackendRefs: []gwalpha2.BackendRef{},
	}

	backends := []Backend{
		{}, {},
	}

	result := convertUDPRouteRule(rule, backends)

	assert.Equal(t, backends, result.GetBackends())
	assert.Equal(t, rule, result.GetRawRouteRule().(*gwalpha2.UDPRouteRule))
}

func Test_ListUDPRoutes(t *testing.T) {
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)
	elbv2api.AddToScheme(k8sSchema)
	gwv1.AddToScheme(k8sSchema)
	gwalpha2.AddToScheme(k8sSchema)
	k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

	k8sClient.Create(context.Background(), &gwalpha2.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo1",
			Namespace: "bar1",
		},
		Spec: gwalpha2.UDPRouteSpec{
			Rules: nil,
		},
	})

	k8sClient.Create(context.Background(), &gwalpha2.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo2",
			Namespace: "bar2",
		},
		Spec: gwalpha2.UDPRouteSpec{
			Rules: nil,
		},
	})

	k8sClient.Create(context.Background(), &gwalpha2.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo3",
			Namespace: "bar3",
		},
	})

	result, err := ListUDPRoutes(context.Background(), k8sClient)

	assert.NoError(t, err)

	itemMap := make(map[string]string)
	for _, v := range result {
		routeNsn := v.GetRouteNamespacedName()
		itemMap[routeNsn.Namespace] = routeNsn.Name
		assert.Equal(t, UDPRouteKind, v.GetRouteKind())
		assert.NotNil(t, v.GetRawRoute())
		assert.Equal(t, 0, len(v.GetHostnames()))
	}

	assert.Equal(t, "foo1", itemMap["bar1"])
	assert.Equal(t, "foo2", itemMap["bar2"])
	assert.Equal(t, "foo3", itemMap["bar3"])
}

func Test_UDP_LoadAttachedRules(t *testing.T) {
	weight := 0
	mockLoader := func(ctx context.Context, k8sClient client.Client, typeSpecificBackend interface{}, backendRef gwv1.BackendRef, routeIdentifier types.NamespacedName, routeKind string) (*Backend, error) {
		weight++
		return &Backend{
			Weight: weight,
		}, nil
	}

	routeDescription := udpRouteDescription{
		route: &gwalpha2.UDPRoute{
			Spec: gwalpha2.UDPRouteSpec{Rules: []gwalpha2.UDPRouteRule{
				{
					BackendRefs: []gwalpha2.BackendRef{
						{},
						{},
					},
				},
				{
					BackendRefs: []gwalpha2.BackendRef{
						{},
						{},
						{},
						{},
					},
				},
				{
					BackendRefs: []gwalpha2.BackendRef{},
				},
			}},
		},
		rules:         nil,
		backendLoader: mockLoader,
	}

	result, err := routeDescription.loadAttachedRules(context.Background(), nil)
	assert.NoError(t, err)
	convertedRules := result.GetAttachedRules()
	assert.Equal(t, 3, len(convertedRules))

	assert.Equal(t, 2, len(convertedRules[0].GetBackends()))
	assert.Equal(t, 4, len(convertedRules[1].GetBackends()))
	assert.Equal(t, 0, len(convertedRules[2].GetBackends()))
}
