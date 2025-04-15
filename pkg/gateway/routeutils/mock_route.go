package routeutils

import (
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type MockRoute struct {
	Kind string
}

func (m *MockRoute) GetRouteNamespacedName() types.NamespacedName {
	//TODO implement me
	panic("implement me")
}

func (m *MockRoute) GetRouteKind() string {
	return m.Kind
}

func (m *MockRoute) GetHostnames() []gwv1.Hostname {
	//TODO implement me
	panic("implement me")
}

func (m *MockRoute) GetParentRefs() []gwv1.ParentReference {
	//TODO implement me
	panic("implement me")
}

func (m *MockRoute) GetRawRoute() interface{} {
	//TODO implement me
	panic("implement me")
}

func (m *MockRoute) GetAttachedRules() []RouteRule {
	//TODO implement me
	panic("implement me")
}

var _ RouteDescriptor = &MockRoute{}
