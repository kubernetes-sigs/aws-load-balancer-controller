package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

const (
	resourceIDLoadBalancer         = "LoadBalancer"
	minimalAvailableIPAddressCount = int64(8)
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
	name, err := t.buildLoadBalancerName(ctx, scheme)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
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

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context, scheme elbv2model.LoadBalancerScheme) (string, error) {
	explicitNames := sets.String{}
	for _, member := range t.ingGroup.Members {
		rawName := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixLoadBalancerName, &rawName, member.Ing.Annotations); !exists {
			continue
		}
		explicitNames.Insert(rawName)
	}
	if len(explicitNames) == 1 {
		name, _ := explicitNames.PopAny()
		// The name of the loadbalancer can only have up to 32 characters
		if len(name) > 32 {
			return "", errors.New("load balancer name cannot be longer than 32 characters")
		}
		return name, nil
	}
	if len(explicitNames) > 1 {
		return "", errors.Errorf("conflicting load balancer name: %v", explicitNames)
	}
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.ingGroup.ID.String()))
	_, _ = uuidHash.Write([]byte(scheme))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	if t.ingGroup.ID.IsExplicit() {
		payload := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Name, "")
		return fmt.Sprintf("k8s-%.17s-%.10s", payload, uuid), nil
	}

	sanitizedNamespace := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Namespace, "")
	sanitizedName := invalidLoadBalancerNamePattern.ReplaceAllString(t.ingGroup.ID.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid), nil
}

func (t *defaultModelBuildTask) buildLoadBalancerScheme(_ context.Context) (elbv2model.LoadBalancerScheme, error) {
	explicitSchemes := sets.String{}
	for _, member := range t.ingGroup.Members {
		if member.IngClassConfig.IngClassParams != nil && member.IngClassConfig.IngClassParams.Spec.Scheme != nil {
			scheme := string(*member.IngClassConfig.IngClassParams.Spec.Scheme)
			explicitSchemes.Insert(scheme)
			continue
		}
		rawSchema := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixScheme, &rawSchema, member.Ing.Annotations); !exists {
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
	for _, member := range t.ingGroup.Members {
		if member.IngClassConfig.IngClassParams != nil && member.IngClassConfig.IngClassParams.Spec.IPAddressType != nil {
			ipAddressType := string(*member.IngClassConfig.IngClassParams.Spec.IPAddressType)
			explicitIPAddressTypes.Insert(ipAddressType)
			continue
		}
		rawIPAddressType := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixIPAddressType, &rawIPAddressType, member.Ing.Annotations); !exists {
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
	for _, member := range t.ingGroup.Members {
		var rawSubnetNameOrIDs []string
		if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixSubnets, &rawSubnetNameOrIDs, member.Ing.Annotations); !exists {
			continue
		}
		explicitSubnetNameOrIDsList = append(explicitSubnetNameOrIDsList, rawSubnetNameOrIDs)
	}

	if len(explicitSubnetNameOrIDsList) != 0 {
		chosenSubnetNameOrIDs := explicitSubnetNameOrIDsList[0]
		for _, subnetNameOrIDs := range explicitSubnetNameOrIDsList[1:] {
			// subnetNameOrIDs order doesn't matter
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
	stackTags := t.trackingProvider.StackTags(t.stack)

	sdkLBs, err := t.elbv2TaggingManager.ListLoadBalancers(ctx, tracking.TagsAsTagFilter(stackTags))
	if err != nil {
		return nil, err
	}

	if len(sdkLBs) == 0 || (string(scheme) != awssdk.StringValue(sdkLBs[0].LoadBalancer.Scheme)) {
		chosenSubnets, err := t.subnetsResolver.ResolveViaDiscovery(ctx,
			networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeApplication),
			networking.WithSubnetsResolveLBScheme(scheme),
			networking.WithSubnetsResolveAvailableIPAddressCount(minimalAvailableIPAddressCount),
			networking.WithSubnetsClusterTagCheck(t.featureGates.Enabled(config.SubnetsClusterTagCheck)),
		)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't auto-discover subnets")
		}
		return buildLoadBalancerSubnetMappingsWithSubnets(chosenSubnets), nil
	}

	availabilityZones := sdkLBs[0].LoadBalancer.AvailabilityZones
	subnetIDs := make([]string, 0, len(availabilityZones))
	for _, availabilityZone := range availabilityZones {
		subnetID := awssdk.StringValue(availabilityZone.SubnetId)
		subnetIDs = append(subnetIDs, subnetID)
	}
	return buildLoadBalancerSubnetMappingsWithSubnetIDs(subnetIDs), nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSecurityGroups(ctx context.Context, listenPortConfigByPort map[int64]listenPortConfig, ipAddressType elbv2model.IPAddressType) ([]core.StringToken, error) {
	sgNameOrIDsViaAnnotation, err := t.buildFrontendSGNameOrIDsFromAnnotation(ctx)
	if err != nil {
		return nil, err
	}
	var lbSGTokens []core.StringToken
	if len(sgNameOrIDsViaAnnotation) == 0 {
		managedSG, err := t.buildManagedSecurityGroup(ctx, listenPortConfigByPort, ipAddressType)
		if err != nil {
			return nil, err
		}
		lbSGTokens = append(lbSGTokens, managedSG.GroupID())
		if !t.enableBackendSG {
			t.backendSGIDToken = managedSG.GroupID()
		} else {
			backendSGID, err := t.backendSGProvider.Get(ctx)
			if err != nil {
				return nil, err
			}
			t.backendSGIDToken = core.LiteralStringToken((backendSGID))
			lbSGTokens = append(lbSGTokens, t.backendSGIDToken)
		}
		t.logger.Info("Auto Create SG", "LB SGs", lbSGTokens, "backend SG", t.backendSGIDToken)
	} else {
		manageBackendSGRules, err := t.buildManageSecurityGroupRulesFlag(ctx)
		if err != nil {
			return nil, err
		}
		frontendSGIDs, err := t.resolveSecurityGroupIDsViaNameOrIDSlice(ctx, sgNameOrIDsViaAnnotation)
		if err != nil {
			return nil, err
		}
		for _, sgID := range frontendSGIDs {
			lbSGTokens = append(lbSGTokens, core.LiteralStringToken(sgID))
		}

		if manageBackendSGRules {
			if !t.enableBackendSG {
				return nil, errors.New("backendSG feature is required to manage worker node SG rules when frontendSG manually specified")
			}
			backendSGID, err := t.backendSGProvider.Get(ctx)
			if err != nil {
				return nil, err
			}
			t.backendSGIDToken = core.LiteralStringToken(backendSGID)
			lbSGTokens = append(lbSGTokens, t.backendSGIDToken)
		}
		t.logger.Info("SG configured via annotation", "LB SGs", lbSGTokens, "backend SG", t.backendSGIDToken)
	}
	return lbSGTokens, nil
}

func (t *defaultModelBuildTask) buildFrontendSGNameOrIDsFromAnnotation(ctx context.Context) ([]string, error) {
	var explicitSGNameOrIDsList [][]string
	for _, member := range t.ingGroup.Members {
		var rawSGNameOrIDs []string
		if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.IngressSuffixSecurityGroups, &rawSGNameOrIDs, member.Ing.Annotations); !exists {
			continue
		}
		explicitSGNameOrIDsList = append(explicitSGNameOrIDsList, rawSGNameOrIDs)
	}
	if len(explicitSGNameOrIDsList) == 0 {
		return nil, nil
	}
	chosenSGNameOrIDs := explicitSGNameOrIDsList[0]
	for _, sgNameOrIDs := range explicitSGNameOrIDsList[1:] {
		if !cmp.Equal(chosenSGNameOrIDs, sgNameOrIDs) {
			return nil, errors.Errorf("conflicting securityGroups: %v | %v", chosenSGNameOrIDs, sgNameOrIDs)
		}
	}
	return chosenSGNameOrIDs, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerCOIPv4Pool(_ context.Context) (*string, error) {
	explicitCOIPv4Pools := sets.NewString()
	for _, member := range t.ingGroup.Members {
		rawCOIPv4Pool := ""
		if exists := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixCustomerOwnedIPv4Pool, &rawCOIPv4Pool, member.Ing.Annotations); !exists {
			continue
		}
		if len(rawCOIPv4Pool) == 0 {
			return nil, errors.Errorf("cannot use empty value for %s annotation, ingress: %v",
				annotations.IngressSuffixCustomerOwnedIPv4Pool, k8s.NamespacedName(member.Ing))
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
	ingGroupAttributes, err := t.buildIngressGroupLoadBalancerAttributes(t.ingGroup.Members)
	if err != nil {
		return nil, err
	}
	attributes := make([]elbv2model.LoadBalancerAttribute, 0, len(ingGroupAttributes))
	for attrKey, attrValue := range ingGroupAttributes {
		attributes = append(attributes, elbv2model.LoadBalancerAttribute{
			Key:   attrKey,
			Value: attrValue,
		})
	}
	return attributes, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerTags(_ context.Context) (map[string]string, error) {
	ingGroupTags, err := t.buildIngressGroupResourceTags(t.ingGroup.Members)
	if err != nil {
		return nil, err
	}
	return algorithm.MergeStringMap(t.defaultTags, ingGroupTags), nil
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
		return nil, errors.Errorf("couldn't find all securityGroups, nameOrIDs: %v, found: %v", sgNameOrIDs, resolvedSGIDs)
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

func buildLoadBalancerSubnetMappingsWithSubnetIDs(subnetIDs []string) []elbv2model.SubnetMapping {
	subnetMappings := make([]elbv2model.SubnetMapping, 0, len(subnetIDs))
	for _, subnetID := range subnetIDs {
		subnetMappings = append(subnetMappings, elbv2model.SubnetMapping{
			SubnetID: subnetID,
		})
	}
	return subnetMappings
}
