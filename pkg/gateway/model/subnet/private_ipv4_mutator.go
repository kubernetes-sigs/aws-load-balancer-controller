package subnet

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
	"net/netip"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

type privateIPv4Mutator struct {
	// networking.GetSubnetAssociatedIPv4CIDRs(ec2Subnet)
	prefixResolver func(subnet ec2types.Subnet) ([]netip.Prefix, error)

	// networking.FilterIPsWithinCIDRs([]netip.Addr{ipv4Address}, subnetIPv4CIDRs)
	ipCidrFilter func(ips []netip.Addr, cidrs []netip.Prefix) []netip.Addr
}

func NewPrivateIPv4Mutator() Mutator {
	return &privateIPv4Mutator{
		prefixResolver: networking.GetSubnetAssociatedIPv4CIDRs,
		ipCidrFilter:   networking.FilterIPsWithinCIDRs,
	}
}

func (mutator *privateIPv4Mutator) Mutate(elbSubnets []*elbv2model.SubnetMapping, ec2Subnets []ec2types.Subnet, subnetConfig []elbv2gw.SubnetConfiguration) error {
	// We've already validated it's already all or nothing for private ipv4 allocations.
	if elbSubnets == nil || len(subnetConfig) == 0 || subnetConfig[0].PrivateIPv4Allocation == nil {
		return nil
	}

	if len(elbSubnets) != len(subnetConfig) {
		return errors.Errorf("Unable to assign private IPv4 addresses we have %+v subnets and %v addresses", len(elbSubnets), len(subnetConfig))
	}

	ipv4Addrs := make([]netip.Addr, 0)
	for _, cfg := range subnetConfig {
		ipv4Address, err := netip.ParseAddr(*cfg.PrivateIPv4Allocation)
		if err != nil {
			return errors.Errorf("private IPv4 addresses must be valid IP address: %v", *cfg.PrivateIPv4Allocation)
		}
		if !ipv4Address.Is4() {
			return errors.Errorf("private IPv4 addresses must be valid IPv4 address: %v", *cfg.PrivateIPv4Allocation)
		}
		ipv4Addrs = append(ipv4Addrs, ipv4Address)
	}

	for i, elbSubnet := range elbSubnets {
		ec2Subnet := ec2Subnets[i]
		subnetIPv4CIDRs, err := mutator.prefixResolver(ec2Subnet)
		if err != nil {
			return err
		}
		ipv4AddressesWithinSubnet := mutator.ipCidrFilter(ipv4Addrs, subnetIPv4CIDRs)
		if len(ipv4AddressesWithinSubnet) != 1 {
			return errors.Errorf("expect one private IPv4 address configured for subnet: %v", awssdk.ToString(ec2Subnet.SubnetId))
		}
		elbSubnet.PrivateIPv4Address = awssdk.String(ipv4AddressesWithinSubnet[0].String())
	}
	return nil
}
