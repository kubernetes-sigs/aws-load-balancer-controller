package cloud

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

type Cloud interface {
	ELBV2() ELBV2
	EC2() EC2
	RGT() RGT

	ClusterName() string
	VpcID() string
}

type defaultCloud struct {
	config Config

	elbv2 ELBV2
	ec2   EC2
	rgt   RGT
}

// Initialize the global AWS clients.
func New(cfg Config) (Cloud, error) {
	session, err := session.NewSession(&aws.Config{MaxRetries: aws.Int(cfg.APIMaxRetries)})
	if err != nil {
		return nil, err
	}
	metadata := NewEC2Metadata(session)

	if len(cfg.VpcID) == 0 {
		vpcID, err := metadata.VpcID()
		if err != nil {
			return nil, fmt.Errorf("failed to introspect vpcID from ec2Metadata due to %v, specify --aws-vpc-id instead if ec2Metadata is unavailable", err)
		}
		cfg.VpcID = vpcID
	}
	if len(cfg.Region) == 0 {
		region, err := metadata.Region()
		if err != nil {
			return nil, fmt.Errorf("failed to introspect region from ec2Metadata due to %v, specify --aws-region instead if ec2Metadata is unavailable", err)
		}
		cfg.Region = region
	}
	session = session.Copy(&aws.Config{Region: aws.String(cfg.Region)})
	return &defaultCloud{
		config: cfg,
		elbv2:  NewELBV2(session),
		ec2:    NewEC2(session),
		rgt:    NewRGT(session),
	}, nil
}

func (c *defaultCloud) ELBV2() ELBV2 {
	return c.elbv2
}

func (c *defaultCloud) EC2() EC2 {
	return c.ec2
}

func (c *defaultCloud) RGT() RGT {
	return c.rgt
}

func (c *defaultCloud) ClusterName() string {
	return c.config.ClusterName
}

func (c *defaultCloud) VpcID() string {
	return c.config.VpcID
}
