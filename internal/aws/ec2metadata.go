package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
)

func GetVpcIDFromEC2Metadata(metadata *ec2metadata.EC2Metadata) (string, error) {
	mac, err := metadata.GetMetadata("mac")
	if err != nil {
		return "", err
	}
	vpcID, err := metadata.GetMetadata(fmt.Sprintf("network/interfaces/macs/%s/vpc-id", mac))
	if err != nil {
		return "", err
	}
	return vpcID, nil
}
