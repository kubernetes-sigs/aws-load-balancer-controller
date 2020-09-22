package services

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/shield"
	"github.com/aws/aws-sdk-go/service/shield/shieldiface"
)

type Shield interface {
	shieldiface.ShieldAPI

	Available() (bool, error)
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

func (c *defaultShield) Available() (bool, error) {
	req := &shield.GetSubscriptionStateInput{}
	resp, err := c.GetSubscriptionStateWithContext(context.Background(), req)
	if err != nil {
		return false, err
	}
	return *resp.SubscriptionState == "ACTIVE", nil
}
