package services

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/shield"
	"github.com/aws/aws-sdk-go/service/shield/shieldiface"
)

type Shield interface {
	shieldiface.ShieldAPI
}

// NewShield constructs new Shield implementation.
func NewShield(session *session.Session) Shield {
	return &defaultShield{
		// shield is only available as a global API in us-east-1.
		ShieldAPI: shield.New(session, aws.NewConfig().WithRegion("us-east-1")),
	}
}

// default implementation for Shield.
type defaultShield struct {
	shieldiface.ShieldAPI
}
