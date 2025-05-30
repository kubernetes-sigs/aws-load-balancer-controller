package model

import (
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	v1 "sigs.k8s.io/gateway-api/apis/v1"
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

func getHighestPrecedenceHostname(hostnames []v1.Hostname) string {
	if len(hostnames) == 0 {
		return ""
	}

	highestHostname := hostnames[0]
	for _, hostname := range hostnames {
		if routeutils.GetHostnamePrecedenceOrder(string(hostname), string(highestHostname)) < 0 {
			highestHostname = hostname
		}
	}
	return string(highestHostname)
}

func sortRoutesByHostnamePrecedence(routes []routeutils.RouteDescriptor) {
	// sort routes based on their highest precedence hostname
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

		highestPrecedenceHostnameOne := getHighestPrecedenceHostname(hostnameOne)
		highestPrecedenceHostnameTwo := getHighestPrecedenceHostname(hostnameTwo)

		precedence := routeutils.GetHostnamePrecedenceOrder(highestPrecedenceHostnameOne, highestPrecedenceHostnameTwo)
		if precedence != 0 {
			return precedence < 0 // -1 means higher precedence
		}
		return false
	})
}
