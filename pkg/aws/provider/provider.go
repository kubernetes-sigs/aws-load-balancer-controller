package provider

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type AWSClientsProvider interface {
	GetEC2Client(ctx context.Context, operationName string) (*ec2.Client, error)
}
