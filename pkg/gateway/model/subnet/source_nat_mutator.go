package subnet

import (
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

type sourceNATMutator struct {
	// networking.ValidateSourceNatPrefixForSubnetPair(sourceNatPrefix, ec2Subnet)
	validator func(sourceNatIpv6Prefix string, subnet ec2types.Subnet) error
}

func NewSourceNATMutator() Mutator {
	return &sourceNATMutator{
		validator: networking.ValidateSourceNatPrefixForSubnetPair,
	}
}

func (mutator *sourceNATMutator) Mutate(elbSubnets []*elbv2model.SubnetMapping, ec2Subnet []ec2types.Subnet, subnetConfig []elbv2gw.SubnetConfiguration) error {
	// We've already validated it's already all or nothing for source nat prefixes
	if elbSubnets == nil || len(subnetConfig) == 0 || subnetConfig[0].SourceNatIPv6Prefix == nil {
		return nil
	}

	if len(elbSubnets) != len(subnetConfig) {
		return errors.Errorf("Unable to assign Source NAT prefix because we have %+v subnets and %v prefixes", len(elbSubnets), len(subnetConfig))
	}

	for i, elbSubnet := range elbSubnets {
		validationErr := mutator.validator(*subnetConfig[i].SourceNatIPv6Prefix, ec2Subnet[i])
		if validationErr != nil {
			return validationErr
		}
		elbSubnet.SourceNatIpv6Prefix = subnetConfig[i].SourceNatIPv6Prefix
	}
	return nil
}
