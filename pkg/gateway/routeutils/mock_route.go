package routeutils

import (
	"time"

	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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
	Kind                      RouteKind
	Name                      string
	Namespace                 string
	Hostnames                 []string
	CreationTime              time.Time
	Rules                     []RouteRule
	CompatibleHostnamesByPort map[int32][]gwv1.Hostname
	GatewayDefaultTGConfig    *elbv2gw.TargetGroupConfiguration
}

func (m *MockRoute) GetBackendRefs() []gwv1.BackendRef {
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

func (m *MockRoute) GetRouteIdentifier() string {
	panic("implement me")
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

func (m *MockRoute) GetCompatibleHostnamesByPort() map[int32][]gwv1.Hostname {
	return m.CompatibleHostnamesByPort
}

func (m *MockRoute) setCompatibleHostnamesByPort(hostnamesByPort map[int32][]gwv1.Hostname) {
	m.CompatibleHostnamesByPort = hostnamesByPort
}

func (m *MockRoute) setGatewayDefaultTGConfig(config *elbv2gw.TargetGroupConfiguration) {
	m.GatewayDefaultTGConfig = config
}

var _ RouteDescriptor = &MockRoute{}
