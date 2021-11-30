package networking

import "inet.af/netaddr"

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
