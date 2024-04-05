package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

const (
	lbAttrsAccessLogsS3Enabled                 = "access_logs.s3.enabled"
	lbAttrsAccessLogsS3Bucket                  = "access_logs.s3.bucket"
	lbAttrsAccessLogsS3Prefix                  = "access_logs.s3.prefix"
	lbAttrsLoadBalancingCrossZoneEnabled       = "load_balancing.cross_zone.enabled"
	lbAttrsLoadBalancingDnsClientRoutingPolicy = "dns_record.client_routing_policy"
	availabilityZoneAffinity                   = "availability_zone_affinity"
	partialAvailabilityZoneAffinity            = "partial_availability_zone_affinity"
	anyAvailabilityZone                        = "any_availability_zone"
	resourceIDLoadBalancer                     = "LoadBalancer"
	minimalAvailableIPAddressCount             = int64(8)
)

func (t *defaultModelBuildTask) buildLoadBalancer(ctx context.Context, scheme elbv2model.LoadBalancerScheme) error {
	existingLB, err := t.fetchExistingLoadBalancer(ctx)
	if err != nil {
		return err
	}
	spec, err := t.buildLoadBalancerSpec(ctx, scheme, existingLB)
	if err != nil {
		return err
	}
	t.loadBalancer = elbv2model.NewLoadBalancer(t.stack, resourceIDLoadBalancer, spec)
	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSpec(ctx context.Context, scheme elbv2model.LoadBalancerScheme,
	existingLB *elbv2deploy.LoadBalancerWithTags) (elbv2model.LoadBalancerSpec, error) {
	ipAddressType, err := t.buildLoadBalancerIPAddressType(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	lbAttributes, err := t.buildLoadBalancerAttributes(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	securityGroups, err := t.buildLoadBalancerSecurityGroups(ctx, existingLB, ipAddressType)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	tags, err := t.buildLoadBalancerTags(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	subnetMappings, err := t.buildLoadBalancerSubnetMappings(ctx, ipAddressType, scheme, t.ec2Subnets)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	name, err := t.buildLoadBalancerName(ctx, scheme)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	securityGroupsInboundRulesOnPrivateLink, err := t.buildSecurityGroupsInboundRulesOnPrivateLink(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}

	spec := elbv2model.LoadBalancerSpec{
		Name:                   name,
		Type:                   elbv2model.LoadBalancerTypeNetwork,
		Scheme:                 &scheme,
		IPAddressType:          &ipAddressType,
		SecurityGroups:         securityGroups,
		SubnetMappings:         subnetMappings,
		LoadBalancerAttributes: lbAttributes,
		Tags:                   tags,
	}

	if securityGroupsInboundRulesOnPrivateLink != nil {
		spec.SecurityGroupsInboundRulesOnPrivateLink = securityGroupsInboundRulesOnPrivateLink
	}

	return spec, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSecurityGroups(ctx context.Context, existingLB *elbv2deploy.LoadBalancerWithTags,
	ipAddressType elbv2model.IPAddressType) ([]core.StringToken, error) {
	if existingLB != nil && len(existingLB.LoadBalancer.SecurityGroups) == 0 {
		return nil, nil
	}
	if !t.featureGates.Enabled(config.NLBSecurityGroup) {
		if existingLB != nil && len(existingLB.LoadBalancer.SecurityGroups) != 0 {
			return nil, errors.New("conflicting security groups configuration")
		}
		return nil, nil
	}
	var sgNameOrIDs []string
	var lbSGTokens []core.StringToken
	t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixLoadBalancerSecurityGroups, &sgNameOrIDs, t.service.Annotations)
	if len(sgNameOrIDs) == 0 {
		managedSG, err := t.buildManagedSecurityGroup(ctx, ipAddressType)
		if err != nil {
			return nil, err
		}
		lbSGTokens = append(lbSGTokens, managedSG.GroupID())
		if !t.enableBackendSG {
			t.backendSGIDToken = managedSG.GroupID()
		} else {
			backendSGID, err := t.backendSGProvider.Get(ctx, networking.ResourceTypeService, []types.NamespacedName{k8s.NamespacedName(t.service)})
			if err != nil {
				return nil, err
			}
			t.backendSGIDToken = core.LiteralStringToken(backendSGID)
			t.backendSGAllocated = true
			lbSGTokens = append(lbSGTokens, t.backendSGIDToken)
		}
	} else {
		manageBackendSGRules, err := t.buildManageSecurityGroupRulesFlag(ctx)
		if err != nil {
			return nil, err
		}
		frontendSGIDs, err := t.sgResolver.ResolveViaNameOrID(ctx, sgNameOrIDs)
		if err != nil {
			return nil, err
		}
		for _, sgID := range frontendSGIDs {
			lbSGTokens = append(lbSGTokens, core.LiteralStringToken(sgID))
		}
		if manageBackendSGRules {
			if !t.enableBackendSG {
				return nil, errors.New("backendSG feature is required to manage worker node SG rules when frontendSG is manually specified")
			}
			backendSGID, err := t.backendSGProvider.Get(ctx, networking.ResourceTypeService, []types.NamespacedName{k8s.NamespacedName(t.service)})
			if err != nil {
				return nil, err
			}
			t.backendSGIDToken = core.LiteralStringToken(backendSGID)
			t.backendSGAllocated = true
			lbSGTokens = append(lbSGTokens, t.backendSGIDToken)
		}
	}
	return lbSGTokens, nil
}

func (t *defaultModelBuildTask) buildManageSecurityGroupRulesFlag(ctx context.Context) (bool, error) {
	var rawEnabled bool
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixManageSGRules, &rawEnabled, t.service.Annotations)
	if err != nil {
		return false, err
	}
	if exists {
		return rawEnabled, nil
	}
	return false, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerIPAddressType(_ context.Context) (elbv2model.IPAddressType, error) {
	rawIPAddressType := ""
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixIPAddressType, &rawIPAddressType, t.service.Annotations); !exists {
		return t.defaultIPAddressType, nil
	}

	switch rawIPAddressType {
	case string(elbv2model.IPAddressTypeIPV4):
		return elbv2model.IPAddressTypeIPV4, nil
	case string(elbv2model.IPAddressTypeDualStack):
		return elbv2model.IPAddressTypeDualStack, nil
	default:
		return "", errors.Errorf("unknown IPAddressType: %v", rawIPAddressType)
	}
}

func (t *defaultModelBuildTask) buildSecurityGroupsInboundRulesOnPrivateLink(_ context.Context) (*elbv2model.SecurityGroupsInboundRulesOnPrivateLinkStatus, error) {
	var rawSecurityGroupsInboundRulesOnPrivateLink string
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixEnforceSGInboundRulesOnPrivateLinkTraffic, &rawSecurityGroupsInboundRulesOnPrivateLink, t.service.Annotations); !exists {
		return nil, nil
	}
	securityGroupsInboundRulesOnPrivateLink := elbv2model.SecurityGroupsInboundRulesOnPrivateLinkStatus(rawSecurityGroupsInboundRulesOnPrivateLink)

	switch securityGroupsInboundRulesOnPrivateLink {
	case elbv2model.SecurityGroupsInboundRulesOnPrivateLinkOn:
		return &securityGroupsInboundRulesOnPrivateLink, nil
	case elbv2model.SecurityGroupsInboundRulesOnPrivateLinkOff:
		return &securityGroupsInboundRulesOnPrivateLink, nil
	default:
		return nil, errors.Errorf("Invalid value for securityGroupsInboundRulesOnPrivateLink status: %v, value must be one of [%v, %v]", securityGroupsInboundRulesOnPrivateLink, string(elbv2model.SecurityGroupsInboundRulesOnPrivateLinkOn), string(elbv2model.SecurityGroupsInboundRulesOnPrivateLinkOff))
	}
}

func (t *defaultModelBuildTask) buildLoadBalancerScheme(ctx context.Context) (elbv2model.LoadBalancerScheme, error) {
	scheme, explicitSchemeSpecified, err := t.buildLoadBalancerSchemeViaAnnotation(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSchemeInternal, err
	}
	if explicitSchemeSpecified {
		return scheme, nil
	}
	existingLB, err := t.fetchExistingLoadBalancer(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSchemeInternal, err
	}
	if existingLB != nil {
		switch awssdk.StringValue(existingLB.LoadBalancer.Scheme) {
		case string(elbv2model.LoadBalancerSchemeInternal):
			return elbv2model.LoadBalancerSchemeInternal, nil
		case string(elbv2model.LoadBalancerSchemeInternetFacing):
			return elbv2model.LoadBalancerSchemeInternetFacing, nil
		default:
			return "", errors.New("invalid load balancer scheme")
		}
	}
	return elbv2model.LoadBalancerSchemeInternal, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSchemeViaAnnotation(ctx context.Context) (elbv2model.LoadBalancerScheme, bool, error) {
	rawScheme := ""
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixScheme, &rawScheme, t.service.Annotations); exists {
		switch rawScheme {
		case string(elbv2model.LoadBalancerSchemeInternetFacing):
			return elbv2model.LoadBalancerSchemeInternetFacing, true, nil
		case string(elbv2model.LoadBalancerSchemeInternal):
			return elbv2model.LoadBalancerSchemeInternal, true, nil
		default:
			return "", false, errors.Errorf("unknown scheme: %v", rawScheme)
		}
	}
	return t.buildLoadBalancerSchemeLegacyAnnotation(ctx)
}

func (t *defaultModelBuildTask) buildLoadBalancerSchemeLegacyAnnotation(_ context.Context) (elbv2model.LoadBalancerScheme, bool, error) {
	internal := true
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixInternal, &internal, t.service.Annotations)
	if err != nil {
		return "", false, err
	}
	if exists {
		switch internal {
		case false:
			return elbv2model.LoadBalancerSchemeInternetFacing, true, nil
		case true:
			return elbv2model.LoadBalancerSchemeInternal, true, nil
		}
	}
	return elbv2model.LoadBalancerSchemeInternal, false, nil
}

func (t *defaultModelBuildTask) fetchExistingLoadBalancer(ctx context.Context) (*elbv2deploy.LoadBalancerWithTags, error) {
	var fetchError error
	t.fetchExistingLoadBalancerOnce.Do(func() {
		stackTags := t.trackingProvider.StackTags(t.stack)
		sdkLBs, err := t.elbv2TaggingManager.ListLoadBalancers(ctx, tracking.TagsAsTagFilter(stackTags))
		if err != nil {
			fetchError = err
		}
		if len(sdkLBs) == 0 {
			t.existingLoadBalancer = nil
		} else {
			t.existingLoadBalancer = &sdkLBs[0]
		}
	})
	return t.existingLoadBalancer, fetchError
}

func (t *defaultModelBuildTask) buildAdditionalResourceTags(_ context.Context) (map[string]string, error) {
	var annotationTags map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixAdditionalTags, &annotationTags, t.service.Annotations); err != nil {
		return nil, err
	}
	for tagKey := range annotationTags {
		if t.externalManagedTags.Has(tagKey) {
			return nil, errors.Errorf("external managed tag key %v cannot be specified on Service", tagKey)
		}
	}

	mergedTags := algorithm.MergeStringMap(t.defaultTags, annotationTags)
	return mergedTags, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerTags(ctx context.Context) (map[string]string, error) {
	return t.buildAdditionalResourceTags(ctx)
}

func (t *defaultModelBuildTask) buildLoadBalancerSubnetMappings(_ context.Context, ipAddressType elbv2model.IPAddressType, scheme elbv2model.LoadBalancerScheme, ec2Subnets []*ec2sdk.Subnet) ([]elbv2model.SubnetMapping, error) {
	var eipAllocation []string
	eipConfigured := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixEIPAllocations, &eipAllocation, t.service.Annotations)
	if eipConfigured {
		if scheme != elbv2model.LoadBalancerSchemeInternetFacing {
			return nil, errors.Errorf("EIP allocations can only be set for internet facing load balancers")
		}
		if len(eipAllocation) != len(ec2Subnets) {
			return nil, errors.Errorf("count of EIP allocations (%d) and subnets (%d) must match", len(eipAllocation), len(ec2Subnets))
		}
	}

	var ipv4Addresses []netip.Addr
	var rawIPv4Addresses []string
	ipv4AddrConfigured := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixPrivateIpv4Addresses, &rawIPv4Addresses, t.service.Annotations)
	if ipv4AddrConfigured {
		if scheme != elbv2model.LoadBalancerSchemeInternal {
			return nil, errors.Errorf("private IPv4 addresses can only be set for internal load balancers")
		}
		// TODO: consider relax this requirement as ELBv2 API don't require every subnet to have IPv4 address specified.
		if len(rawIPv4Addresses) != len(ec2Subnets) {
			return nil, errors.Errorf("count of private IPv4 addresses (%d) and subnets (%d) must match", len(rawIPv4Addresses), len(ec2Subnets))
		}
		for _, rawIPv4Address := range rawIPv4Addresses {
			ipv4Address, err := netip.ParseAddr(rawIPv4Address)
			if err != nil {
				return nil, errors.Errorf("private IPv4 addresses must be valid IP address: %v", rawIPv4Address)
			}
			if !ipv4Address.Is4() {
				return nil, errors.Errorf("private IPv4 addresses must be valid IPv4 address: %v", rawIPv4Address)
			}
			ipv4Addresses = append(ipv4Addresses, ipv4Address)
		}
	}

	var ipv6Addresses []netip.Addr
	var rawIPv6Addresses []string
	ipv6AddrConfigured := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixIpv6Addresses, &rawIPv6Addresses, t.service.Annotations)
	if ipv6AddrConfigured {
		if ipAddressType != elbv2model.IPAddressTypeDualStack {
			return nil, errors.Errorf("IPv6 addresses can only be set for dualstack load balancers")
		}
		// TODO: consider relax this requirement as ELBv2 API don't require every subnet to have IPv6 address specified.
		if len(rawIPv6Addresses) != len(ec2Subnets) {
			return nil, errors.Errorf("count of IPv6 addresses (%d) and subnets (%d) must match", len(rawIPv6Addresses), len(ec2Subnets))
		}
		for _, rawIPv6Address := range rawIPv6Addresses {
			ipv6Address, err := netip.ParseAddr(rawIPv6Address)
			if err != nil {
				return nil, errors.Errorf("IPv6 addresses must be valid IP address: %v", rawIPv6Address)
			}
			if !ipv6Address.Is6() {
				return nil, errors.Errorf("IPv6 addresses must be valid IPv6 address: %v", rawIPv6Address)
			}
			ipv6Addresses = append(ipv6Addresses, ipv6Address)
		}
	}

	subnetMappings := make([]elbv2model.SubnetMapping, 0, len(ec2Subnets))
	for idx, subnet := range ec2Subnets {
		mapping := elbv2model.SubnetMapping{
			SubnetID: awssdk.StringValue(subnet.SubnetId),
		}
		if eipConfigured {
			mapping.AllocationID = awssdk.String(eipAllocation[idx])
		}
		if ipv4AddrConfigured {
			subnetIPv4CIDRs, err := networking.GetSubnetAssociatedIPv4CIDRs(subnet)
			if err != nil {
				return nil, err
			}
			ipv4AddressesWithinSubnet := networking.FilterIPsWithinCIDRs(ipv4Addresses, subnetIPv4CIDRs)
			if len(ipv4AddressesWithinSubnet) != 1 {
				return nil, errors.Errorf("expect one private IPv4 address configured for subnet: %v", awssdk.StringValue(subnet.SubnetId))
			}
			mapping.PrivateIPv4Address = awssdk.String(ipv4AddressesWithinSubnet[0].String())
		}
		if ipv6AddrConfigured {
			subnetIPv6CIDRs, err := networking.GetSubnetAssociatedIPv6CIDRs(subnet)
			if err != nil {
				return nil, err
			}
			ipv6AddressesWithinSubnet := networking.FilterIPsWithinCIDRs(ipv6Addresses, subnetIPv6CIDRs)
			if len(ipv6AddressesWithinSubnet) != 1 {
				return nil, errors.Errorf("expect one IPv6 address configured for subnet: %v", awssdk.StringValue(subnet.SubnetId))
			}
			mapping.IPv6Address = awssdk.String(ipv6AddressesWithinSubnet[0].String())
		}
		subnetMappings = append(subnetMappings, mapping)
	}
	return subnetMappings, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSubnets(ctx context.Context, scheme elbv2model.LoadBalancerScheme) ([]*ec2sdk.Subnet, error) {
	var rawSubnetNameOrIDs []string
	if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSubnets, &rawSubnetNameOrIDs, t.service.Annotations); exists {
		return t.subnetsResolver.ResolveViaNameOrIDSlice(ctx, rawSubnetNameOrIDs,
			networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
			networking.WithSubnetsResolveLBScheme(scheme),
		)
	}

	existingLB, err := t.fetchExistingLoadBalancer(ctx)
	if err != nil {
		return nil, err
	}
	if existingLB != nil && string(scheme) == awssdk.StringValue(existingLB.LoadBalancer.Scheme) {
		availabilityZones := existingLB.LoadBalancer.AvailabilityZones
		subnetIDs := make([]string, 0, len(availabilityZones))
		for _, availabilityZone := range availabilityZones {
			subnetID := awssdk.StringValue(availabilityZone.SubnetId)
			subnetIDs = append(subnetIDs, subnetID)
		}
		return t.subnetsResolver.ResolveViaNameOrIDSlice(ctx, subnetIDs,
			networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
			networking.WithSubnetsResolveLBScheme(scheme),
		)
	}

	// for internet-facing Load Balancers, the subnets mush have at least 8 available IP addresses;
	// for internal Load Balancers, this is only required if private ip address is not assigned
	var privateIpv4Addresses []string
	ipv4Configured := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixPrivateIpv4Addresses, &privateIpv4Addresses, t.service.Annotations)
	if (scheme == elbv2model.LoadBalancerSchemeInternetFacing) ||
		((scheme == elbv2model.LoadBalancerSchemeInternal) && !ipv4Configured) {
		return t.subnetsResolver.ResolveViaDiscovery(ctx,
			networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
			networking.WithSubnetsResolveLBScheme(scheme),
			networking.WithSubnetsResolveAvailableIPAddressCount(minimalAvailableIPAddressCount),
			networking.WithSubnetsClusterTagCheck(t.featureGates.Enabled(config.SubnetsClusterTagCheck)),
		)
	}
	return t.subnetsResolver.ResolveViaDiscovery(ctx,
		networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
		networking.WithSubnetsResolveLBScheme(scheme),
		networking.WithSubnetsClusterTagCheck(t.featureGates.Enabled(config.SubnetsClusterTagCheck)),
	)
}

func (t *defaultModelBuildTask) buildLoadBalancerAttributes(_ context.Context) ([]elbv2model.LoadBalancerAttribute, error) {
	loadBalancerAttributes, err := t.getLoadBalancerAttributes()
	if err != nil {
		return []elbv2model.LoadBalancerAttribute{}, err
	}
	specificAttributes, err := t.getAnnotationSpecificLbAttributes()
	if err != nil {
		return []elbv2model.LoadBalancerAttribute{}, err
	}
	mergedAttributes := algorithm.MergeStringMap(specificAttributes, loadBalancerAttributes)
	return makeAttributesSliceFromMap(mergedAttributes), nil
}

func makeAttributesSliceFromMap(loadBalancerAttributesMap map[string]string) []elbv2model.LoadBalancerAttribute {
	attributes := make([]elbv2model.LoadBalancerAttribute, 0, len(loadBalancerAttributesMap))
	for attrKey, attrValue := range loadBalancerAttributesMap {
		attributes = append(attributes, elbv2model.LoadBalancerAttribute{
			Key:   attrKey,
			Value: attrValue,
		})
	}
	sort.Slice(attributes, func(i, j int) bool {
		return attributes[i].Key < attributes[j].Key
	})
	return attributes
}

func (t *defaultModelBuildTask) getLoadBalancerAttributes() (map[string]string, error) {
	var attributes map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixLoadBalancerAttributes, &attributes, t.service.Annotations); err != nil {
		return nil, err
	}
	dnsRecordClientRoutingPolicy, exists := attributes[lbAttrsLoadBalancingDnsClientRoutingPolicy]
	if exists {
		switch dnsRecordClientRoutingPolicy {
		case availabilityZoneAffinity:
		case partialAvailabilityZoneAffinity:
		case anyAvailabilityZone:
		default:
			return nil, errors.Errorf("invalid dns_record.client_routing_policy set in annotation %s: got '%s' expected one of ['%s', '%s', '%s']",
				annotations.SvcLBSuffixLoadBalancerAttributes, dnsRecordClientRoutingPolicy,
				anyAvailabilityZone, partialAvailabilityZoneAffinity, availabilityZoneAffinity)
		}
	}
	return attributes, nil
}

func (t *defaultModelBuildTask) getAnnotationSpecificLbAttributes() (map[string]string, error) {
	var accessLogEnabled bool
	var bucketName string
	var bucketPrefix string
	var crossZoneEnabled bool
	annotationSpecificAttrs := make(map[string]string)

	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixAccessLogEnabled, &accessLogEnabled, t.service.Annotations)
	if err != nil {
		return nil, err
	}
	if exists && accessLogEnabled {
		annotationSpecificAttrs[lbAttrsAccessLogsS3Enabled] = strconv.FormatBool(accessLogEnabled)
		if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixAccessLogS3BucketName, &bucketName, t.service.Annotations); exists {
			annotationSpecificAttrs[lbAttrsAccessLogsS3Bucket] = bucketName
		}
		if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixAccessLogS3BucketPrefix, &bucketPrefix, t.service.Annotations); exists {
			annotationSpecificAttrs[lbAttrsAccessLogsS3Prefix] = bucketPrefix
		}
	}
	exists, err = t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixCrossZoneLoadBalancingEnabled, &crossZoneEnabled, t.service.Annotations)
	if err != nil {
		return nil, err
	}
	if exists {
		annotationSpecificAttrs[lbAttrsLoadBalancingCrossZoneEnabled] = strconv.FormatBool(crossZoneEnabled)
	}
	return annotationSpecificAttrs, nil
}

var invalidLoadBalancerNamePattern = regexp.MustCompile("[[:^alnum:]]")

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context, scheme elbv2model.LoadBalancerScheme) (string, error) {
	var name string
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixLoadBalancerName, &name, t.service.Annotations); exists {
		// The name of the loadbalancer can only have up to 32 characters
		if len(name) > 32 {
			return "", errors.New("load balancer name cannot be longer than 32 characters")
		}
		return name, nil
	}
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.service.UID))
	_, _ = uuidHash.Write([]byte(scheme))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidLoadBalancerNamePattern.ReplaceAllString(t.service.Namespace, "")
	sanitizedName := invalidLoadBalancerNamePattern.ReplaceAllString(t.service.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid), nil
}
