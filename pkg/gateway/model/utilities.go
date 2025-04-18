package model

import (
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"strings"
)

func isIPv6Supported(ipAddressType elbv2model.IPAddressType) bool {
	switch ipAddressType {
	case elbv2model.IPAddressTypeDualStack, elbv2model.IPAddressTypeDualStackWithoutPublicIPV4:
		return true
	default:
		return false
	}
}

// TODO - Refactor?
func isIPv6CIDR(cidr string) bool {
	return strings.Contains(cidr, ":")
}
