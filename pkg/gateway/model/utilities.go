package model

import (
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sort"
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

func sortRoutesByHostnamePrecedence(routes []routeutils.RouteDescriptor) {
	sort.SliceStable(routes, func(i, j int) bool {
		hostnameOne := routes[i].GetHostnames()
		hostnameTwo := routes[j].GetHostnames()

		if len(hostnameOne) == 0 && len(hostnameTwo) == 0 {
			return false
		}
		if len(hostnameOne) == 0 {
			return false
		}
		if len(hostnameTwo) == 0 {
			return true
		}
		precedence := routeutils.GetHostnamePrecedenceOrder(string(hostnameOne[0]), string(hostnameTwo[0]))
		if precedence != 0 {
			return precedence < 0 // -1 means higher precedence
		}
		return false
	})
}
