package routeutils

import (
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"time"
)

type MockRule struct {
	RawRule            interface{}
	BackendRefs        []Backend
	ListenerRuleConfig *elbv2gw.ListenerRuleConfiguration
}

func (m *MockRule) GetRawRouteRule() interface{} {
	return m.RawRule
}

func (m *MockRule) GetBackends() []Backend {
	return m.BackendRefs
}

func (m *MockRule) GetListenerRuleConfig() *elbv2gw.ListenerRuleConfiguration {
	return m.ListenerRuleConfig
}

var _ RouteRule = &MockRule{}

type MockRoute struct {
	Kind                RouteKind
	Name                string
	Namespace           string
	Hostnames           []string
	CreationTime        time.Time
	Rules               []RouteRule
	CompatibleHostnames []gwv1.Hostname
}

func (m *MockRoute) GetBackendRefs() []gwv1.BackendRef {
	//TODO implement me
	panic("implement me")
}

func (m *MockRoute) GetRouteListenerRuleConfigRefs() []gwv1.LocalObjectReference {
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
	return m.Rules
}

func (m *MockRoute) GetRouteGeneration() int64 {
	panic("implement me")
}

func (m *MockRoute) GetRouteCreateTimestamp() time.Time {
	return m.CreationTime
}

func (m *MockRoute) GetCompatibleHostnames() []gwv1.Hostname {
	return m.CompatibleHostnames
}

func (m *MockRoute) SetCompatibleHostnames(hostnames []gwv1.Hostname) {
	m.CompatibleHostnames = hostnames
}

var _ RouteDescriptor = &MockRoute{}
