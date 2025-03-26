package routeutils

import v1 "k8s.io/api/core/v1"

type Rule interface {
	GetSectionName() string
	GetService() *v1.Service
	GetHostnames() []string
	GetWeight() int32
}

var _ Rule = &ruleImpl{}

type ruleImpl struct{}

func (s *ruleImpl) GetHostnames() []string {
	//TODO implement me
	panic("implement me")
}

func (s *ruleImpl) GetSectionName() string {
	//TODO implement me
	panic("implement me")
}

func (s *ruleImpl) GetService() *v1.Service {
	//TODO implement me
	panic("implement me")
}

func (s *ruleImpl) GetWeight() int32 {
	//TODO implement me
	panic("implement me")
}
