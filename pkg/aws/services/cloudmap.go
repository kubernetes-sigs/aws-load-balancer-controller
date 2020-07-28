package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/aws/aws-sdk-go/service/servicediscovery/servicediscoveryiface"
)

type CloudMap interface {
	servicediscoveryiface.ServiceDiscoveryAPI
}

// NewCloudMap constructs new CloudMap implementation.
func NewCloudMap(session *session.Session) CloudMap {
	return &defaultCloudMap{
		ServiceDiscoveryAPI: servicediscovery.New(session),
	}
}

type defaultCloudMap struct {
	servicediscoveryiface.ServiceDiscoveryAPI
}
