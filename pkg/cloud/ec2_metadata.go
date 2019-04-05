package cloud

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

type EC2Metadata interface {
	VpcID() (string, error)
	Region() (string, error)
}

func NewEC2Metadata(session *session.Session) EC2Metadata {
	return &defaultEC2Metadata{
		ec2metadata.New(session),
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
