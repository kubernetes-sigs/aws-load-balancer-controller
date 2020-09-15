package ec2

import (
	"github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/equality"
)

// CompareOptionForIPRange returns the compare option for ec2 IPRange
func CompareOptionForIPRange() cmp.Option {
	return equality.IgnoreOtherFields(ec2sdk.IpRange{}, "CidrIp")
}

// CompareOptionForIPRanges returns the compare option for ec2 IPRange slice
func CompareOptionForIPRanges() cmp.Options {
	return cmp.Options{
		cmpopts.SortSlices(func(lhs *ec2sdk.IpRange, rhs *ec2sdk.IpRange) bool {
			return aws.StringValue(lhs.CidrIp) < aws.StringValue(rhs.CidrIp)
		}),
		CompareOptionForIPRange(),
	}
}

// CompareOptionForIPv6Range returns the compare option for ec2 IPv6Range
func CompareOptionForIPv6Range() cmp.Option {
	return equality.IgnoreOtherFields(ec2sdk.Ipv6Range{}, "CidrIpv6")
}

// CompareOptionForIPV6Ranges returns the compare option for ec2 IPv6Range slice
func CompareOptionForIPv6Ranges() cmp.Options {
	return cmp.Options{
		cmpopts.SortSlices(func(lhs *ec2sdk.Ipv6Range, rhs *ec2sdk.Ipv6Range) bool {
			return aws.StringValue(lhs.CidrIpv6) < aws.StringValue(rhs.CidrIpv6)
		}),
		CompareOptionForIPv6Range(),
	}
}

// CompareOptionForIPV6Ranges returns the compare option for ec2 UserIDGroupPair
func CompareOptionForUserIDGroupPair() cmp.Option {
	return equality.IgnoreOtherFields(ec2sdk.UserIdGroupPair{}, "GroupId")
}

// CompareOptionForIPV6Ranges returns the compare option for ec2 UserIDGroupPair slice
func CompareOptionForUserIDGroupPairs() cmp.Option {
	return cmp.Options{
		cmpopts.SortSlices(func(lhs *ec2sdk.UserIdGroupPair, rhs *ec2sdk.UserIdGroupPair) bool {
			return aws.StringValue(lhs.GroupId) < aws.StringValue(rhs.GroupId)
		}),
		CompareOptionForUserIDGroupPair(),
	}
}

// CompareOptionForIPV6Ranges returns the compare option for ec2 prefixListId
func CompareOptionForPrefixListId() cmp.Option {
	return equality.IgnoreOtherFields(ec2sdk.PrefixListId{}, "PrefixListId")
}

// CompareOptionForIPV6Ranges returns the compare option for ec2 prefixListId slice
func CompareOptionForPrefixListIds() cmp.Option {
	return cmp.Options{
		cmpopts.SortSlices(func(lhs *ec2sdk.PrefixListId, rhs *ec2sdk.PrefixListId) bool {
			return aws.StringValue(lhs.PrefixListId) < aws.StringValue(rhs.PrefixListId)
		}),
		CompareOptionForPrefixListId(),
	}
}

// CompareOptionForIPPermission returns the compare option for ec2 IPPermission object.
func CompareOptionForIPPermission() cmp.Option {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		CompareOptionForIPRanges(),
		CompareOptionForIPv6Ranges(),
		CompareOptionForUserIDGroupPairs(),
		CompareOptionForPrefixListIds(),
	}
}
