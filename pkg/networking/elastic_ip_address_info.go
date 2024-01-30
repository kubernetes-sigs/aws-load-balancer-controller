package networking

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
)

// ElasticIPAddressInfo wraps necessary information about a ElasticIPAddress.
type ElasticIPAddressInfo struct {
	// Elastic IP Address's ID.
	AllocationID string

	// ID of an address pool.
	PublicIPv4Pool string

	// Tags for EIP.
	Tags map[string]string
}

// NewRawElasticIPAddressInfo constructs new ElasticIPAddressInfo with raw ec2SDK's Address object.
func NewRawElasticIPAddressInfo(sdkEIP *ec2sdk.Address) ElasticIPAddressInfo {
	eipID := awssdk.StringValue(sdkEIP.AllocationId)
	publicIPv4Pool := awssdk.StringValue(sdkEIP.PublicIpv4Pool)

	tags := buildElasticIPAddressTags(sdkEIP)
	return ElasticIPAddressInfo{
		AllocationID:   eipID,
		PublicIPv4Pool: publicIPv4Pool,
		Tags:           tags,
	}
}

// buildElasticIPAddressTags generates the tags for securityGroup.
func buildElasticIPAddressTags(sdkEIP *ec2sdk.Address) map[string]string {
	eipTags := make(map[string]string, len(sdkEIP.Tags))
	for _, tag := range sdkEIP.Tags {
		eipTags[awssdk.StringValue(tag.Key)] = awssdk.StringValue(tag.Value)
	}
	return eipTags
}
