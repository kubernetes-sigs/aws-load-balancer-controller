package ingress

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"regexp"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/equality"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"strings"
)

const (
	resourceIDLoadBalancer = "LoadBalancer"
)

func (b *defaultModelBuilder) buildLoadBalancer(ctx context.Context, stack core.Stack, ingGroup Group) (*elbv2model.LoadBalancer, error) {
	lbSpec, err := b.buildLoadBalancerSpec(ctx, stack, ingGroup)
	if err != nil {
		return nil, err
	}
	return elbv2model.NewLoadBalancer(stack, resourceIDLoadBalancer, lbSpec), nil
}

func (b *defaultModelBuilder) buildLoadBalancerSpec(ctx context.Context, stack core.Stack, ingGroup Group) (elbv2model.LoadBalancerSpec, error) {
	scheme, err := b.buildLoadBalancerScheme(ctx, ingGroup)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	ipAddressType, err := b.buildLoadBalancerIPAddressType(ctx, ingGroup)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	subnetMappings, err := b.buildLoadBalancerSubnetMappings(ctx, ingGroup, scheme)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	securityGroups, err := b.buildLoadBalancerSecurityGroups(ctx, stack, ingGroup)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	loadBalancerAttributes, err := b.buildLoadBalancerAttributes(ctx, ingGroup)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	tags, err := b.buildLoadBalancerTags(ctx, ingGroup)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	name := b.buildLoadBalancerName(ctx, ingGroup, scheme)
	return elbv2model.LoadBalancerSpec{
		Name:                   name,
		Type:                   elbv2model.LoadBalancerTypeApplication,
		Scheme:                 &scheme,
		IPAddressType:          &ipAddressType,
		SubnetMappings:         subnetMappings,
		SecurityGroups:         securityGroups,
		LoadBalancerAttributes: loadBalancerAttributes,
		Tags:                   tags,
	}, nil
}

var loadBalancerNamePattern, _ = regexp.Compile("[[:^alnum:]]")

func (b *defaultModelBuilder) buildLoadBalancerName(ctx context.Context, ingGroup Group, scheme elbv2model.LoadBalancerScheme) string {
	uuidHash := md5.New()
	_, _ = uuidHash.Write([]byte(b.clusterName))
	_, _ = uuidHash.Write([]byte(ingGroup.ID.String()))
	_, _ = uuidHash.Write([]byte(scheme))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	payload := loadBalancerNamePattern.ReplaceAllString(ingGroup.ID.String(), "-")
	return fmt.Sprintf("k8s-%.17s-%.10s", payload, uuid)
}

func (b *defaultModelBuilder) buildLoadBalancerScheme(ctx context.Context, ingGroup Group) (elbv2model.LoadBalancerScheme, error) {
	explicitSchemes := sets.String{}
	for _, ing := range ingGroup.Members {
		rawSchema := ""
		if exists := b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixScheme, &rawSchema, ing.Annotations); !exists {
			continue
		}
		explicitSchemes.Insert(rawSchema)
	}
	if len(explicitSchemes) == 0 {
		return b.defaultScheme, nil
	}
	if len(explicitSchemes) > 1 {
		return "", errors.Errorf("conflicting scheme: %v", explicitSchemes)
	}
	rawScheme, _ := explicitSchemes.PopAny()
	switch rawScheme {
	case string(elbv2model.LoadBalancerSchemeInternetFacing):
		return elbv2model.LoadBalancerSchemeInternetFacing, nil
	case string(elbv2model.LoadBalancerSchemeInternal):
		return elbv2model.LoadBalancerSchemeInternal, nil
	default:
		return "", errors.Errorf("unknown scheme: %v", rawScheme)
	}
}

// buildLoadBalancerIPAddressType builds the LoadBalancer IPAddressType.
func (b *defaultModelBuilder) buildLoadBalancerIPAddressType(ctx context.Context, ingGroup Group) (elbv2model.IPAddressType, error) {
	explicitIPAddressTypes := sets.NewString()
	for _, ing := range ingGroup.Members {
		rawIPAddressType := ""
		if exists := b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixIPAddressType, &rawIPAddressType, ing.Annotations); !exists {
			continue
		}
		explicitIPAddressTypes.Insert(rawIPAddressType)
	}
	if len(explicitIPAddressTypes) == 0 {
		return b.defaultIPAddressType, nil
	}
	if len(explicitIPAddressTypes) > 1 {
		return "", errors.Errorf("conflicting IPAddressType: %v", explicitIPAddressTypes.List())
	}
	rawIPAddressType, _ := explicitIPAddressTypes.PopAny()
	switch rawIPAddressType {
	case string(elbv2model.IPAddressTypeIPV4):
		return elbv2model.IPAddressTypeIPV4, nil
	case string(elbv2model.IPAddressTypeDualStack):
		return elbv2model.IPAddressTypeDualStack, nil
	default:
		return "", errors.Errorf("unknown IPAddressType: %v", rawIPAddressType)
	}
}

func (b *defaultModelBuilder) buildLoadBalancerSubnetMappings(ctx context.Context, ingGroup Group, scheme elbv2model.LoadBalancerScheme) ([]elbv2model.SubnetMapping, error) {
	var explicitSubnetNameOrIDsList [][]string
	for _, ing := range ingGroup.Members {
		var rawSubnetNameOrIDs []string
		if exists := b.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixSubnets, &rawSubnetNameOrIDs, ing.Annotations); !exists {
			continue
		}
		explicitSubnetNameOrIDsList = append(explicitSubnetNameOrIDsList, rawSubnetNameOrIDs)
	}
	if len(explicitSubnetNameOrIDsList) == 0 {
		chosenSubnetIDs, err := b.subnetsResolver.DiscoverSubnets(ctx, scheme)
		if err != nil {
			return nil, err
		}
		if len(chosenSubnetIDs) <= 2 {
			return nil, errors.Errorf("cannot found at least two subnet from different Availability Zones, discovered subnetIDs: %v", chosenSubnetIDs)
		}
		return buildLoadBalancerSubnetMappingsWithSubnetIDs(chosenSubnetIDs), nil
	}

	chosenSubnetNameOrIDs := explicitSubnetNameOrIDsList[0]
	for _, subnetNameOrIDs := range explicitSubnetNameOrIDsList[1:] {
		// subnetNameOrIDs orders doesn't matter.
		if !cmp.Equal(chosenSubnetNameOrIDs, subnetNameOrIDs, equality.IgnoreStringSliceOrder()) {
			return nil, errors.Errorf("conflicting subnets: %v | %v", chosenSubnetNameOrIDs, subnetNameOrIDs)
		}
	}
	chosenSubnetIDs, err := b.resolveSubnetIDsViaNameOrIDSlice(ctx, chosenSubnetNameOrIDs)
	if err != nil {
		return nil, err
	}
	return buildLoadBalancerSubnetMappingsWithSubnetIDs(chosenSubnetIDs), nil
}

func (b *defaultModelBuilder) buildLoadBalancerSecurityGroups(ctx context.Context, stack core.Stack, ingGroup Group) ([]core.StringToken, error) {
	var explicitSGNameOrIDsList [][]string
	for _, ing := range ingGroup.Members {
		var rawSGNameOrIDs []string
		if exists := b.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixSecurityGroups, &rawSGNameOrIDs, ing.Annotations); !exists {
			continue
		}
		explicitSGNameOrIDsList = append(explicitSGNameOrIDsList, rawSGNameOrIDs)
	}
	if len(explicitSGNameOrIDsList) == 0 {
		// TODO, add a managed securityGroup into stack.
		return nil, errors.Errorf("not supported for now")
	}

	chosenSGNameOrIDs := explicitSGNameOrIDsList[0]
	for _, sgNameOrIDs := range explicitSGNameOrIDsList[1:] {
		// securityGroups order might matters in the future(e.g. use the first securityGroup for traffic to nodeGroups)
		if !cmp.Equal(chosenSGNameOrIDs, sgNameOrIDs) {
			return nil, errors.Errorf("conflicting securityGroups: %v | %v", chosenSGNameOrIDs, sgNameOrIDs)
		}
	}
	chosenSGIDs, err := b.resolveSecurityGroupIDsViaNameOrIDSlice(ctx, chosenSGNameOrIDs)
	if err != nil {
		return nil, err
	}
	sgIDTokens := make([]core.StringToken, 0, len(chosenSGIDs))
	for _, sgID := range chosenSGIDs {
		sgIDTokens = append(sgIDTokens, core.LiteralStringToken(sgID))
	}
	return sgIDTokens, nil
}

func (b *defaultModelBuilder) buildLoadBalancerAttributes(ctx context.Context, ingGroup Group) ([]elbv2model.LoadBalancerAttribute, error) {
	mergedAttributes := make(map[string]string)
	for _, ing := range ingGroup.Members {
		var rawAttributes map[string]string
		if _, err := b.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixLoadBalancerAttributes, &rawAttributes, ing.Annotations); err != nil {
			return nil, err
		}
		for attrKey, attrValue := range rawAttributes {
			if existingAttrValue, exists := mergedAttributes[attrKey]; exists && existingAttrValue != attrValue {
				return nil, errors.Errorf("conflicting loadBalancerAttribute %v: %v | %v", attrKey, existingAttrValue, attrValue)
			}
			mergedAttributes[attrKey] = attrValue
		}
	}
	attributes := make([]elbv2model.LoadBalancerAttribute, 0, len(mergedAttributes))
	for attrKey, attrValue := range mergedAttributes {
		attributes = append(attributes, elbv2model.LoadBalancerAttribute{
			Key:   attrKey,
			Value: attrValue,
		})
	}
	return attributes, nil
}

func (b *defaultModelBuilder) buildLoadBalancerTags(ctx context.Context, ingGroup Group) (map[string]string, error) {
	mergedTags := make(map[string]string)
	for _, ing := range ingGroup.Members {
		var rawTags map[string]string
		if _, err := b.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTags, &rawTags, ing.Annotations); err != nil {
			return nil, err
		}
		for tagKey, tagValue := range rawTags {
			if existingTagValue, exists := mergedTags[tagKey]; exists && existingTagValue != tagValue {
				return nil, errors.Errorf("conflicting tag %v: %v | %v", tagKey, existingTagValue, tagValue)
			}
			mergedTags[tagKey] = tagValue
		}
	}
	return mergedTags, nil
}

// resolveSubnetIDsViaNameOrIDSlice resolves the subnetIDs for LoadBalancer via a slice of subnetName or subnetIDs.
func (b *defaultModelBuilder) resolveSubnetIDsViaNameOrIDSlice(ctx context.Context, subnetNameOrIDs []string) ([]string, error) {
	var subnetIDs []string
	var subnetNames []string
	for _, nameOrID := range subnetNameOrIDs {
		if strings.HasPrefix(nameOrID, "subnet-") {
			subnetIDs = append(subnetIDs, nameOrID)
		} else {
			subnetNames = append(subnetNames, nameOrID)
		}
	}
	var resolvedSubnets []*ec2sdk.Subnet
	if len(subnetIDs) > 0 {
		req := &ec2sdk.DescribeSubnetsInput{
			SubnetIds: awssdk.StringSlice(subnetIDs),
		}
		subnets, err := b.ec2Client.DescribeSubnetsAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		resolvedSubnets = append(resolvedSubnets, subnets...)
	}
	if len(subnetNames) > 0 {
		req := &ec2sdk.DescribeSubnetsInput{
			Filters: []*ec2sdk.Filter{
				{
					Name:   awssdk.String("tag:Name"),
					Values: awssdk.StringSlice(subnetNames),
				},
				{
					Name:   awssdk.String("vpc-id"),
					Values: awssdk.StringSlice([]string{b.vpcID}),
				},
			},
		}
		subnets, err := b.ec2Client.DescribeSubnetsAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		resolvedSubnets = append(resolvedSubnets, subnets...)
	}
	resolvedSubnetIDs := make([]string, 0, len(resolvedSubnets))
	for _, subnet := range resolvedSubnets {
		resolvedSubnetIDs = append(resolvedSubnetIDs, awssdk.StringValue(subnet.SubnetId))
	}
	if len(resolvedSubnetIDs) != len(subnetNameOrIDs) {
		return nil, errors.Errorf("couldn't found all subnets, nameOrIDs: %v, found: %v", subnetNameOrIDs, resolvedSubnetIDs)
	}
	return resolvedSubnetIDs, nil
}

func (b *defaultModelBuilder) resolveSecurityGroupIDsViaNameOrIDSlice(ctx context.Context, sgNameOrIDs []string) ([]string, error) {
	var sgIDs []string
	var sgNames []string
	for _, nameOrID := range sgNameOrIDs {
		if strings.HasPrefix(nameOrID, "sg-") {
			sgIDs = append(sgIDs, nameOrID)
		} else {
			sgNames = append(sgNames, nameOrID)
		}
	}
	var resolvedSGs []*ec2sdk.SecurityGroup
	if len(sgIDs) > 0 {
		req := &ec2sdk.DescribeSecurityGroupsInput{
			GroupIds: awssdk.StringSlice(sgIDs),
		}
		sgs, err := b.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		resolvedSGs = append(resolvedSGs, sgs...)
	}
	if len(sgNames) > 0 {
		req := &ec2sdk.DescribeSecurityGroupsInput{
			Filters: []*ec2sdk.Filter{
				{
					Name:   awssdk.String("tag:Name"),
					Values: awssdk.StringSlice(sgNames),
				},
				{
					Name:   awssdk.String("vpc-id"),
					Values: awssdk.StringSlice([]string{b.vpcID}),
				},
			},
		}
		sgs, err := b.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
		if err != nil {
			return nil, err
		}
		resolvedSGs = append(resolvedSGs, sgs...)
	}
	resolvedSGIDs := make([]string, 0, len(resolvedSGs))
	for _, sg := range resolvedSGs {
		resolvedSGIDs = append(resolvedSGIDs, awssdk.StringValue(sg.GroupId))
	}
	if len(resolvedSGIDs) != len(sgNameOrIDs) {
		return nil, errors.Errorf("couldn't found all securityGroups, nameOrIDs: %v, found: %v", sgNameOrIDs, resolvedSGIDs)
	}
	return resolvedSGIDs, nil
}

func buildLoadBalancerSubnetMappingsWithSubnetIDs(subnetIDs []string) []elbv2model.SubnetMapping {
	subnetMappings := make([]elbv2model.SubnetMapping, 0, len(subnetIDs))
	for _, subnetID := range subnetIDs {
		subnetMappings = append(subnetMappings, elbv2model.SubnetMapping{
			SubnetID: subnetID,
		})
	}
	return subnetMappings
}
