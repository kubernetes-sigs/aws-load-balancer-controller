package networking

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// ENIInfo wraps necessary information about a ENI.
type ENIInfo struct {
	// ENI's ID
	NetworkInterfaceID string

	// SecurityGroups on ENI
	SecurityGroups []string
}

func buildENIInfoViaENI(eni ec2types.NetworkInterface) ENIInfo {
	sgIDs := make([]string, 0, len(eni.Groups))
	for _, group := range eni.Groups {
		sgIDs = append(sgIDs, awssdk.ToString(group.GroupId))
	}
	return ENIInfo{
		NetworkInterfaceID: awssdk.ToString(eni.NetworkInterfaceId),
		SecurityGroups:     sgIDs,
	}
}

func buildENIInfoViaInstanceENI(eni ec2types.InstanceNetworkInterface) ENIInfo {
	sgIDs := make([]string, 0, len(eni.Groups))
	for _, group := range eni.Groups {
		sgIDs = append(sgIDs, awssdk.ToString(group.GroupId))
	}
	return ENIInfo{
		NetworkInterfaceID: awssdk.ToString(eni.NetworkInterfaceId),
		SecurityGroups:     sgIDs,
	}
}
