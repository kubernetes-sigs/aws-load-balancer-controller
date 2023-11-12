package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/eks/eksiface"
)

type EKS interface {
	eksiface.EKSAPI
}

// NewEKS constructs new EKS implementation.
func NewEKS(session *session.Session) EKS {
	return &defaultEKS{
		EKSAPI: eks.New(session),
	}
}

// default implementation for EKS.
type defaultEKS struct {
	eksiface.EKSAPI
}
