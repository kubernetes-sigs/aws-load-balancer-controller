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

type ipv6Mutator struct {
	// networking.GetSubnetAssociatedIPv6CIDRs(ec2Subnet)
	prefixResolver func(subnet ec2types.Subnet) ([]netip.Prefix, error)

	// networking.FilterIPsWithinCIDRs([]netip.Addr{ipv4Address}, subnetIPv4CIDRs)
	ipCidrFilter func(ips []netip.Addr, cidrs []netip.Prefix) []netip.Addr
}

func NewIPv6Mutator() Mutator {
	return &ipv6Mutator{
		prefixResolver: networking.GetSubnetAssociatedIPv6CIDRs,
		ipCidrFilter:   networking.FilterIPsWithinCIDRs,
	}
}

func (mutator *ipv6Mutator) Mutate(elbSubnets []*elbv2model.SubnetMapping, ec2Subnets []ec2types.Subnet, subnetConfig []elbv2gw.SubnetConfiguration) error {

	// We've already validated it's already all or nothing for ipv6 allocations.
	if elbSubnets == nil || len(subnetConfig) == 0 || subnetConfig[0].IPv6Allocation == nil {
		return nil
	}

	if len(elbSubnets) != len(subnetConfig) {
		return errors.Errorf("Unable to assign IPv6 addresses we have %+v subnets and %v addresses", len(elbSubnets), len(subnetConfig))
	}

	ipv6Addrs := make([]netip.Addr, 0)
	for _, cfg := range subnetConfig {
		ipv6Address, err := netip.ParseAddr(*cfg.IPv6Allocation)
		if err != nil {
			return errors.Errorf("IPv6 addresses must be valid IP address: %v", *cfg.IPv6Allocation)
		}
		if !ipv6Address.Is6() {
			return errors.Errorf("IPv6 addresses must be valid IPv6 address: %v", *cfg.IPv6Allocation)
		}

		ipv6Addrs = append(ipv6Addrs, ipv6Address)
	}

	for i, elbSubnet := range elbSubnets {
		ec2Subnet := ec2Subnets[i]
		subnetIPv6CIDRs, err := mutator.prefixResolver(ec2Subnet)
		if err != nil {
			return err
		}
		ipv6AddressesWithinSubnet := mutator.ipCidrFilter(ipv6Addrs, subnetIPv6CIDRs)
		if len(ipv6AddressesWithinSubnet) != 1 {
			return errors.Errorf("expect one IPv6 address configured for subnet: %v", awssdk.ToString(ec2Subnet.SubnetId))
		}
		elbSubnet.IPv6Address = awssdk.String(ipv6AddressesWithinSubnet[0].String())
	}
	return nil
}
