package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"regexp"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"strings"
)

const (
	resourceIDLoadBalancer = "LoadBalancer"
)

func (t *defaultModelBuildTask) buildLoadBalancer(ctx context.Context, listenPortConfigByPort map[int64]listenPortConfig) (*elbv2model.LoadBalancer, error) {
	lbSpec, err := t.buildLoadBalancerSpec(ctx, listenPortConfigByPort)
	if err != nil {
		return nil, err
	}
	lb := elbv2model.NewLoadBalancer(t.stack, resourceIDLoadBalancer, lbSpec)
	t.loadBalancer = lb
	return lb, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSpec(ctx context.Context, listenPortConfigByPort map[int64]listenPortConfig) (elbv2model.LoadBalancerSpec, error) {
	scheme, err := t.buildLoadBalancerScheme(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	ipAddressType, err := t.buildLoadBalancerIPAddressType(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	subnetMappings, err := t.buildLoadBalancerSubnetMappings(ctx, scheme)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	securityGroups, err := t.buildLoadBalancerSecurityGroups(ctx, listenPortConfigByPort, ipAddressType)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	coIPv4Pool, err := t.buildLoadBalancerCOIPv4Pool(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	loadBalancerAttributes, err := t.buildLoadBalancerAttributes(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	tags, err := t.buildLoadBalancerTags(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	name := t.buildLoadBalancerName(ctx, scheme)
	return elbv2model.LoadBalancerSpec{
		Name:                   name,
		Type:                   elbv2model.LoadBalancerTypeApplication,
		Scheme:                 &scheme,
		IPAddressType:          &ipAddressType,
		SubnetMappings:         subnetMappings,
		SecurityGroups:         securityGroups,
		CustomerOwnedIPv4Pool:  coIPv4Pool,
		LoadBalancerAttributes: loadBalancerAttributes,
		Tags:                   tags,
	}, nil
}

var invalidLoadBalancerNamePattern = regexp.MustCompile("[[:^alnum:]]")

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context, scheme elbv2model.LoadBalancerScheme) string {
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.ingGroup.ID.String()))
	_, _ = uuidHash.Write([]byte(scheme))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	if t.ingGroup.ID.IsExplicit() {
		payload := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Name, "")
		return fmt.Sprintf("k8s-%.17s-%.10s", payload, uuid)
	}

	sanitizedNamespace := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Namespace, "")
	sanitizedName := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (t *defaultModelBuildTask) buildLoadBalancerScheme(_ context.Context) (elbv2model.LoadBalancerScheme, error) {
	explicitSchemes := sets.String{}
	for _, ing := range t.ingGroup.Members {
		rawSchema := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixScheme, &rawSchema, ing.Annotations); !exists {
			continue
		}
		explicitSchemes.Insert(rawSchema)
	}
	if len(explicitSchemes) == 0 {
		return t.defaultScheme, nil
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
func (t *defaultModelBuildTask) buildLoadBalancerIPAddressType(_ context.Context) (elbv2model.IPAddressType, error) {
	explicitIPAddressTypes := sets.NewString()
	for _, ing := range t.ingGroup.Members {
		rawIPAddressType := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixIPAddressType, &rawIPAddressType, ing.Annotations); !exists {
			continue
		}
		explicitIPAddressTypes.Insert(rawIPAddressType)
	}
	if len(explicitIPAddressTypes) == 0 {
		return t.defaultIPAddressType, nil
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

func (t *defaultModelBuildTask) buildLoadBalancerSubnetMappings(ctx context.Context, scheme elbv2model.LoadBalancerScheme) ([]elbv2model.SubnetMapping, error) {
	var explicitSubnetNameOrIDsList [][]string
	for _, ing := range t.ingGroup.Members {
		var rawSubnetNameOrIDs []string
		if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixSubnets, &rawSubnetNameOrIDs, ing.Annotations); !exists {
			continue
		}
		explicitSubnetNameOrIDsList = append(explicitSubnetNameOrIDsList, rawSubnetNameOrIDs)
	}

	if len(explicitSubnetNameOrIDsList) == 0 {
		chosenSubnets, err := t.subnetsResolver.ResolveViaDiscovery(ctx,
			networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
			networking.WithSubnetsResolveLBScheme(scheme),
		)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't auto-discover subnets")
		}
		return buildLoadBalancerSubnetMappingsWithSubnets(chosenSubnets), nil
	}

	chosenSubnetNameOrIDs := explicitSubnetNameOrIDsList[0]
	for _, subnetNameOrIDs := range explicitSubnetNameOrIDsList[1:] {
		// subnetNameOrIDs orders doesn't matter.
		if !cmp.Equal(chosenSubnetNameOrIDs, subnetNameOrIDs, equality.IgnoreStringSliceOrder()) {
			return nil, errors.Errorf("conflicting subnets: %v | %v", chosenSubnetNameOrIDs, subnetNameOrIDs)
		}
	}
	chosenSubnets, err := t.subnetsResolver.ResolveViaNameOrIDSlice(ctx, chosenSubnetNameOrIDs,
		networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
		networking.WithSubnetsResolveLBScheme(scheme),
	)
	if err != nil {
		return nil, err
	}
	return buildLoadBalancerSubnetMappingsWithSubnets(chosenSubnets), nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSecurityGroups(ctx context.Context, listenPortConfigByPort map[int64]listenPortConfig, ipAddressType elbv2model.IPAddressType) ([]core.StringToken, error) {
	var explicitSGNameOrIDsList [][]string
	for _, ing := range t.ingGroup.Members {
		var rawSGNameOrIDs []string
		if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixSecurityGroups, &rawSGNameOrIDs, ing.Annotations); !exists {
			continue
		}
		explicitSGNameOrIDsList = append(explicitSGNameOrIDsList, rawSGNameOrIDs)
	}
	if len(explicitSGNameOrIDsList) == 0 {
		sg, err := t.buildManagedSecurityGroup(ctx, listenPortConfigByPort, ipAddressType)
		if err != nil {
			return nil, err
		}
		return []core.StringToken{sg.GroupID()}, nil
	}

	chosenSGNameOrIDs := explicitSGNameOrIDsList[0]
	for _, sgNameOrIDs := range explicitSGNameOrIDsList[1:] {
		// securityGroups order might matters in the future(e.g. use the first securityGroup for traffic to nodeGroups)
		if !cmp.Equal(chosenSGNameOrIDs, sgNameOrIDs) {
			return nil, errors.Errorf("conflicting securityGroups: %v | %v", chosenSGNameOrIDs, sgNameOrIDs)
		}
	}
	chosenSGIDs, err := t.resolveSecurityGroupIDsViaNameOrIDSlice(ctx, chosenSGNameOrIDs)
	if err != nil {
		return nil, err
	}
	sgIDTokens := make([]core.StringToken, 0, len(chosenSGIDs))
	for _, sgID := range chosenSGIDs {
		sgIDTokens = append(sgIDTokens, core.LiteralStringToken(sgID))
	}
	return sgIDTokens, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerCOIPv4Pool(_ context.Context) (*string, error) {
	explicitCOIPv4Pools := sets.NewString()
	for _, ing := range t.ingGroup.Members {
		rawCOIPv4Pool := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixCustomerOwnedIPv4Pool, &rawCOIPv4Pool, ing.Annotations); !exists {
			continue
		}
		if len(rawCOIPv4Pool) == 0 {
			return nil, errors.Errorf("cannot use empty value for %s annotation, ingress: %v",
				annotations.IngressSuffixCustomerOwnedIPv4Pool, k8s.NamespacedName(ing))
		}
		explicitCOIPv4Pools.Insert(rawCOIPv4Pool)
	}

	if len(explicitCOIPv4Pools) == 0 {
		return nil, nil
	}
	if len(explicitCOIPv4Pools) > 1 {
		return nil, errors.Errorf("conflicting CustomerOwnedIPv4Pool: %v", explicitCOIPv4Pools.List())
	}

	rawCOIPv4Pool, _ := explicitCOIPv4Pools.PopAny()
	return &rawCOIPv4Pool, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerAttributes(_ context.Context) ([]elbv2model.LoadBalancerAttribute, error) {
	mergedAttributes := make(map[string]string)
	for _, ing := range t.ingGroup.Members {
		var rawAttributes map[string]string
		if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixLoadBalancerAttributes, &rawAttributes, ing.Annotations); err != nil {
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

func (t *defaultModelBuildTask) buildLoadBalancerTags(_ context.Context) (map[string]string, error) {
	annotationTags := make(map[string]string)
	for _, ing := range t.ingGroup.Members {
		var rawTags map[string]string
		if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTags, &rawTags, ing.Annotations); err != nil {
			return nil, err
		}
		for tagKey, tagValue := range rawTags {
			if existingTagValue, exists := annotationTags[tagKey]; exists && existingTagValue != tagValue {
				return nil, errors.Errorf("conflicting tag %v: %v | %v", tagKey, existingTagValue, tagValue)
			}
			annotationTags[tagKey] = tagValue
		}
	}
	mergedTags := make(map[string]string)
	for k, v := range t.defaultTags {
		mergedTags[k] = v
	}
	for k, v := range annotationTags {
		mergedTags[k] = v
	}
	return mergedTags, nil
}

func (t *defaultModelBuildTask) resolveSecurityGroupIDsViaNameOrIDSlice(ctx context.Context, sgNameOrIDs []string) ([]string, error) {
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
		sgs, err := t.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
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
					Values: awssdk.StringSlice([]string{t.vpcID}),
				},
			},
		}
		sgs, err := t.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
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

func buildLoadBalancerSubnetMappingsWithSubnets(subnets []*ec2sdk.Subnet) []elbv2model.SubnetMapping {
	subnetMappings := make([]elbv2model.SubnetMapping, 0, len(subnets))
	for _, subnet := range subnets {
		subnetMappings = append(subnetMappings, elbv2model.SubnetMapping{
			SubnetID: awssdk.StringValue(subnet.SubnetId),
		})
	}
	return subnetMappings
}
