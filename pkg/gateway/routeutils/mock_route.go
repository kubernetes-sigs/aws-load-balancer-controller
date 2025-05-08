package routeutils

import (
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type MockRoute struct {
	Kind      RouteKind
	Name      string
	Namespace string
	Hostnames []string
}

func (m *MockRoute) GetBackendRefs() []gwv1.BackendRef {
	//TODO implement me
	panic("implement me")
}

func (m *MockRoute) GetRouteNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: m.Namespace,
		Name:      m.Name,
	}
}

func (m *MockRoute) GetRouteKind() RouteKind {
	return m.Kind
}

func (m *MockRoute) GetHostnames() []gwv1.Hostname {
	hostnames := make([]gwv1.Hostname, len(m.Hostnames))
	for i, h := range m.Hostnames {
		hostnames[i] = gwv1.Hostname(h)
	}
	return hostnames
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
