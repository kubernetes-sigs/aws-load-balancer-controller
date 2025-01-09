package networking

import (
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
	"net/netip"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"strings"
)

// ParseCIDRs will parse CIDRs in string format into parsed IPPrefix
func ParseCIDRs(cidrs []string) ([]netip.Prefix, error) {
	var ipPrefixes []netip.Prefix
	for _, cidr := range cidrs {
		ipPrefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, err
		}
		ipPrefixes = append(ipPrefixes, ipPrefix)
	}
	return ipPrefixes, nil
}

// IsIPWithinCIDRs checks whether specific IP is in IPv4 CIDR or IPv6 CIDRs.
func IsIPWithinCIDRs(ip netip.Addr, cidrs []netip.Prefix) bool {
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// FilterIPsWithinCIDRs returns IP addresses that were within specified CIDRs.
func FilterIPsWithinCIDRs(ips []netip.Addr, cidrs []netip.Prefix) []netip.Addr {
	var ipsWithinCIDRs []netip.Addr
	for _, ip := range ips {
		for _, cidr := range cidrs {
			if cidr.Contains(ip) {
				ipsWithinCIDRs = append(ipsWithinCIDRs, ip)
				break
			}
		}
	}
	return ipsWithinCIDRs
}

// GetSubnetAssociatedIPv4CIDRs returns the IPv4 CIDRs associated with EC2 subnet
func GetSubnetAssociatedIPv4CIDRs(subnet ec2types.Subnet) ([]netip.Prefix, error) {
	if subnet.CidrBlock == nil {
		return nil, nil
	}
	cidrBlock := awssdk.ToString(subnet.CidrBlock)
	ipv4CIDR, err := netip.ParsePrefix(cidrBlock)
	if err != nil {
		return nil, err
	}
	return []netip.Prefix{ipv4CIDR}, nil
}

// GetSubnetAssociatedIPv6CIDRs returns the IPv6 CIDRs associated with EC2 subnet
func GetSubnetAssociatedIPv6CIDRs(subnet ec2types.Subnet) ([]netip.Prefix, error) {
	var ipv6CIDRs []netip.Prefix
	for _, cidrAssociation := range subnet.Ipv6CidrBlockAssociationSet {
		if cidrAssociation.Ipv6CidrBlockState.State != ec2types.SubnetCidrBlockStateCodeAssociated {
			continue
		}
		cidrBlock := awssdk.ToString(cidrAssociation.Ipv6CidrBlock)
		ipv6CIDR, err := netip.ParsePrefix(cidrBlock)
		if err != nil {
			return nil, err
		}
		ipv6CIDRs = append(ipv6CIDRs, ipv6CIDR)
	}
	return ipv6CIDRs, nil
}

// ValidateEnablePrefixForIpv6SourceNat function returns the validation error if error exists for EnablePrefixForIpv6SourceNat annotation value
func ValidateEnablePrefixForIpv6SourceNat(EnablePrefixForIpv6SourceNat string, ipAddressType elbv2model.IPAddressType, ec2Subnets []ec2types.Subnet) error {
	if EnablePrefixForIpv6SourceNat != string(elbv2model.EnablePrefixForIpv6SourceNatOn) && EnablePrefixForIpv6SourceNat != string(elbv2model.EnablePrefixForIpv6SourceNatOff) {
		return errors.Errorf(fmt.Sprintf("Invalid enable-prefix-for-ipv6-source-nat value: %v. Valid values are ['on', 'off'].", EnablePrefixForIpv6SourceNat))
	}

	if EnablePrefixForIpv6SourceNat != string(elbv2model.EnablePrefixForIpv6SourceNatOn) {
		return nil
	}

	if ipAddressType == elbv2model.IPAddressTypeIPV4 {
		return errors.Errorf(fmt.Sprintf("enable-prefix-for-ipv6-source-nat annotation is only applicable to Network Load Balancers using Dualstack IP address type."))
	}
	var subnetsWithoutIPv6CIDR []string

	for _, subnet := range ec2Subnets {
		subnetIPv6CIDRs, err := GetSubnetAssociatedIPv6CIDRs(subnet)
		if err != nil {
			return errors.Errorf(fmt.Sprintf("%v", err))
		}
		if len(subnetIPv6CIDRs) < 1 {
			subnetsWithoutIPv6CIDR = append(subnetsWithoutIPv6CIDR, awssdk.ToString(subnet.SubnetId))

		}
	}
	if len(subnetsWithoutIPv6CIDR) > 0 {
		return errors.Errorf(fmt.Sprintf("To enable prefix for source NAT, all associated subnets must have an IPv6 CIDR. Subnets without IPv6 CIDR: %v.", subnetsWithoutIPv6CIDR))
	}

	return nil
}

// ValidateSourceNatPrefixes function returns the validation error if error exists for sourceNatIpv6Prefixes annotation value
func ValidateSourceNatPrefixes(sourceNatIpv6Prefixes []string, ipAddressType elbv2model.IPAddressType, isPrefixForIpv6SourceNatEnabled bool, ec2Subnets []ec2types.Subnet) error {
	const requiredPrefixLengthForSourceNatCidr = "80"
	if ipAddressType != elbv2model.IPAddressTypeDualStack {
		return errors.Errorf("source-nat-ipv6-prefixes annotation can only be set for Network Load Balancers using Dualstack IP address type.")
	}
	if !isPrefixForIpv6SourceNatEnabled {
		return errors.Errorf("source-nat-ipv6-prefixes annotation is only applicable if enable-prefix-for-ipv6-source-nat annotation is set to on.")
	}

	if len(sourceNatIpv6Prefixes) != len(ec2Subnets) {
		return errors.Errorf(fmt.Sprintf("Number of values in source-nat-ipv6-prefixes (%d) must match the number of subnets (%d).", len(sourceNatIpv6Prefixes), len(ec2Subnets)))
	}
	for idx, sourceNatIpv6Prefix := range sourceNatIpv6Prefixes {
		var subnet = ec2Subnets[idx]
		var sourceNatIpv6PrefixParsedList []netip.Addr
		if sourceNatIpv6Prefix != elbv2model.SourceNatIpv6PrefixAutoAssigned {
			subStrings := strings.Split(sourceNatIpv6Prefix, "/")
			if len(subStrings) < 2 {
				return errors.Errorf(fmt.Sprintf("Invalid value in source-nat-ipv6-prefixes: %v.", sourceNatIpv6Prefix))
			}
			var ipAddressPart = subStrings[0]
			var prefixLengthPart = subStrings[1]
			if prefixLengthPart != requiredPrefixLengthForSourceNatCidr {
				return errors.Errorf(fmt.Sprintf("Invalid value in source-nat-ipv6-prefixes: %v. Prefix length must be %v, but %v is specified.", sourceNatIpv6Prefix, requiredPrefixLengthForSourceNatCidr, prefixLengthPart))
			}
			sourceNatIpv6PrefixNetIpParsed, err := netip.ParseAddr(ipAddressPart)
			if err != nil {
				return errors.Errorf(fmt.Sprintf("Invalid value in source-nat-ipv6-prefixes: %v. Value must be a valid IPv6 CIDR.", sourceNatIpv6Prefix))
			}
			sourceNatIpv6PrefixParsedList = append(sourceNatIpv6PrefixParsedList, sourceNatIpv6PrefixNetIpParsed)
			if !sourceNatIpv6PrefixNetIpParsed.Is6() {
				return errors.Errorf(fmt.Sprintf("Invalid value in source-nat-ipv6-prefixes: %v. Value must be a valid IPv6 CIDR.", sourceNatIpv6Prefix))
			}
			subnetIPv6CIDRs, err := GetSubnetAssociatedIPv6CIDRs(subnet)
			if err != nil {
				return errors.Errorf(fmt.Sprintf("Subnet has invalid IPv6 CIDRs: %v. Subnet must have valid IPv6 CIDRs.", subnetIPv6CIDRs))
			}
			sourceNatIpv6PrefixWithinSubnet := FilterIPsWithinCIDRs(sourceNatIpv6PrefixParsedList, subnetIPv6CIDRs)
			if len(sourceNatIpv6PrefixWithinSubnet) != 1 {
				return errors.Errorf(fmt.Sprintf("Invalid value in source-nat-ipv6-prefixes: %v. Value must be within subnet CIDR range: %v.", sourceNatIpv6Prefix, subnetIPv6CIDRs))
			}
		}
	}

	return nil
}
