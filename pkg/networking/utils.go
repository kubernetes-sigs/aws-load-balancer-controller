package networking

import (
	"context"
	"net/netip"

	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
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

func IsMtlsNotSupportedForSelectedSubnets(ctx context.Context, subnets []*ec2sdk.Subnet, azInfoProvider *defaultAZInfoProvider) (bool, string) {
	const (
		zoneTypeLocalZone      string = "local-zone"
		zoneTypeWavelengthZone string = "wavelength-zone"
		outPostSubnet          string = "outpost"
	)

	isAnySubnetInLocalZoneOrOutPostOrWaveLengthZone := false
	zoneType := ""
	var azIds []string

	for _, subnet := range subnets {
		if subnet.OutpostArn != nil && len(*subnet.OutpostArn) != 0 {
			isAnySubnetInLocalZoneOrOutPostOrWaveLengthZone = true
			zoneType = outPostSubnet
			break
		}

		azIds = append(azIds, awssdk.StringValue(subnet.AvailabilityZoneId))

	}

	azsInfo, _ := azInfoProvider.FetchAZInfos(ctx, azIds)

	for _, azInfo := range azsInfo {
		if azInfo.ZoneType != nil && (awssdk.StringValue(azInfo.ZoneType) == zoneTypeLocalZone || awssdk.StringValue(azInfo.ZoneType) == zoneTypeWavelengthZone) {
			isAnySubnetInLocalZoneOrOutPostOrWaveLengthZone = true
			zoneType = zoneTypeLocalZone
			if awssdk.StringValue(azInfo.ZoneType) == zoneTypeWavelengthZone {
				zoneType = zoneTypeWavelengthZone
			}
			break
		}
	}

	return isAnySubnetInLocalZoneOrOutPostOrWaveLengthZone, zoneType
}
