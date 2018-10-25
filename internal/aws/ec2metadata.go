package aws

import "github.com/aws/aws-sdk-go/aws/ec2metadata"

type EC2MetadataAPI interface {
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
}

func (c *Cloud) GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error) {
	return c.ec2metadata.GetInstanceIdentityDocument()
}
