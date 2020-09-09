package services

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

type EC2Metadata interface {
	Region() (string, error)
	VpcID() (string, error)
}

// NewEC2Metadata constructs new EC2Metadata implementation.
func NewEC2Metadata(session *session.Session) EC2Metadata {
	return &defaultEC2Metadata{
		EC2Metadata: ec2metadata.New(session),
	}
}

type defaultEC2Metadata struct {
	*ec2metadata.EC2Metadata
}

func (c *defaultEC2Metadata) VpcID() (string, error) {
	mac, err := c.GetMetadata("mac")
	if err != nil {
		return "", err
	}
	vpcID, err := c.GetMetadata(fmt.Sprintf("network/interfaces/macs/%s/vpc-id", mac))
	if err != nil {
		return "", err
	}
	return vpcID, nil
}
