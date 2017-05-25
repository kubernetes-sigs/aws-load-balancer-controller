package config

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/coreos/alb-ingress-controller/awsutil"
)

// resolveVPC attempt to resolve a VPC based on the provided subnets. This also acts as a way to
// validate provided subnets exist.
func (a *Annotations) resolveVPCValidateSubnets() error {
	VPCID, err := awsutil.Ec2svc.GetVPCID(a.Subnets)
	if err != nil {
		return fmt.Errorf("Subnets %s were invalid. Could not resolve to a VPC.", a.Subnets)
	}
	a.VPCID = VPCID

	// if there's a duplicate AZ, return a failure.
	in := ec2.DescribeSubnetsInput{
		SubnetIds: a.Subnets,
	}
	subs, err := awsutil.Ec2svc.DescribeSubnets(in)
	if err != nil {
		return err
	}
	subnetMap := make(map[string]string)
	for _, sub := range subs {
		if _, ok := subnetMap[*sub.AvailabilityZone]; ok {
			return fmt.Errorf("Subnets %s contained duplicate availability zone.", subs)
		}
		subnetMap[*sub.AvailabilityZone] = *sub.SubnetId
	}

	return nil
}

func (a *Annotations) validateSecurityGroups() error {
	in := ec2.DescribeSecurityGroupsInput{GroupIds: a.SecurityGroups}
	if _, err := awsutil.Ec2svc.DescribeSecurityGroups(in); err != nil {
		return err
	}
	return nil
}

func (a *Annotations) validateCertARN() error {
	if e := awsutil.ACMsvc.CertExists(a.CertificateArn); !e {
		if awsutil.IAMsvc.CertExists(a.CertificateArn) {
			return nil
		}
		return fmt.Errorf("ACM certificate ARN does not exist. ARN: %s", *a.CertificateArn)
	}
	return nil
}
