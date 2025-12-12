package utils

import "strings"

// IsCommercialPartition returns true if the region is in the commercial AWS partition
func IsCommercialPartition(region string) bool {
	unsupportedPrefixes := []string{"cn-", "us-gov-", "us-iso", "eu-isoe-"}
	for _, prefix := range unsupportedPrefixes {
		if strings.HasPrefix(strings.ToLower(region), prefix) {
			return false
		}
	}
	return true
}
