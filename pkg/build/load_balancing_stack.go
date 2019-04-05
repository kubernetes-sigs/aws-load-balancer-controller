package build

import (
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
)

// The resource stack for an Ingress group.
// which contains the following items:
// 1. ManagedLBSecurityGroup is the securityGroup that firewalls traffic to LB(which may not exists if custom security group are used)
// 1. InstanceSecurityGroups is the desired node securityGroups that should accept incoming traffic from ManagedLBSecurityGroup.
// 1. LoadBalancer is the ApplicationLoadBalancer that accepts traffic.
// 1. TargetGroups is the TargetGroups that will be registered to this LoadBalancer.
// 1. EndpointBindings is the api object that responsible for binding endpoints to targetGroup
type LoadBalancingStack struct {
	ID string

	ManagedLBSecurityGroup *api.SecurityGroup
	InstanceSecurityGroups []string

	LoadBalancer     *api.LoadBalancer
	TargetGroups     map[string]*api.TargetGroup
	EndpointBindings map[string]*api.EndpointBinding
}

func (s *LoadBalancingStack) FindSecurityGroup(sgID string) (*api.SecurityGroup, bool) {
	if s.ManagedLBSecurityGroup != nil && s.ManagedLBSecurityGroup.Name == sgID {
		return s.ManagedLBSecurityGroup, true
	}
	return nil, false
}

func (s *LoadBalancingStack) AddTargetGroup(tg *api.TargetGroup) {
	if s.TargetGroups == nil {
		s.TargetGroups = make(map[string]*api.TargetGroup)
	}

	s.TargetGroups[tg.Name] = tg
}

func (s *LoadBalancingStack) FindTargetGroup(tgID string) (*api.TargetGroup, bool) {
	tg, ok := s.TargetGroups[tgID]
	return tg, ok
}

func (s *LoadBalancingStack) AddEndpointBinding(eb *api.EndpointBinding) {
	if s.EndpointBindings == nil {
		s.EndpointBindings = make(map[string]*api.EndpointBinding)
	}
	s.EndpointBindings[eb.Name] = eb
}
