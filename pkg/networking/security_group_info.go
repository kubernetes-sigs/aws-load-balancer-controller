package networking

import (
	"encoding/json"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"k8s.io/apimachinery/pkg/util/sets"
	"regexp"
	"strings"
)

const (
	// the raw permission description
	labelKeyRawDescription = "raw/description"
)

// SecurityGroupInfo wraps necessary information about a SecurityGroup.
type SecurityGroupInfo struct {
	// SecurityGroup's ID.
	SecurityGroupID string

	// Ingress permission for securityGroup.
	Ingress []IPPermissionInfo

	// Tags for securityGroup.
	Tags map[string]string
}

type IPPermissionInfo struct {
	// the aws sdk permission
	Permission ec2sdk.IpPermission

	// a set of computed labels for IPPermission.
	// we can use labels to select the rules we want to manage.
	Labels map[string]string
}

// HashCode returns the hashcode for the IPPermissionInfo.
// The hashCode should only include the actual permission but not labels/descriptions.
func (perm *IPPermissionInfo) HashCode() string {
	protocol := awssdk.StringValue(perm.Permission.IpProtocol)
	fromPort := awssdk.Int64Value(perm.Permission.FromPort)
	toPort := awssdk.Int64Value(perm.Permission.ToPort)
	base := fmt.Sprintf("IpProtocol: %v, FromPort: %v, ToPort: %v", protocol, fromPort, toPort)
	if len(perm.Permission.IpRanges) == 1 {
		cidrIP := awssdk.StringValue(perm.Permission.IpRanges[0].CidrIp)
		return fmt.Sprintf("%v, IpRange: %v", base, cidrIP)
	}
	if len(perm.Permission.Ipv6Ranges) == 1 {
		cidrIPv6 := awssdk.StringValue(perm.Permission.Ipv6Ranges[0].CidrIpv6)
		return fmt.Sprintf("%v, Ipv6Range: %v", base, cidrIPv6)
	}
	if len(perm.Permission.PrefixListIds) == 1 {
		prefixListID := awssdk.StringValue(perm.Permission.PrefixListIds[0].PrefixListId)
		return fmt.Sprintf("%v, PrefixListId: %v", base, prefixListID)
	}
	if len(perm.Permission.UserIdGroupPairs) == 1 {
		groupID := awssdk.StringValue(perm.Permission.UserIdGroupPairs[0].GroupId)
		return fmt.Sprintf("%v, UserIdGroupPair: %v", base, groupID)
	}

	// we are facing some unknown permission, let's include all fields.
	payload, _ := json.Marshal(perm.Permission)
	return string(payload)
}

// NewRawSecurityGroupInfo constructs new SecurityGroupInfo with raw ec2SDK's SecurityGroup object.
func NewRawSecurityGroupInfo(sdkSG *ec2sdk.SecurityGroup) SecurityGroupInfo {
	sgID := awssdk.StringValue(sdkSG.GroupId)
	var ingress []IPPermissionInfo
	for _, sdkPermission := range sdkSG.IpPermissions {
		for _, expandedPermission := range expandSDKIPPermission(*sdkPermission) {
			ingress = append(ingress, NewRawIPPermission(expandedPermission))
		}
	}
	tags := buildSecurityGroupTags(sdkSG)
	return SecurityGroupInfo{
		SecurityGroupID: sgID,
		Ingress:         ingress,
		Tags:            tags,
	}
}

// NewCIDRIPPermission constructs new IPPermissionInfo with CIDR configuration.
func NewCIDRIPPermission(ipProtocol string, fromPort *int64, toPort *int64, cidr string, labels map[string]string) IPPermissionInfo {
	description := buildIPPermissionDescriptionForLabels(labels)
	return IPPermissionInfo{
		Permission: ec2sdk.IpPermission{
			IpProtocol: awssdk.String(ipProtocol),
			FromPort:   fromPort,
			ToPort:     toPort,
			IpRanges: []*ec2sdk.IpRange{
				{
					CidrIp:      awssdk.String(cidr),
					Description: awssdk.String(description),
				},
			},
		},
		Labels: labels,
	}
}

// NewCIDRv6IPPermission constructs new IPPermissionInfo with CIDRv6 configuration.
func NewCIDRv6IPPermission(ipProtocol string, fromPort *int64, toPort *int64, cidrV6 string, labels map[string]string) IPPermissionInfo {
	description := buildIPPermissionDescriptionForLabels(labels)
	return IPPermissionInfo{
		Permission: ec2sdk.IpPermission{
			IpProtocol: awssdk.String(ipProtocol),
			FromPort:   fromPort,
			ToPort:     toPort,
			Ipv6Ranges: []*ec2sdk.Ipv6Range{
				{
					CidrIpv6:    awssdk.String(cidrV6),
					Description: awssdk.String(description),
				},
			},
		},
		Labels: labels,
	}
}

// NewCIDRv6IPPermission constructs new IPPermissionInfo with groupID configuration.
func NewGroupIDIPPermission(ipProtocol string, fromPort *int64, toPort *int64, groupID string, labels map[string]string) IPPermissionInfo {
	description := buildIPPermissionDescriptionForLabels(labels)
	return IPPermissionInfo{
		Permission: ec2sdk.IpPermission{
			IpProtocol: awssdk.String(ipProtocol),
			FromPort:   fromPort,
			ToPort:     toPort,
			UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
				{
					GroupId:     awssdk.String(groupID),
					Description: awssdk.String(description),
				},
			},
		},
		Labels: labels,
	}
}

// NewPrefixListIDPermission constructs new IPPermissionInfo with prefixListID configuration
func NewPrefixListIDPermission(ipProtocol string, fromPort *int64, toPort *int64, prefixListID string, labels map[string]string) IPPermissionInfo {
	description := buildIPPermissionDescriptionForLabels(labels)
	return IPPermissionInfo{
		Permission: ec2sdk.IpPermission{
			IpProtocol: awssdk.String(ipProtocol),
			FromPort:   fromPort,
			ToPort:     toPort,
			PrefixListIds: []*ec2sdk.PrefixListId{
				{
					PrefixListId: awssdk.String(prefixListID),
					Description:  awssdk.String(description),
				},
			},
		},
		Labels: labels,
	}
}

// NewRawIPPermission constructs new IPPermissionInfo with raw ec2SDK's IpPermission object.
// Note: this IpPermission should be expanded(i.e. only contains one source configuration)
func NewRawIPPermission(sdkPermission ec2sdk.IpPermission) IPPermissionInfo {
	if len(sdkPermission.IpRanges) == 1 {
		return IPPermissionInfo{
			Permission: sdkPermission,
			Labels:     buildIPPermissionLabelsForDescription(awssdk.StringValue(sdkPermission.IpRanges[0].Description)),
		}
	}
	if len(sdkPermission.Ipv6Ranges) == 1 {
		return IPPermissionInfo{
			Permission: sdkPermission,
			Labels:     buildIPPermissionLabelsForDescription(awssdk.StringValue(sdkPermission.Ipv6Ranges[0].Description)),
		}
	}
	if len(sdkPermission.PrefixListIds) == 1 {
		return IPPermissionInfo{
			Permission: sdkPermission,
			Labels:     buildIPPermissionLabelsForDescription(awssdk.StringValue(sdkPermission.PrefixListIds[0].Description)),
		}
	}
	if len(sdkPermission.UserIdGroupPairs) == 1 {
		return IPPermissionInfo{
			Permission: sdkPermission,
			Labels:     buildIPPermissionLabelsForDescription(awssdk.StringValue(sdkPermission.UserIdGroupPairs[0].Description)),
		}
	}
	return IPPermissionInfo{
		Permission: sdkPermission,
		Labels:     nil,
	}
}

// NewIPPermissionLabelsForRawDescription constructs permission labels from description only.
func NewIPPermissionLabelsForRawDescription(description string) map[string]string {
	return map[string]string{labelKeyRawDescription: description}
}

// buildSecurityGroupTags generates the tags for securityGroup.
func buildSecurityGroupTags(sdkSG *ec2sdk.SecurityGroup) map[string]string {
	sgTags := make(map[string]string, len(sdkSG.Tags))
	for _, tag := range sdkSG.Tags {
		sgTags[awssdk.StringValue(tag.Key)] = awssdk.StringValue(tag.Value)
	}
	return sgTags
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

// buildIPPermissionLabelsForDescription computes labels parsed from IPPermission description
func buildIPPermissionLabelsForDescription(description string) map[string]string {
	labels := map[string]string{labelKeyRawDescription: description}
	for _, groups := range commaSeparatedKVPairPattern.FindAllStringSubmatch(description, -1) {
		labels[groups[1]] = groups[2]
	}
	return labels
}

// buildIPPermissionDescriptionForLabels compute a description from labels.
func buildIPPermissionDescriptionForLabels(labels map[string]string) string {
	if rawDescription, exists := labels[labelKeyRawDescription]; exists {
		return rawDescription
	}

	kvPairs := make([]string, 0, len(labels))
	sortedLabelKeys := sets.StringKeySet(labels).List()
	for _, key := range sortedLabelKeys {
		value := labels[key]
		kvPairs = append(kvPairs, fmt.Sprintf("%v=%v", key, value))
	}
	return strings.Join(kvPairs, ",")
}
