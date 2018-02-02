package annotations

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/coreos/alb-ingress-controller/pkg/aws/acm"
	albec2 "github.com/coreos/alb-ingress-controller/pkg/aws/ec2"
	"github.com/coreos/alb-ingress-controller/pkg/aws/iam"
	"github.com/coreos/alb-ingress-controller/pkg/config"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

// resolveVPC attempt to resolve a VPC based on the provided subnets. This also acts as a way to
// validate provided subnets exist.
func (a *Annotations) resolveVPCValidateSubnets() error {
	VPCID, err := albec2.EC2svc.GetVPCID()
	if err != nil {
		return fmt.Errorf("subnets %s were invalid, could not resolve to a VPC", a.Subnets)
	}
	a.VPCID = VPCID

	// if there's a duplicate AZ, return a failure.
	in := &ec2.DescribeSubnetsInput{
		SubnetIds: a.Subnets,
	}
	describeSubnetsOutput, err := albec2.EC2svc.DescribeSubnets(in)
	if err != nil {
		return err
	}
	subnetMap := make(map[string]string)
	for _, sub := range describeSubnetsOutput.Subnets {
		if _, ok := subnetMap[*sub.AvailabilityZone]; ok {
			return fmt.Errorf("subnets %s contained duplicate availability zone", describeSubnetsOutput.Subnets)
		}
		subnetMap[*sub.AvailabilityZone] = *sub.SubnetId
	}

	return nil
}

func (a *Annotations) validateSecurityGroups() error {
	in := &ec2.DescribeSecurityGroupsInput{GroupIds: a.SecurityGroups}
	if _, err := albec2.EC2svc.DescribeSecurityGroups(in); err != nil {
		return err
	}
	return nil
}

func (a *Annotations) validateCertARN() error {
	if e := acm.ACMsvc.CertExists(a.CertificateArn); !e {
		if iam.IAMsvc.CertExists(a.CertificateArn) {
			return nil
		}
		return fmt.Errorf("ACM certificate ARN does not exist. ARN: %s", *a.CertificateArn)
	}
	return nil
}

func (a *Annotations) ValidateScheme(ingressNamespace, ingressName string) bool {
	if config.RestrictScheme && *a.Scheme == "internet-facing" {
		allowed := util.IngressAllowedExternal(config.RestrictSchemeNamespace, ingressNamespace, ingressName)
		if !allowed {
			return false
		}
	}
	return true
}
