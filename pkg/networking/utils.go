package networking

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"inet.af/netaddr"
	"net/netip"
)

// TODO: replace netaddr package with built-in netip package once golang 1.18 released: https://pkg.go.dev/net/netip@master#Prefix

// ParseCIDRs will parse CIDRs in string format into parsed IPPrefix
func ParseCIDRs(cidrs []string) ([]netaddr.IPPrefix, error) {
	var ipPrefixes []netaddr.IPPrefix
	for _, cidr := range cidrs {
		ipPrefix, err := netaddr.ParseIPPrefix(cidr)
		if err != nil {
			return nil, err
		}
		ipPrefixes = append(ipPrefixes, ipPrefix)
	}
	return ipPrefixes, nil
}

// IsIPWithinCIDRs checks whether specific IP is in IPv4 CIDR or IPv6 CIDRs.
func IsIPWithinCIDRs(ip netaddr.IP, cidrs []netaddr.IPPrefix) bool {
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
func GetSubnetAssociatedIPv4CIDRs(subnet *ec2sdk.Subnet) ([]netip.Prefix, error) {
	if subnet.CidrBlock == nil {
		return nil, nil
	}
	cidrBlock := awssdk.StringValue(subnet.CidrBlock)
	ipv4CIDR, err := netip.ParsePrefix(cidrBlock)
	if err != nil {
		return nil, err
	}
	return []netip.Prefix{ipv4CIDR}, nil
}

// GetSubnetAssociatedIPv6CIDRs returns the IPv6 CIDRs associated with EC2 subnet
func GetSubnetAssociatedIPv6CIDRs(subnet *ec2sdk.Subnet) ([]netip.Prefix, error) {
	var ipv6CIDRs []netip.Prefix
	for _, cidrAssociation := range subnet.Ipv6CidrBlockAssociationSet {
		if awssdk.StringValue(cidrAssociation.Ipv6CidrBlockState.State) != ec2sdk.SubnetCidrBlockStateCodeAssociated {
			continue
		}
		cidrBlock := awssdk.StringValue(cidrAssociation.Ipv6CidrBlock)
		ipv6CIDR, err := netip.ParsePrefix(cidrBlock)
		if err != nil {
			return nil, err
		}
		ipv6CIDRs = append(ipv6CIDRs, ipv6CIDR)
	}
	return ipv6CIDRs, nil
}
