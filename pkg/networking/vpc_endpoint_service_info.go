package networking

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
)

// VPCEndpointServiceInfo wraps necessary information about an Endpoint Service.
type VPCEndpointServiceInfo struct {
	// The ID of the endpoint service.
	ServiceID string

	// whether requests from service consumers to create an endpoint to the service must be accepted
	AcceptanceRequired bool

	NetworkLoadBalancerArns []string

	PrivateDNSName *string

	BaseEndpointDnsNames []string
	// +optional
	Tags map[string]string
}

// NewRawVPCEndpointServiceInfo constructs new VPCEndpointServiceInfo with raw ec2SDK's ServiceConfiguration object.
func NewRawVPCEndpointServiceInfo(sdkES *ec2sdk.ServiceConfiguration) VPCEndpointServiceInfo {
	esID := awssdk.StringValue(sdkES.ServiceId)

	tags := make(map[string]string, len(sdkES.Tags))
	for _, tag := range sdkES.Tags {
		tags[awssdk.StringValue(tag.Key)] = awssdk.StringValue(tag.Value)
	}
	return VPCEndpointServiceInfo{
		ServiceID:               esID,
		AcceptanceRequired:      awssdk.BoolValue(sdkES.AcceptanceRequired),
		NetworkLoadBalancerArns: awssdk.StringValueSlice(sdkES.NetworkLoadBalancerArns),
		PrivateDNSName:          sdkES.PrivateDnsName,
		BaseEndpointDnsNames:    awssdk.StringValueSlice(sdkES.BaseEndpointDnsNames),
		Tags:                    tags,
	}
}

// VPCEndpointServiceInfo wraps necessary information about Endpoint Service Permissions.
type VPCEndpointServicePermissionsInfo struct {
	// The allowed principals for the endpoint service
	AllowedPrincipals []string

	// The service these principals apply to
	ServiceId string
}

func NewRawVPCEndpointServicePermissionsInfo(sdkPermissions *ec2sdk.DescribeVpcEndpointServicePermissionsOutput) VPCEndpointServicePermissionsInfo {
	var principals []string
	for _, p := range sdkPermissions.AllowedPrincipals {
		principals = append(principals, *p.Principal)
	}

	return VPCEndpointServicePermissionsInfo{
		AllowedPrincipals: principals,
	}
}
