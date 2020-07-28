package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

type EC2 interface {
	ec2iface.EC2API
}

// NewEC2 constructs new EC2 implementation.
func NewEC2(session *session.Session) EC2 {
	return &defaultEC2{
		EC2API: ec2.New(session),
	}
}

type defaultEC2 struct {
	ec2iface.EC2API
}
