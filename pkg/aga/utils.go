package aga

import (
	"strings"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
)

// IsPortInRanges checks if a port is within any of the specified port ranges
func IsPortInRanges(port int32, portRanges []agamodel.PortRange) bool {
	for _, portRange := range portRanges {
		if portRange.FromPort <= port && port <= portRange.ToPort {
			return true
		}
	}
	return false
}

// IsAGAControllerEnabled checks if the AGA controller is both enabled via feature gate
// and if the region is in a partition that supports Global Accelerator
func IsAGAControllerEnabled(featureGates config.FeatureGates, region string) bool {
	// First check if AGA controller is enabled via feature gate
	if !featureGates.Enabled(config.AGAController) {
		return false
	}

	// Global Accelerator is only available in standard AWS partition
	// Not available in specialized AWS partitions
	regionLower := strings.ToLower(region)

	// Check for non-standard AWS partitions where Global Accelerator is not available
	unsupportedPrefixes := []string{
		"cn-",      // China regions
		"us-gov-",  // GovCloud regions
		"us-iso",   // ISO regions
		"eu-isoe-", // ISO-E regions
	}

	for _, prefix := range unsupportedPrefixes {
		if strings.HasPrefix(regionLower, prefix) {
			return false
		}
	}

	return true
}
