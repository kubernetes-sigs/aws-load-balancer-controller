package routeutils

import v1 "k8s.io/api/core/v1"

type BackendDescription interface {
	GetService() *v1.Service
	GetWeight() int32
	GetPort() int
}

var _ BackendDescription = &backendDescriptionImpl{}

type backendDescriptionImpl struct {
}

func (b backendDescriptionImpl) GetService() *v1.Service {
	//TODO implement me
	panic("implement me")
}

func (b backendDescriptionImpl) GetWeight() int32 {
	//TODO implement me
	panic("implement me")
}

func (b backendDescriptionImpl) GetPort() int {
	//TODO implement me
	panic("implement me")
}
