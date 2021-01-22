package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

const (
	lbAttrsAccessLogsS3Enabled           = "access_logs.s3.enabled"
	lbAttrsAccessLogsS3Bucket            = "access_logs.s3.bucket"
	lbAttrsAccessLogsS3Prefix            = "access_logs.s3.prefix"
	lbAttrsLoadBalancingCrossZoneEnabled = "load_balancing.cross_zone.enabled"

	resourceIDLoadBalancer = "LoadBalancer"
)

func (t *defaultModelBuildTask) buildLoadBalancer(ctx context.Context, scheme elbv2model.LoadBalancerScheme) error {
	spec, err := t.buildLoadBalancerSpec(ctx, scheme)
	if err != nil {
		return err
	}
	t.loadBalancer = elbv2model.NewLoadBalancer(t.stack, resourceIDLoadBalancer, spec)
	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSpec(ctx context.Context, scheme elbv2model.LoadBalancerScheme) (elbv2model.LoadBalancerSpec, error) {
	ipAddressType, err := t.buildLoadBalancerIPAddressType(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	lbAttributes, err := t.buildLoadBalancerAttributes(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	tags, err := t.buildLoadBalancerTags(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	subnetMappings, err := t.buildLoadBalancerSubnetMappings(ctx, scheme, t.ec2Subnets)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	name := t.buildLoadBalancerName(ctx, scheme)
	spec := elbv2model.LoadBalancerSpec{
		Name:                   name,
		Type:                   elbv2model.LoadBalancerTypeNetwork,
		Scheme:                 &scheme,
		IPAddressType:          &ipAddressType,
		SubnetMappings:         subnetMappings,
		LoadBalancerAttributes: lbAttributes,
		Tags:                   tags,
	}
	return spec, nil
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

func (t *defaultModelBuildTask) buildLoadBalancerScheme(_ context.Context) (elbv2model.LoadBalancerScheme, error) {
	internal := false
	if _, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixInternal, &internal, t.service.Annotations); err != nil {
		return "", err
	}
	if internal {
		return elbv2model.LoadBalancerSchemeInternal, nil
	}
	return elbv2model.LoadBalancerSchemeInternetFacing, nil
}

func (t *defaultModelBuildTask) buildAdditionalResourceTags(_ context.Context) (map[string]string, error) {
	var annotationTags map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixAdditionalTags, &annotationTags, t.service.Annotations); err != nil {
		return nil, err
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

func (t *defaultModelBuildTask) buildLoadBalancerTags(ctx context.Context) (map[string]string, error) {
	return t.buildAdditionalResourceTags(ctx)
}

func (t *defaultModelBuildTask) buildLoadBalancerSubnetMappings(ctx context.Context, scheme elbv2model.LoadBalancerScheme, ec2Subnets []*ec2.Subnet) ([]elbv2model.SubnetMapping, error) {
	var eipAllocation []string
	eipConfigured := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixEIPAllocations, &eipAllocation, t.service.Annotations)
	var privateIpv4Addresses []string
	ipv4Configured := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixPrivateIpv4Addresses, &privateIpv4Addresses, t.service.Annotations)

	// Validation
	if eipConfigured && ipv4Configured {
		return []elbv2model.SubnetMapping{}, errors.Errorf("only one of EIP allocations or PrivateIpv4Addresses can be set")
	}
	if eipConfigured {
		if scheme == elbv2model.LoadBalancerSchemeInternal {
			return []elbv2model.SubnetMapping{}, errors.Errorf("EIP allocations can only be set for internet facing load balancers")
		} else if len(eipAllocation) != len(ec2Subnets) {
			return []elbv2model.SubnetMapping{}, errors.Errorf("number of EIP allocations (%d) and subnets (%d) must match", len(eipAllocation), len(ec2Subnets))
		}
	}
	if ipv4Configured {
		if scheme == elbv2model.LoadBalancerSchemeInternetFacing {
			return []elbv2model.SubnetMapping{}, errors.Errorf("PrivateIpv4Addresses can only be set for internal balancers")
		} else if len(privateIpv4Addresses) != len(ec2Subnets) {
			return []elbv2model.SubnetMapping{}, errors.Errorf("number of PrivateIpv4Addresses (%d) and subnets (%d) must match", len(privateIpv4Addresses), len(ec2Subnets))
		}
	}

	subnetMappings := make([]elbv2model.SubnetMapping, 0, len(ec2Subnets))
	for idx, subnet := range ec2Subnets {
		mapping := elbv2model.SubnetMapping{
			SubnetID: aws.StringValue(subnet.SubnetId),
		}
		if eipConfigured {
			mapping.AllocationID = aws.String(eipAllocation[idx])
		}
		if ipv4Configured {
			ip, err := t.getMatchingIPforSubnet(ctx, subnet, privateIpv4Addresses)
			if err != nil {
				return []elbv2model.SubnetMapping{}, err
			}
			mapping.PrivateIPv4Address = aws.String(ip)
		}
		subnetMappings = append(subnetMappings, mapping)
	}
	return subnetMappings, nil
}

// Return the ip address which is in the subnet. Error if not match
// Can be extended for ipv6 if required
func (t *defaultModelBuildTask) getMatchingIPforSubnet(_ context.Context, subnet *ec2.Subnet, privateIpv4Addresses []string) (string, error) {
	_, ipv4Net, err := net.ParseCIDR(*subnet.CidrBlock)
	if err != nil {
		return "", errors.Wrap(err, "subnet CIDR block could not be parsed")
	}
	for _, ipString := range privateIpv4Addresses {
		if ipv4Net.Contains(net.ParseIP(ipString)) {
			return ipString, nil
		}
	}
	return "", errors.Errorf("no matching ip for subnet %s", *subnet.SubnetId)
}

func (t *defaultModelBuildTask) resolveLoadBalancerSubnets(ctx context.Context, scheme elbv2model.LoadBalancerScheme) ([]*ec2.Subnet, error) {
	var rawSubnetNameOrIDs []string
	if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSubnets, &rawSubnetNameOrIDs, t.service.Annotations); exists {
		return t.subnetsResolver.ResolveViaNameOrIDSlice(ctx, rawSubnetNameOrIDs,
			networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
			networking.WithSubnetsResolveLBScheme(scheme),
		)
	}
	return t.subnetsResolver.ResolveViaDiscovery(ctx,
		networking.WithSubnetsResolveLBType(elbv2model.LoadBalancerTypeNetwork),
		networking.WithSubnetsResolveLBScheme(scheme),
	)
}

func (t *defaultModelBuildTask) buildLoadBalancerAttributes(_ context.Context) ([]elbv2model.LoadBalancerAttribute, error) {
	var attrs []elbv2model.LoadBalancerAttribute
	accessLogEnabled := t.defaultAccessLogS3Enabled
	bucketName := t.defaultAccessLogsS3Bucket
	bucketPrefix := t.defaultAccessLogsS3Prefix
	if _, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixAccessLogEnabled, &accessLogEnabled, t.service.Annotations); err != nil {
		return []elbv2model.LoadBalancerAttribute{}, err
	}
	if accessLogEnabled {
		t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixAccessLogS3BucketName, &bucketName, t.service.Annotations)
		t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixAccessLogS3BucketPrefix, &bucketPrefix, t.service.Annotations)
	}
	crossZoneEnabled := t.defaultLoadBalancingCrossZoneEnabled
	if _, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixCrossZoneLoadBalancingEnabled, &crossZoneEnabled, t.service.Annotations); err != nil {
		return []elbv2model.LoadBalancerAttribute{}, err
	}

	attrs = []elbv2model.LoadBalancerAttribute{
		{
			Key:   lbAttrsAccessLogsS3Enabled,
			Value: strconv.FormatBool(accessLogEnabled),
		},
		{
			Key:   lbAttrsAccessLogsS3Bucket,
			Value: bucketName,
		},
		{
			Key:   lbAttrsAccessLogsS3Prefix,
			Value: bucketPrefix,
		},
		{
			Key:   lbAttrsLoadBalancingCrossZoneEnabled,
			Value: strconv.FormatBool(crossZoneEnabled),
		},
	}

	return attrs, nil
}

var invalidLoadBalancerNamePattern = regexp.MustCompile("[[:^alnum:]]")

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context, scheme elbv2model.LoadBalancerScheme) string {
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.service.UID))
	_, _ = uuidHash.Write([]byte(scheme))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidLoadBalancerNamePattern.ReplaceAllString(t.service.Namespace, "")
	sanitizedName := invalidLoadBalancerNamePattern.ReplaceAllString(t.service.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}
