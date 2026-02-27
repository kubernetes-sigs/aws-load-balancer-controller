package networking

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

const (
	TagKeySubnetInternalELB = "kubernetes.io/role/internal-elb"
	TagKeySubnetPublicELB   = "kubernetes.io/role/elb"
)

type subnetLocaleType string

const (
	subnetLocaleTypeAvailabilityZone subnetLocaleType = "availability-zone"
	subnetLocaleTypeLocalZone        subnetLocaleType = "local-zone"
	subnetLocaleTypeWavelengthZone   subnetLocaleType = "wavelength-zone"
	subnetLocaleTypeOutpost          subnetLocaleType = "outpost"
)

const (
	zoneTypeAvailabilityZone string = "availability-zone"
	zoneTypeLocalZone        string = "local-zone"
	zoneTypeWavelengthZone   string = "wavelength-zone"
)

const (
	// both ALB & NLB requires minimal 8 ip address count for it's subnet
	defaultMinimalAvailableIPAddressCount = 8
	// the ec2's vpcID filter
	ec2FilterNameVpcID = "vpc-id"
)

// options for resolve subnets.
type SubnetsResolveOptions struct {
	// The Load Balancer Type.
	// By default, it's ALB.
	LBType elbv2model.LoadBalancerType
	// The Load Balancer Scheme.
	// By default, it's internet-facing.
	LBScheme elbv2model.LoadBalancerScheme
}

// ApplyOptions applies slice of SubnetsResolveOption.
func (opts *SubnetsResolveOptions) ApplyOptions(options []SubnetsResolveOption) {
	for _, option := range options {
		option(opts)
	}
}

// defaultSubnetsResolveOptions generates the default SubnetsResolveOptions
func defaultSubnetsResolveOptions() SubnetsResolveOptions {
	return SubnetsResolveOptions{
		LBType:   elbv2model.LoadBalancerTypeApplication,
		LBScheme: elbv2model.LoadBalancerSchemeInternetFacing,
	}
}

type SubnetsResolveOption func(opts *SubnetsResolveOptions)

// WithSubnetsResolveLBType generates an option that configures LBType.
func WithSubnetsResolveLBType(lbType elbv2model.LoadBalancerType) SubnetsResolveOption {
	return func(opts *SubnetsResolveOptions) {
		opts.LBType = lbType
	}
}

// WithSubnetsResolveLBScheme generates an option that configures LBScheme.
func WithSubnetsResolveLBScheme(lbScheme elbv2model.LoadBalancerScheme) SubnetsResolveOption {
	return func(opts *SubnetsResolveOptions) {
		opts.LBScheme = lbScheme
	}
}

// SubnetsResolver is responsible for resolve EC2 Subnets for Load Balancers.
type SubnetsResolver interface {
	// ResolveViaDiscovery resolve subnets by auto discover matching subnets.
	ResolveViaDiscovery(ctx context.Context, opts ...SubnetsResolveOption) ([]ec2types.Subnet, error)

	// ResolveViaSelector resolves subnets using a SubnetSelector.
	ResolveViaSelector(ctx context.Context, selector elbv2api.SubnetSelector, opts ...SubnetsResolveOption) ([]ec2types.Subnet, error)

	// ResolveViaNameOrIDSlice resolve subnets using subnet name or ID.
	ResolveViaNameOrIDSlice(ctx context.Context, subnetNameOrIDs []string, opts ...SubnetsResolveOption) ([]ec2types.Subnet, error)

	// Checks whether the subnet is in Outpost or local-zone
	IsSubnetInLocalZoneOrOutpost(ctx context.Context, subnetID string) (bool, error)
}

// NewDefaultSubnetsResolver constructs new defaultSubnetsResolver.
func NewDefaultSubnetsResolver(
	azInfoProvider AZInfoProvider,
	ec2Client services.EC2,
	vpcID string,
	clusterName string,
	clusterTagCheckEnabled bool,
	albSingleSubnetEnabled bool,
	discoveryByReachabilityEnabled bool,
	logger logr.Logger) *defaultSubnetsResolver {
	return &defaultSubnetsResolver{
		azInfoProvider:                 azInfoProvider,
		ec2Client:                      ec2Client,
		vpcID:                          vpcID,
		clusterName:                    clusterName,
		clusterTagCheckEnabled:         clusterTagCheckEnabled,
		albSingleSubnetEnabled:         albSingleSubnetEnabled,
		discoverByReachabilityEnabled:  discoveryByReachabilityEnabled,
		minimalAvailableIPAddressCount: defaultMinimalAvailableIPAddressCount,
		logger:                         logger,
	}
}

var _ SubnetsResolver = &defaultSubnetsResolver{}

// default implementation for SubnetsResolver.
//  1. If a specific subnet name/id is provided, those subnets will be selected directly.
//  2. Otherwise, the controller selects one subnet per Availability Zone using the subnet selection algorithm:
//     a. Candidate subnet determination:
//     - If tag filters are specified: Only subnets matching these filters become candidates
//     - If no tag filters are specified:
//     * If subnets with role tags exists: Only these become candidates
//     * If no subnets have role tags: Candidates are subnets whose reachability(public/private) matches the ELB's schema
//     b. Subnet filtering:
//     - Subnets tagged for other clusters (not the current cluster) are filtered out
//     - Subnets with insufficient available IP addresses are filtered out
//     c. Final selection:
//     - One subnet per AZ is selected based on:
//     * Priority given to subnets with cluster tag for current cluster
//     * When priority is equal, selection is based on lexicographical ordering of subnet IDs
type defaultSubnetsResolver struct {
	azInfoProvider AZInfoProvider
	ec2Client      services.EC2
	vpcID          string
	clusterName    string
	// whether enable the cluster tag check on subnets
	// when enabled, only below subnets are eligible to be used as loadbalancer subnet
	// - The subnet has no Kubernetes cluster tags at all
	// - The subnet has a Kubernetes cluster tag matching the current cluster
	clusterTagCheckEnabled bool
	// whether to enable a single subnet as ALB subnet
	// by default ALB requires two subent, only allowlisted users can use a single subnet
	albSingleSubnetEnabled bool
	// whether to enable discovery subnet by reachability(public/private)
	discoverByReachabilityEnabled bool
	// the minimal available IP address required for ELB subnets
	minimalAvailableIPAddressCount int32
	logger                         logr.Logger
}

func (r *defaultSubnetsResolver) ResolveViaDiscovery(ctx context.Context, opts ...SubnetsResolveOption) ([]ec2types.Subnet, error) {
	resolveOpts := defaultSubnetsResolveOptions()
	resolveOpts.ApplyOptions(opts)

	var subnetRoleTagKey string
	var needsPublicSubnet bool
	switch resolveOpts.LBScheme {
	case elbv2model.LoadBalancerSchemeInternal:
		subnetRoleTagKey = TagKeySubnetInternalELB
		needsPublicSubnet = false
	case elbv2model.LoadBalancerSchemeInternetFacing:
		subnetRoleTagKey = TagKeySubnetPublicELB
		needsPublicSubnet = true
	default:
		return nil, fmt.Errorf("unknown lbScheme: %v", resolveOpts.LBScheme)
	}
	tagFilters := map[string][]string{subnetRoleTagKey: {"", "1"}}
	subnets, err := r.listSubnetsByTagFilters(ctx, tagFilters)
	if err != nil {
		return nil, fmt.Errorf("failed to list subnets by role tag: %w", err)
	}
	// when there are no subnets with matching role tag, we fallback to discovery by subnet reachability.
	if len(subnets) == 0 && r.discoverByReachabilityEnabled {
		subnets, err = r.listSubnetsByReachability(ctx, needsPublicSubnet)
		if err != nil {
			return nil, fmt.Errorf("failed to list subnets by reachability: %w", err)
		}
	}
	chosenSubnets, err := r.chooseAndValidateSubnetsPerAZ(ctx, subnets, resolveOpts)
	if err != nil {
		return nil, err
	}
	return chosenSubnets, nil
}

func (r *defaultSubnetsResolver) ResolveViaSelector(ctx context.Context, selector elbv2api.SubnetSelector, opts ...SubnetsResolveOption) ([]ec2types.Subnet, error) {
	resolveOpts := defaultSubnetsResolveOptions()
	resolveOpts.ApplyOptions(opts)

	if len(selector.IDs) > 0 {
		subnetIDs := make([]string, 0, len(selector.IDs))
		for _, subnetID := range selector.IDs {
			subnetIDs = append(subnetIDs, string(subnetID))
		}
		subnets, err := r.listSubnetsByIDs(ctx, subnetIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to list subnets by IDs: %w", err)
		}
		if err := r.validateSpecifiedSubnets(ctx, subnets, resolveOpts); err != nil {
			return nil, err
		}
		return subnets, nil
	}
	subnets, err := r.listSubnetsByTagFilters(ctx, selector.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to list subnets by tag filters: %w", err)
	}
	chosenSubnets, err := r.chooseAndValidateSubnetsPerAZ(ctx, subnets, resolveOpts)
	if err != nil {
		return nil, err
	}
	return chosenSubnets, nil
}

func (r *defaultSubnetsResolver) ResolveViaNameOrIDSlice(ctx context.Context, subnetNameOrIDs []string, opts ...SubnetsResolveOption) ([]ec2types.Subnet, error) {
	resolveOpts := defaultSubnetsResolveOptions()
	resolveOpts.ApplyOptions(opts)

	subnets, err := r.listSubnetsByNameOrIDs(ctx, subnetNameOrIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list subnets by names or IDs: %w", err)
	}
	if err := r.validateSpecifiedSubnets(ctx, subnets, resolveOpts); err != nil {
		return nil, err
	}
	return subnets, nil
}

// listSubnetsByNameOrIDs lists subnets within vpc matching given ID or name.
// The returned subnets will be in the same order as the input subnetNameOrIDs slice.
func (r *defaultSubnetsResolver) listSubnetsByNameOrIDs(ctx context.Context, subnetNameOrIDs []string) ([]ec2types.Subnet, error) {
	var subnetIDs []string
	var subnetNames []string
	for _, nameOrID := range subnetNameOrIDs {
		if strings.HasPrefix(nameOrID, "subnet-") {
			subnetIDs = append(subnetIDs, nameOrID)
		} else {
			subnetNames = append(subnetNames, nameOrID)
		}
	}

	subnetByID := make(map[string]ec2types.Subnet)
	subnetsByName := make(map[string][]ec2types.Subnet)

	if len(subnetIDs) > 0 {
		subnets, err := r.listSubnetsByIDs(ctx, subnetIDs)
		if err != nil {
			return nil, err
		}
		for _, subnet := range subnets {
			subnetByID[awssdk.ToString(subnet.SubnetId)] = subnet
		}
	}
	if len(subnetNames) > 0 {
		subnets, err := r.listSubnetsByNames(ctx, subnetNames)
		if err != nil {
			return nil, err
		}
		for _, subnet := range subnets {
			// Extract the Name tag value for mapping
			var subnetName string
			for _, tag := range subnet.Tags {
				if awssdk.ToString(tag.Key) == "Name" {
					subnetName = awssdk.ToString(tag.Value)
					break
				}
			}
			if subnetName != "" {
				// Use a slice to support multiple subnets with the same Name tag
				subnetsByName[subnetName] = append(subnetsByName[subnetName], subnet)
			}
		}
	}

	// Reconstruct the subnet list in the original requested order
	resolvedSubnets := make([]ec2types.Subnet, 0, len(subnetNameOrIDs))
	// Track how many times we've used each subnet name to handle duplicates
	nameUsageCount := make(map[string]int)
	for _, nameOrID := range subnetNameOrIDs {
		if strings.HasPrefix(nameOrID, "subnet-") {
			if subnet, ok := subnetByID[nameOrID]; ok {
				resolvedSubnets = append(resolvedSubnets, subnet)
			} else {
				return nil, fmt.Errorf("subnet ID not found: %s", nameOrID)
			}
		} else {
			if subnets, ok := subnetsByName[nameOrID]; ok {
				// Get the next available subnet with this name
				usageIndex := nameUsageCount[nameOrID]
				if usageIndex >= len(subnets) {
					return nil, fmt.Errorf("subnet with Name tag %q requested %d times but only %d subnet(s) found with that name", nameOrID, usageIndex+1, len(subnets))
				}
				resolvedSubnets = append(resolvedSubnets, subnets[usageIndex])
				nameUsageCount[nameOrID]++
			} else {
				return nil, fmt.Errorf("subnet with Name tag not found: %s", nameOrID)
			}
		}
	}

	return resolvedSubnets, nil
}

func (r *defaultSubnetsResolver) listSubnetsByIDs(ctx context.Context, subnetIDs []string) ([]ec2types.Subnet, error) {
	req := &ec2sdk.DescribeSubnetsInput{
		SubnetIds: subnetIDs,
	}
	subnets, err := r.ec2Client.DescribeSubnetsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(subnets) != len(subnetIDs) {
		return nil, fmt.Errorf("couldn't find all subnets, want: %v, found: %v", subnetIDs, extractSubnetIDs(subnets))
	}
	return subnets, nil
}

func (r *defaultSubnetsResolver) listSubnetsByNames(ctx context.Context, subnetNames []string) ([]ec2types.Subnet, error) {
	// Deduplicate subnet names for the AWS query while preserving order
	seen := make(map[string]bool)
	deduplicatedNames := make([]string, 0, len(subnetNames))
	for _, name := range subnetNames {
		if !seen[name] {
			deduplicatedNames = append(deduplicatedNames, name)
			seen[name] = true
		}
	}

	req := &ec2sdk.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   awssdk.String(ec2FilterNameVpcID),
				Values: []string{r.vpcID},
			},
			{
				Name:   awssdk.String("tag:Name"),
				Values: deduplicatedNames,
			},
		},
	}
	subnets, err := r.ec2Client.DescribeSubnetsAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	// Verify we found at least one subnet for each unique name
	// Note: There may be multiple subnets with the same Name tag, which is valid
	foundNames := make(map[string]bool)
	for _, subnet := range subnets {
		for _, tag := range subnet.Tags {
			if awssdk.ToString(tag.Key) == "Name" {
				foundNames[awssdk.ToString(tag.Value)] = true
				break
			}
		}
	}

	var missingNames []string
	for name := range seen {
		if !foundNames[name] {
			missingNames = append(missingNames, name)
		}
	}
	if len(missingNames) > 0 {
		return nil, fmt.Errorf("couldn't find all subnets, missing names: %v", missingNames)
	}

	return subnets, nil
}

// listSubnetsByTagFilters list subnets in vpc matches specified tag filter
func (r *defaultSubnetsResolver) listSubnetsByTagFilters(ctx context.Context, tagFilters map[string][]string) ([]ec2types.Subnet, error) {
	req := &ec2sdk.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   awssdk.String(ec2FilterNameVpcID),
				Values: []string{r.vpcID},
			},
		},
	}
	for key, values := range tagFilters {
		req.Filters = append(req.Filters, ec2types.Filter{
			Name:   awssdk.String("tag:" + key),
			Values: values,
		})
	}
	return r.ec2Client.DescribeSubnetsAsList(ctx, req)
}

// listSubnetsByReachability list subnets in vpc that matches given public/private subnet
func (r *defaultSubnetsResolver) listSubnetsByReachability(ctx context.Context, needsPublicSubnet bool) ([]ec2types.Subnet, error) {
	subnetsReq := &ec2sdk.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   awssdk.String(ec2FilterNameVpcID),
				Values: []string{r.vpcID},
			},
		},
	}
	subnets, err := r.ec2Client.DescribeSubnetsAsList(ctx, subnetsReq)
	if err != nil {
		return nil, err
	}
	routeTablesReq := &ec2sdk.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{
				Name:   awssdk.String(ec2FilterNameVpcID),
				Values: []string{r.vpcID},
			},
		},
	}
	routeTables, err := r.ec2Client.DescribeRouteTablesAsList(ctx, routeTablesReq)
	if err != nil {
		return nil, err
	}
	var subnetsMatchingReachability []ec2types.Subnet
	for _, subnet := range subnets {
		isPublicSubnet, err := r.isSubnetContainsRouteToIGW(awssdk.ToString(subnet.SubnetId), routeTables)
		if err != nil {
			return nil, err
		}
		if isPublicSubnet == needsPublicSubnet {
			subnetsMatchingReachability = append(subnetsMatchingReachability, subnet)
		}
	}
	return subnetsMatchingReachability, nil
}

// isSubnetContainsRouteToIGW tests whether a subnet contains a route to internetIGW, i.e. have route to IGW.
func (r *defaultSubnetsResolver) isSubnetContainsRouteToIGW(subnetID string, routeTables []ec2types.RouteTable) (bool, error) {
	var mainRT *ec2types.RouteTable
	var subnetRT *ec2types.RouteTable
	for i, rt := range routeTables {
		for _, rtAssoc := range rt.Associations {
			if awssdk.ToBool(rtAssoc.Main) {
				mainRT = &routeTables[i]
			}
			if awssdk.ToString(rtAssoc.SubnetId) == subnetID {
				subnetRT = &routeTables[i]
				break
			}
		}
	}
	// If there is no explicit association, the subnet will be implicitly associated with the VPC's main routing table.
	if subnetRT == nil {
		if mainRT == nil {
			// every VPC shall have a main route table. let's make this an error so we know if this precondition breaks.
			return false, fmt.Errorf("[this shall never happen] could not identify main route table for vpc: %v", r.vpcID)
		}
		subnetRT = mainRT
	}
	for _, route := range subnetRT.Routes {
		if strings.HasPrefix(awssdk.ToString(route.GatewayId), "igw") {
			return true, nil
		}
	}
	return false, nil
}

// validateSpecifiedSubnets will validate explicitly supplied subnets from given id or name.
func (r *defaultSubnetsResolver) validateSpecifiedSubnets(ctx context.Context, subnets []ec2types.Subnet, resolveOpts SubnetsResolveOptions) error {
	if len(subnets) == 0 {
		return errors.New("unable to resolve at least one subnet")
	}
	if err := r.validateSubnetsAZExclusivity(subnets); err != nil {
		return err
	}
	subnetLocale, err := r.validateSubnetsLocaleUniformity(ctx, subnets)
	if err != nil {
		return err
	}
	if err := r.validateSubnetsMinimalCount(subnets, subnetLocale, resolveOpts); err != nil {
		return err
	}
	return nil
}

// chooseAndValidateSubnetsPerAZ will choose one subnet per AZ from eligible subnets and then validate against chosen subnets.
func (r *defaultSubnetsResolver) chooseAndValidateSubnetsPerAZ(ctx context.Context, subnets []ec2types.Subnet, resolveOpts SubnetsResolveOptions) ([]ec2types.Subnet, error) {
	categorizedSubnets := r.categorizeSubnetsByEligibility(subnets)
	chosenSubnets := r.chooseSubnetsPerAZ(categorizedSubnets.eligible)
	if len(chosenSubnets) == 0 {
		return nil, fmt.Errorf("unable to resolve at least one subnet. Evaluated %d subnets: %d are tagged for other clusters, and %d have insufficient available IP addresses",
			len(subnets), len(categorizedSubnets.ineligibleClusterTag), len(categorizedSubnets.insufficientIPs))
	}
	subnetLocale, err := r.validateSubnetsLocaleUniformity(ctx, chosenSubnets)
	if err != nil {
		return nil, err
	}
	if err := r.validateSubnetsMinimalCount(chosenSubnets, subnetLocale, resolveOpts); err != nil {
		return nil, err
	}
	return chosenSubnets, nil
}

type categorizeSubnetsByEligibilityResult struct {
	eligible             []ec2types.Subnet
	ineligibleClusterTag []ec2types.Subnet
	insufficientIPs      []ec2types.Subnet
}

// categorizeSubnetsByEligibility will categorize subnets based it's eligibility of ELB subnet
func (r *defaultSubnetsResolver) categorizeSubnetsByEligibility(subnets []ec2types.Subnet) categorizeSubnetsByEligibilityResult {
	var ret categorizeSubnetsByEligibilityResult
	for _, subnet := range subnets {
		if !r.isSubnetContainsEligibleClusterTag(subnet) {
			ret.ineligibleClusterTag = append(ret.ineligibleClusterTag, subnet)
			continue
		}
		if !r.isSubnetContainsSufficientIPAddresses(subnet) {
			ret.insufficientIPs = append(ret.insufficientIPs, subnet)
			continue
		}
		ret.eligible = append(ret.eligible, subnet)
	}
	return ret
}

// chooseSubnetsPerAZ will choose one subnet per AZ.
// * subnets with current cluster tag will be prioritized.
func (r *defaultSubnetsResolver) chooseSubnetsPerAZ(subnets []ec2types.Subnet) []ec2types.Subnet {
	subnetsByAZ := mapSDKSubnetsByAZ(subnets)
	chosenSubnets := make([]ec2types.Subnet, 0, len(subnetsByAZ))
	for az, azSubnets := range subnetsByAZ {
		if len(azSubnets) == 1 {
			chosenSubnets = append(chosenSubnets, azSubnets[0])
		} else if len(azSubnets) > 1 {
			sort.Slice(azSubnets, func(i, j int) bool {
				subnetIHasCurrentClusterTag := r.isSubnetContainsCurrentClusterTag(azSubnets[i])
				subnetJHasCurrentClusterTag := r.isSubnetContainsCurrentClusterTag(azSubnets[j])
				if subnetIHasCurrentClusterTag && (!subnetJHasCurrentClusterTag) {
					return true
				} else if (!subnetIHasCurrentClusterTag) && subnetJHasCurrentClusterTag {
					return false
				}
				return awssdk.ToString(azSubnets[i].SubnetId) < awssdk.ToString(azSubnets[j].SubnetId)
			})
			r.logger.V(1).Info("multiple subnets in the same AvailabilityZone", "AvailabilityZone", az,
				"chosen", azSubnets[0].SubnetId, "ignored", extractSubnetIDs(azSubnets[1:]))
			chosenSubnets = append(chosenSubnets, azSubnets[0])
		}
	}
	sortSubnetsByID(chosenSubnets)
	return chosenSubnets
}

// isSubnetContainsCurrentClusterTag checks whether a subnet is tagged with current Kubernetes cluster tag.
func (r *defaultSubnetsResolver) isSubnetContainsCurrentClusterTag(subnet ec2types.Subnet) bool {
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", r.clusterName)
	for _, tag := range subnet.Tags {
		tagKey := awssdk.ToString(tag.Key)
		if tagKey == clusterResourceTagKey {
			return true
		}
	}
	return false
}

// isSubnetContainsEligibleClusterTag checks whether a subnet is eligible as load balancer subnet based on Kubernetes cluster tag existence
// Returns true if either:
//   - The subnet has no Kubernetes cluster tags at all
//   - The subnet has a Kubernetes cluster tag matching the current cluster
//   - The clusterTagCheck feature is disabled
//
// Returns false if the subnet only contains tags for other clusters and subnetsClusterTagCheck is enabled.
// This prevents load balancers from using subnets tagged exclusively for other clusters.
func (r *defaultSubnetsResolver) isSubnetContainsEligibleClusterTag(subnet ec2types.Subnet) bool {
	if !r.clusterTagCheckEnabled {
		return true
	}
	clusterResourceTagPrefix := "kubernetes.io/cluster"
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", r.clusterName)
	hasClusterResourceTagPrefix := false
	for _, tag := range subnet.Tags {
		tagKey := awssdk.ToString(tag.Key)
		if tagKey == clusterResourceTagKey {
			return true
		}
		if strings.HasPrefix(tagKey, clusterResourceTagPrefix) {
			// If the cluster tag is for a different cluster, keep track of it and exclude
			// the subnet if no matching tag found for the current cluster.
			hasClusterResourceTagPrefix = true
		}
	}
	return !hasClusterResourceTagPrefix
}

// isSubnetContainsSufficientIPAddresses checks whether subnet has minimal AvailableIPAddressAcount needed.
func (r *defaultSubnetsResolver) isSubnetContainsSufficientIPAddresses(subnet ec2types.Subnet) bool {
	return awssdk.ToInt32(subnet.AvailableIpAddressCount) >= r.minimalAvailableIPAddressCount
}

// validateSDKSubnetsAZExclusivity validates subnets belong to different AZs.
// subnets passed-in must be non-empty
func (r *defaultSubnetsResolver) validateSubnetsAZExclusivity(subnets []ec2types.Subnet) error {
	subnetsByAZ := mapSDKSubnetsByAZ(subnets)
	for az, azSubnets := range subnetsByAZ {
		if len(azSubnets) > 1 {
			return fmt.Errorf("multiple subnets in same Availability Zone %v: %v", az, extractSubnetIDs(azSubnets))
		}
	}
	return nil
}

// validateSDKSubnetsLocaleExclusivity validates all subnets belong to same locale, and returns the same locale.
// subnets passed-in must be non-empty
func (r *defaultSubnetsResolver) validateSubnetsLocaleUniformity(ctx context.Context, subnets []ec2types.Subnet) (subnetLocaleType, error) {
	subnetLocales := sets.NewString()
	for _, subnet := range subnets {
		subnetLocale, err := r.buildSDKSubnetLocaleType(ctx, subnet)
		if err != nil {
			return "", err
		}
		subnetLocales.Insert(string(subnetLocale))
	}
	if len(subnetLocales) > 1 {
		return "", fmt.Errorf("subnets in multiple locales: %v", subnetLocales.List())
	}
	subnetLocale, _ := subnetLocales.PopAny()
	return subnetLocaleType(subnetLocale), nil
}

// validateSubnetsMinimalCount validates subnets meets minimal count requirement.
func (r *defaultSubnetsResolver) validateSubnetsMinimalCount(subnets []ec2types.Subnet, subnetLocale subnetLocaleType, resolveOpts SubnetsResolveOptions) error {
	minimalCount := r.computeSubnetsMinimalCount(subnetLocale, resolveOpts)
	if len(subnets) < minimalCount {
		return fmt.Errorf("subnets count less than minimal required count: %v < %v", len(subnets), minimalCount)
	}
	return nil
}

// computeSubnetsMinimalCount returns the minimal count requirement for subnets.
func (r *defaultSubnetsResolver) computeSubnetsMinimalCount(subnetLocale subnetLocaleType, resolveOpts SubnetsResolveOptions) int {
	minimalCount := 1
	if resolveOpts.LBType == elbv2model.LoadBalancerTypeApplication && subnetLocale == subnetLocaleTypeAvailabilityZone && !r.albSingleSubnetEnabled {
		minimalCount = 2
	}
	return minimalCount
}

// buildSDKSubnetLocaleType builds the locale type for subnet.
func (r *defaultSubnetsResolver) buildSDKSubnetLocaleType(ctx context.Context, subnet ec2types.Subnet) (subnetLocaleType, error) {
	if subnet.OutpostArn != nil && len(*subnet.OutpostArn) != 0 {
		return subnetLocaleTypeOutpost, nil
	}
	subnetAZID := awssdk.ToString(subnet.AvailabilityZoneId)
	azInfoByAZID, err := r.azInfoProvider.FetchAZInfos(ctx, []string{subnetAZID})
	if err != nil {
		return "", err
	}
	subnetAZInfo := azInfoByAZID[subnetAZID]
	subnetZoneType := awssdk.ToString(subnetAZInfo.ZoneType)
	switch subnetZoneType {
	case zoneTypeAvailabilityZone:
		return subnetLocaleTypeAvailabilityZone, nil
	case zoneTypeLocalZone:
		return subnetLocaleTypeLocalZone, nil
	case zoneTypeWavelengthZone:
		return subnetLocaleTypeWavelengthZone, nil
	default:
		return "", fmt.Errorf("unknown zone type for subnet %v: %v", awssdk.ToString(subnet.SubnetId), subnetZoneType)
	}
}

func (r *defaultSubnetsResolver) IsSubnetInLocalZoneOrOutpost(ctx context.Context, subnetID string) (bool, error) {
	resolvedList, err := r.listSubnetsByIDs(ctx, []string{subnetID})
	if err != nil {
		return false, fmt.Errorf("failed to list subnet by ID: %w", err)
	}
	subnet := resolvedList[0]
	if subnet.OutpostArn != nil && len(*subnet.OutpostArn) != 0 {
		return true, nil
	}
	subnetAZID := awssdk.ToString(subnet.AvailabilityZoneId)
	azInfoByAZID, err := r.azInfoProvider.FetchAZInfos(ctx, []string{subnetAZID})
	if err != nil {
		return false, fmt.Errorf("failed to fetch AZ infos: %w", err)
	}
	subnetAZInfo := azInfoByAZID[subnetAZID]
	subnetZoneType := awssdk.ToString(subnetAZInfo.ZoneType)
	return zoneTypeAvailabilityZone != subnetZoneType, nil
}

// mapSDKSubnetsByAZ builds the subnets slice by AZ mapping.
func mapSDKSubnetsByAZ(subnets []ec2types.Subnet) map[string][]ec2types.Subnet {
	subnetsByAZ := make(map[string][]ec2types.Subnet)
	for _, subnet := range subnets {
		subnetAZ := awssdk.ToString(subnet.AvailabilityZone)
		subnetsByAZ[subnetAZ] = append(subnetsByAZ[subnetAZ], subnet)
	}
	return subnetsByAZ
}

// sortSubnetsByID sorts given subnets slice by subnetID.
func sortSubnetsByID(subnets []ec2types.Subnet) {
	sort.Slice(subnets, func(i, j int) bool {
		return awssdk.ToString(subnets[i].SubnetId) < awssdk.ToString(subnets[j].SubnetId)
	})
}

// extractSubnetIDs for given subnets.
func extractSubnetIDs(subnets []ec2types.Subnet) []string {
	subnetIDs := make([]string, 0, len(subnets))
	for _, subnet := range subnets {
		subnetIDs = append(subnetIDs, awssdk.ToString(subnet.SubnetId))
	}
	return subnetIDs
}
