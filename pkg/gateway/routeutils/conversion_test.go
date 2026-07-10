package routeutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestConvertAlpha2TCPRouteToV1(t *testing.T) {
	sectionName := gwv1.SectionName("listener-1")
	alpha := &gwalpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-tcp", Namespace: "ns", Generation: 3},
		Spec: gwalpha2.TCPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: "gw", SectionName: &sectionName}},
			},
			Rules: []gwalpha2.TCPRouteRule{
				{BackendRefs: []gwv1.BackendRef{{BackendObjectReference: gwv1.BackendObjectReference{Name: "svc-1"}}}},
			},
		},
		Status: gwalpha2.TCPRouteStatus{
			RouteStatus: gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{{ControllerName: "gateway.k8s.aws/nlb"}}},
		},
	}

	converted := ConvertAlpha2TCPRouteToV1(alpha)

	assert.Equal(t, alpha.ObjectMeta, converted.ObjectMeta)
	assert.Equal(t, alpha.Spec.ParentRefs, converted.Spec.ParentRefs)
	assert.Len(t, converted.Spec.Rules, 1)
	assert.Equal(t, gwv1.ObjectName("svc-1"), converted.Spec.Rules[0].BackendRefs[0].Name)
	assert.Equal(t, gwv1.TCPRouteStatus(alpha.Status), converted.Status)
	assert.Nil(t, ConvertAlpha2TCPRouteToV1(nil))
}

func TestConvertAlpha2UDPRouteToV1(t *testing.T) {
	alpha := &gwalpha2.UDPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-udp", Namespace: "ns"},
		Spec: gwalpha2.UDPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{{Name: "gw"}}},
			Rules: []gwalpha2.UDPRouteRule{
				{BackendRefs: []gwv1.BackendRef{{BackendObjectReference: gwv1.BackendObjectReference{Name: "svc-2"}}}},
			},
		},
		Status: gwalpha2.UDPRouteStatus{
			RouteStatus: gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{{ControllerName: "gateway.k8s.aws/nlb"}}},
		},
	}

	converted := ConvertAlpha2UDPRouteToV1(alpha)

	assert.Equal(t, alpha.ObjectMeta, converted.ObjectMeta)
	assert.Equal(t, alpha.Spec.ParentRefs, converted.Spec.ParentRefs)
	assert.Equal(t, gwv1.ObjectName("svc-2"), converted.Spec.Rules[0].BackendRefs[0].Name)
	assert.Equal(t, gwv1.UDPRouteStatus(alpha.Status), converted.Status)
	assert.Nil(t, ConvertAlpha2UDPRouteToV1(nil))
}
