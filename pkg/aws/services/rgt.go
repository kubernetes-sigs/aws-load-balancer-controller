package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"
)

type RGT interface {
	resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
}

// NewRGT constructs new RGT implementation.
func NewRGT(session *session.Session) RGT {
	return &defaultRGT{
		ResourceGroupsTaggingAPIAPI: resourcegroupstaggingapi.New(session),
	}
}

type defaultRGT struct {
	resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
}
