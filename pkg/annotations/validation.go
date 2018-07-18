package annotations

import (
	"fmt"
	"net"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albacm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albiam"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albwaf"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// Validates AWS resource references and other stateful configuration
type Validator interface {
	ResolveVPCValidateSubnets(a *Annotations) error
	ValidateSecurityGroups(a *Annotations) error
	ValidateCertARN(a *Annotations) error
	ValidateInboundCidrs(a *Annotations) error
	ValidateScheme(a *Annotations, ingressNamespace, ingressName string) bool
	ValidateWebACLId(a *Annotations) error
	ValidateSslPolicy(a *Annotations) error
}

type ConcreteValidator struct {
	// TODO Inject the AWS wrappers
}

func NewConcreteValidator() ConcreteValidator {
	return ConcreteValidator{}
}

// resolveVPC attempt to resolve a VPC based on the provided subnets. This also acts as a way to
// validate provided subnets exist.
func (v ConcreteValidator) ResolveVPCValidateSubnets(a *Annotations) error {
	VPCID, err := albec2.EC2svc.GetVPCID()
	if err != nil {
		return fmt.Errorf("subnets %s were invalid, could not resolve to a VPC: %s", log.Prettify(a.Subnets), err.Error())
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

func (v ConcreteValidator) ValidateSecurityGroups(a *Annotations) error {
	in := &ec2.DescribeSecurityGroupsInput{GroupIds: a.SecurityGroups}
	if _, err := albec2.EC2svc.DescribeSecurityGroups(in); err != nil {
		return err
	}
	return nil
}

func (v ConcreteValidator) ValidateCertARN(a *Annotations) error {
	if e := albacm.ACMsvc.CertExists(a.CertificateArn); !e {
		if albiam.IAMsvc.CertExists(a.CertificateArn) {
			return nil
		}
		return fmt.Errorf("ACM certificate ARN does not exist. ARN: %s", *a.CertificateArn)
	}
	return nil
}

func (v ConcreteValidator) ValidateInboundCidrs(a *Annotations) error {
	for _, cidr := range a.InboundCidrs {
		ip, _, err := net.ParseCIDR(*cidr)
		if err != nil {
			return err
		}

		if ip.To4() == nil {
			return fmt.Errorf("CIDR must use an IPv4 address: %v", *cidr)
		}
	}
	return nil
}

func (v ConcreteValidator) ValidateScheme(a *Annotations, ingressNamespace, ingressName string) bool {
	if config.RestrictScheme && *a.Scheme == "internet-facing" {
		allowed := util.IngressAllowedExternal(config.RestrictSchemeNamespace, ingressNamespace, ingressName)
		if !allowed {
			return false
		}
	}
	return true
}

func (v ConcreteValidator) ValidateWebACLId(a *Annotations) error {
	if success, err := albwaf.WAFRegionalsvc.WebACLExists(a.WebACLId); !success {
		return fmt.Errorf("Web ACL Id does not exist. Id: %s, error: %s", *a.WebACLId, err.Error())
	}
	return nil
}

func (v ConcreteValidator) ValidateSslPolicy(a *Annotations) error {
	in := &elbv2.DescribeSSLPoliciesInput{
		Names: []*string{
			a.SslPolicy,
		},
	}
	if _, err := albelbv2.ELBV2svc.DescribeSSLPolicies(in); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elbv2.ErrCodeSSLPolicyNotFoundException:
				return fmt.Errorf("%s: %s", elbv2.ErrCodeSSLPolicyNotFoundException, aerr.Error())
			default:
				return fmt.Errorf("Error: %s", aerr.Error())
			}
		} else {
			return fmt.Errorf("Error: %s", aerr.Error())
		}
	}
	return nil
}
