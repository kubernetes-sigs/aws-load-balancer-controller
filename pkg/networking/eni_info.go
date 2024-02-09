package networking

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// ENIInfo wraps necessary information about a ENI.
type ENIInfo struct {
	// ENI's ID
	NetworkInterfaceID string

	// SecurityGroups on ENI
	SecurityGroups []string
}

func buildENIInfoViaENI(eni *ec2.NetworkInterface) ENIInfo {
	sgIDs := make([]string, 0, len(eni.Groups))
	for _, group := range eni.Groups {
		sgIDs = append(sgIDs, awssdk.StringValue(group.GroupId))
	}
	return ENIInfo{
		NetworkInterfaceID: awssdk.StringValue(eni.NetworkInterfaceId),
		SecurityGroups:     sgIDs,
	}
}

func buildENIInfoViaInstanceENI(eni *ec2.InstanceNetworkInterface) ENIInfo {
	sgIDs := make([]string, 0, len(eni.Groups))
	for _, group := range eni.Groups {
		sgIDs = append(sgIDs, awssdk.StringValue(group.GroupId))
	}
	return ENIInfo{
		NetworkInterfaceID: awssdk.StringValue(eni.NetworkInterfaceId),
		SecurityGroups:     sgIDs,
	}
}
