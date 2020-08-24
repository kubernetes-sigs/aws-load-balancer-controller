package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
)

type ACM interface {
	acmiface.ACMAPI
}

// NewEC2 constructs new EC2 implementation.
func NewACM(session *session.Session) ACM {
	return &defaultACM{
		ACMAPI: acm.New(session),
	}
}

type defaultACM struct {
	acmiface.ACMAPI
}
