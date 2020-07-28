package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
)

type ELBV2 interface {
	elbv2iface.ELBV2API
}

// NewELBV2 constructs new ELBV2 implementation.
func NewELBV2(session *session.Session) ELBV2 {
	return &defaultELBV2{
		ELBV2API: elbv2.New(session),
	}
}

type defaultELBV2 struct {
	elbv2iface.ELBV2API
}
