package networking

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"regexp"
)

const (
	// the raw permission description
	labelKeyRawDescription = "raw/description"
)

// SecurityGroupInfo wraps necessary information about a SecurityGroup.
type SecurityGroupInfo struct {
	// securityGroup's ID.
	SecurityGroupID string

	// Ingress permission for securityGroup.
	IngressPermissions []IPPermissionInfo
}

type IPPermissionInfo struct {
	// the aws sdk permission
	Permission ec2sdk.IpPermission

	// a set of computed labels for IPPermission.
	// we can use labels to select the rules we want to manage.
	Labels map[string]string
}

func buildSecurityGroupInfo(sdkSG *ec2sdk.SecurityGroup) SecurityGroupInfo {
	sgID := awssdk.StringValue(sdkSG.GroupId)
	var ingressPermissions []IPPermissionInfo
	for _, sdkPermission := range sdkSG.IpPermissions {
		for _, expandedPermission := range expandSDKIPPermission(*sdkPermission) {
			ingressPermissions = append(ingressPermissions, buildIPPermissionInfo(expandedPermission))
		}
	}
	return SecurityGroupInfo{
		SecurityGroupID:    sgID,
		IngressPermissions: ingressPermissions,
	}
}

func buildIPPermissionInfo(sdkPermission ec2sdk.IpPermission) IPPermissionInfo {
	if len(sdkPermission.IpRanges) == 1 {
		return IPPermissionInfo{
			Permission: sdkPermission,
			Labels:     buildIPPermissionLabelForDescription(awssdk.StringValue(sdkPermission.IpRanges[0].Description)),
		}
	}
	if len(sdkPermission.Ipv6Ranges) == 1 {
		return IPPermissionInfo{
			Permission: sdkPermission,
			Labels:     buildIPPermissionLabelForDescription(awssdk.StringValue(sdkPermission.Ipv6Ranges[0].Description)),
		}
	}
	if len(sdkPermission.PrefixListIds) == 1 {
		return IPPermissionInfo{
			Permission: sdkPermission,
			Labels:     buildIPPermissionLabelForDescription(awssdk.StringValue(sdkPermission.PrefixListIds[0].Description)),
		}
	}
	if len(sdkPermission.UserIdGroupPairs) == 1 {
		return IPPermissionInfo{
			Permission: sdkPermission,
			Labels:     buildIPPermissionLabelForDescription(awssdk.StringValue(sdkPermission.UserIdGroupPairs[0].Description)),
		}
	}
	return IPPermissionInfo{
		Permission: sdkPermission,
		Labels:     nil,
	}
}

// expandSDKIPPermission will expand the IPPermission so that each permission only contain single entry.
// EC2 api automatically group IPPermissions, so we need to expand first before further processing.
func expandSDKIPPermission(sdkPermission ec2sdk.IpPermission) []ec2sdk.IpPermission {
	var expandedPermissions []ec2sdk.IpPermission
	base := ec2sdk.IpPermission{
		FromPort:   sdkPermission.FromPort,
		ToPort:     sdkPermission.ToPort,
		IpProtocol: sdkPermission.IpProtocol,
	}

	for _, ipRange := range sdkPermission.IpRanges {
		perm := base
		perm.IpRanges = []*ec2sdk.IpRange{ipRange}
		expandedPermissions = append(expandedPermissions, perm)
	}

	for _, ipRange := range sdkPermission.Ipv6Ranges {
		perm := base
		perm.Ipv6Ranges = []*ec2sdk.Ipv6Range{ipRange}
		expandedPermissions = append(expandedPermissions, perm)
	}
	for _, prefixListID := range sdkPermission.PrefixListIds {
		perm := base
		perm.PrefixListIds = []*ec2sdk.PrefixListId{prefixListID}
		expandedPermissions = append(expandedPermissions, perm)
	}

	for _, ug := range sdkPermission.UserIdGroupPairs {
		perm := base
		perm.UserIdGroupPairs = []*ec2sdk.UserIdGroupPair{ug}
		expandedPermissions = append(expandedPermissions, perm)
	}

	if len(expandedPermissions) == 0 {
		expandedPermissions = append(expandedPermissions, sdkPermission)
	}
	return expandedPermissions
}

var commaSeparatedKVPairPattern = regexp.MustCompile(`(?P<key>[^\s,=]+)=(?P<value>[^\s,=]+)(?:,|$)`)

// buildIPPermissionLabelForDescription constructs a set of labels parsed from IPPermission description
func buildIPPermissionLabelForDescription(description string) map[string]string {
	labels := map[string]string{labelKeyRawDescription: description}
	for _, groups := range commaSeparatedKVPairPattern.FindAllStringSubmatch(description, -1) {
		labels[groups[1]] = groups[2]
	}
	return labels
}
