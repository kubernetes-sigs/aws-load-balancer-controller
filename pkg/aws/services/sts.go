package services

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
)

type STS interface {
	stsiface.STSAPI
	// AccountID returns AWS accountID for current IAM identity.
	AccountID(ctx context.Context) (string, error)
}

// NewSTS constructs new STS implementation.
func NewSTS(session *session.Session) STS {
	return &defaultSTS{
		STSAPI: sts.New(session),
	}
}

type defaultSTS struct {
	stsiface.STSAPI
}

func (c *defaultSTS) AccountID(ctx context.Context) (string, error) {
	resp, err := c.GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	return aws.StringValue(resp.Account), nil
}
