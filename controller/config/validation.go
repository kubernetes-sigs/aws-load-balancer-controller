package config

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/coreos/alb-ingress-controller/awsutil"
)

// resolveVPC attempt to resolve a VPC based on the provided subnets. This also acts as a way to
// validate provided subnets exist.
func (a *Annotations) resolveVPC() error {
	VPCID, err := awsutil.Ec2svc.GetVPCID(a.Subnets)
	if err != nil {
		return fmt.Errorf("Subnets %s were invalid. Could not resolve to a VPC.", a.Subnets)
	}
	a.VPCID = VPCID
	return nil
}

func (a *AnnotationsT) validateSecurityGroups() error {
	in := ec2.DescribeSecurityGroupsInput{GroupIds: a.SecurityGroups}
	if _, err := awsutil.Ec2svc.DescribeSecurityGroups(in); err != nil {
		return err
	}
	return nil
}
