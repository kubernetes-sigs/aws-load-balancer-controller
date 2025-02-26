package elbv2

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

// LoadBalancerManager is responsible for create/update/delete LoadBalancer resources.
type LoadBalancerManager interface {
	Create(ctx context.Context, resLB *elbv2model.LoadBalancer) (elbv2model.LoadBalancerStatus, LoadBalancerWithTags, error)

	Update(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) (elbv2model.LoadBalancerStatus, error)

	Delete(ctx context.Context, sdkLB LoadBalancerWithTags) error
}

// NewDefaultLoadBalancerManager constructs new defaultLoadBalancerManager.
func NewDefaultLoadBalancerManager(elbv2Client services.ELBV2, trackingProvider tracking.Provider,
	taggingManager TaggingManager, externalManagedTags []string, featureGates config.FeatureGates, logger logr.Logger) *defaultLoadBalancerManager {
	return &defaultLoadBalancerManager{
		elbv2Client:                   elbv2Client,
		trackingProvider:              trackingProvider,
		taggingManager:                taggingManager,
		attributesReconciler:          NewDefaultLoadBalancerAttributeReconciler(elbv2Client, logger),
		capacityReservationReconciler: NewDefaultLoadBalancerCapacityReservationReconciler(elbv2Client, featureGates, logger),
		externalManagedTags:           externalManagedTags,
		featureGates:                  featureGates,
		logger:                        logger,
	}
}

var _ LoadBalancerManager = &defaultLoadBalancerManager{}

// defaultLoadBalancerManager implement LoadBalancerManager
type defaultLoadBalancerManager struct {
	elbv2Client                   services.ELBV2
	trackingProvider              tracking.Provider
	taggingManager                TaggingManager
	attributesReconciler          LoadBalancerAttributeReconciler
	capacityReservationReconciler LoadBalancerCapacityReservationReconciler
	externalManagedTags           []string
	featureGates                  config.FeatureGates
	logger                        logr.Logger
}

func (m *defaultLoadBalancerManager) Create(ctx context.Context, resLB *elbv2model.LoadBalancer) (elbv2model.LoadBalancerStatus, LoadBalancerWithTags, error) {
	req, err := buildSDKCreateLoadBalancerInput(resLB.Spec)
	if err != nil {
		return elbv2model.LoadBalancerStatus{}, LoadBalancerWithTags{}, err
	}
	lbTags := m.trackingProvider.ResourceTags(resLB.Stack(), resLB, resLB.Spec.Tags)
	req.Tags = convertTagsToSDKTags(lbTags)

	m.logger.Info("creating loadBalancer",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID())
	resp, err := m.elbv2Client.CreateLoadBalancerWithContext(ctx, req)
	if err != nil {
		return elbv2model.LoadBalancerStatus{}, LoadBalancerWithTags{}, err
	}
	sdkLB := LoadBalancerWithTags{
		LoadBalancer: &resp.LoadBalancers[0],
		Tags:         lbTags,
	}
	m.logger.Info("created loadBalancer",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn))
	if err := m.attributesReconciler.Reconcile(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, LoadBalancerWithTags{}, err
	}

	if resLB.Spec.Type == elbv2model.LoadBalancerTypeNetwork && resLB.Spec.SecurityGroupsInboundRulesOnPrivateLink != nil {
		if err := m.updateSDKLoadBalancerWithSecurityGroups(ctx, resLB, sdkLB); err != nil {
			return elbv2model.LoadBalancerStatus{}, LoadBalancerWithTags{}, err
		}
	}
	return buildResLoadBalancerStatus(sdkLB), sdkLB, nil
}

func (m *defaultLoadBalancerManager) Update(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) (elbv2model.LoadBalancerStatus, error) {
	// It's important to remove ipam pools first, because we need to remove any ipam pools before changing the IP Address type.
	if err := m.removeIPAMPools(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	if err := m.updateSDKLoadBalancerWithTags(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	if err := m.updateSDKLoadBalancerWithSecurityGroups(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	if err := m.updateSDKLoadBalancerWithSubnetMappings(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	if err := m.updateSDKLoadBalancerWithIPAddressType(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	if err := m.attributesReconciler.Reconcile(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	if err := m.checkSDKLoadBalancerWithCOIPv4Pool(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	// We can safely change the IPAM pool here after all other modifications are done.
	if err := m.addIPAMPools(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}

	return buildResLoadBalancerStatus(sdkLB), nil
}

func (m *defaultLoadBalancerManager) Delete(ctx context.Context, sdkLB LoadBalancerWithTags) error {
	req := &elbv2sdk.DeleteLoadBalancerInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
	}
	m.logger.Info("deleting loadBalancer",
		"arn", awssdk.ToString(req.LoadBalancerArn))
	if _, err := m.elbv2Client.DeleteLoadBalancerWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("deleted loadBalancer",
		"arn", awssdk.ToString(req.LoadBalancerArn))
	return nil
}

func (m *defaultLoadBalancerManager) updateSDKLoadBalancerWithIPAddressType(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	if &resLB.Spec.IPAddressType == nil {
		return nil
	}
	desiredIPAddressType := string(resLB.Spec.IPAddressType)
	currentIPAddressType := sdkLB.LoadBalancer.IpAddressType
	if desiredIPAddressType == string(currentIPAddressType) {
		return nil
	}

	req := &elbv2sdk.SetIpAddressTypeInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
		IpAddressType:   elbv2types.IpAddressType(desiredIPAddressType),
	}
	changeDesc := fmt.Sprintf("%v => %v", currentIPAddressType, desiredIPAddressType)
	m.logger.Info("modifying loadBalancer ipAddressType",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn),
		"change", changeDesc)
	if _, err := m.elbv2Client.SetIpAddressTypeWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified loadBalancer ipAddressType",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn))

	return nil
}

func (m *defaultLoadBalancerManager) updateSDKLoadBalancerWithSubnetMappings(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	desiredSubnets := sets.NewString()
	desiredIPv6Addresses := sets.NewString()
	desiredSubnetsSourceNATPrefixes := sets.NewString()
	currentSubnetsSourceNATPrefixes := sets.NewString()
	for _, mapping := range resLB.Spec.SubnetMappings {
		desiredSubnets.Insert(mapping.SubnetID)
		if mapping.SourceNatIpv6Prefix != nil {
			desiredSubnetsSourceNATPrefixes.Insert(awssdk.ToString(mapping.SourceNatIpv6Prefix))
		}
		if mapping.IPv6Address != nil {
			desiredIPv6Addresses.Insert(awssdk.ToString(mapping.IPv6Address))
		}
	}
	currentSubnets := sets.NewString()
	currentIPv6Addresses := sets.NewString()
	for _, az := range sdkLB.LoadBalancer.AvailabilityZones {
		currentSubnets.Insert(awssdk.ToString(az.SubnetId))
		if len(az.SourceNatIpv6Prefixes) != 0 {
			currentSubnetsSourceNATPrefixes.Insert(az.SourceNatIpv6Prefixes[0])
		}
		if len(az.LoadBalancerAddresses) > 0 && az.LoadBalancerAddresses[0].IPv6Address != nil {
			currentIPv6Addresses.Insert(awssdk.ToString(az.LoadBalancerAddresses[0].IPv6Address))
		}
	}
	sdkLBEnablePrefixForIpv6SourceNatValue := string(elbv2model.EnablePrefixForIpv6SourceNatOff)
	resLBEnablePrefixForIpv6SourceNatValue := string(elbv2model.EnablePrefixForIpv6SourceNatOff)

	sdkLBEnablePrefixForIpv6SourceNatValue = string(sdkLB.LoadBalancer.EnablePrefixForIpv6SourceNat)

	resLBEnablePrefixForIpv6SourceNatValue = string(resLB.Spec.EnablePrefixForIpv6SourceNat)

	isFirstTimeIPv6Setup := currentIPv6Addresses.Len() == 0 && desiredIPv6Addresses.Len() > 0
	needsDualstackIPv6Update := isIPv4ToDualstackUpdate(resLB, sdkLB) && isFirstTimeIPv6Setup
	if !needsDualstackIPv6Update && desiredSubnets.Equal(currentSubnets) && desiredSubnetsSourceNATPrefixes.Equal(currentSubnetsSourceNATPrefixes) && ((sdkLBEnablePrefixForIpv6SourceNatValue == resLBEnablePrefixForIpv6SourceNatValue) || (resLBEnablePrefixForIpv6SourceNatValue == "")) {
		return nil
	}
	req := &elbv2sdk.SetSubnetsInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
		SubnetMappings:  buildSDKSubnetMappings(resLB.Spec.SubnetMappings),
	}
	if resLB.Spec.Type == elbv2model.LoadBalancerTypeNetwork {
		req.EnablePrefixForIpv6SourceNat = elbv2types.EnablePrefixForIpv6SourceNatEnum(resLBEnablePrefixForIpv6SourceNatValue)
	}
	changeDesc := fmt.Sprintf("%v => %v", currentSubnets.List(), desiredSubnets.List())
	m.logger.Info("modifying loadBalancer subnetMappings",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn),
		"change", changeDesc)
	if _, err := m.elbv2Client.SetSubnetsWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified loadBalancer subnetMappings",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn))

	return nil
}

func (m *defaultLoadBalancerManager) updateSDKLoadBalancerWithSecurityGroups(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	securityGroups, err := buildSDKSecurityGroups(resLB.Spec.SecurityGroups)
	if err != nil {
		return err
	}
	desiredSecurityGroups := sets.NewString(securityGroups...)
	currentSecurityGroups := sets.NewString(sdkLB.LoadBalancer.SecurityGroups...)

	isEnforceSGInboundRulesOnPrivateLinkUpdated, currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic, desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic := isEnforceSGInboundRulesOnPrivateLinkUpdated(resLB, sdkLB)
	if desiredSecurityGroups.Equal(currentSecurityGroups) && !isEnforceSGInboundRulesOnPrivateLinkUpdated {
		return nil
	}

	var changeDescriptions []string

	if !desiredSecurityGroups.Equal(currentSecurityGroups) {
		changeSecurityGroupsDesc := fmt.Sprintf("%v => %v", currentSecurityGroups.List(), desiredSecurityGroups.List())
		changeDescriptions = append(changeDescriptions, "changeSecurityGroups", changeSecurityGroupsDesc)
	}

	req := &elbv2sdk.SetSecurityGroupsInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
		SecurityGroups:  securityGroups,
	}

	if isEnforceSGInboundRulesOnPrivateLinkUpdated {
		changeEnforceSecurityGroupInboundRulesOnPrivateLinkTrafficDesc := fmt.Sprintf("%v => %v", currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic, desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic)
		changeDescriptions = append(changeDescriptions, "changeEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic", changeEnforceSecurityGroupInboundRulesOnPrivateLinkTrafficDesc)
		req.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic = elbv2types.EnforceSecurityGroupInboundRulesOnPrivateLinkTrafficEnum(desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic)
	}

	if _, err := m.elbv2Client.SetSecurityGroupsWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified loadBalancer securityGroups",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn),
		"changeSecurityGroups", changeDescriptions,
	)

	return nil
}

func (m *defaultLoadBalancerManager) checkSDKLoadBalancerWithCOIPv4Pool(_ context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	if awssdk.ToString(resLB.Spec.CustomerOwnedIPv4Pool) != awssdk.ToString(sdkLB.LoadBalancer.CustomerOwnedIpv4Pool) {
		m.logger.Info("loadBalancer has drifted CustomerOwnedIPv4Pool setting",
			"desired", awssdk.ToString(resLB.Spec.CustomerOwnedIPv4Pool),
			"current", awssdk.ToString(sdkLB.LoadBalancer.CustomerOwnedIpv4Pool))
	}
	return nil
}

func (m *defaultLoadBalancerManager) updateSDKLoadBalancerWithTags(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	desiredLBTags := m.trackingProvider.ResourceTags(resLB.Stack(), resLB, resLB.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn), desiredLBTags,
		WithCurrentTags(sdkLB.Tags),
		WithIgnoredTagKeys(m.trackingProvider.LegacyTagKeys()),
		WithIgnoredTagKeys(m.externalManagedTags))
}

func (m *defaultLoadBalancerManager) removeIPAMPools(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	// No IPAM pool to remove or the request is to actually add / change IPAM pool.
	if sdkLB.LoadBalancer.IpamPools == nil || resLB.Spec.IPv4IPAMPool != nil {
		return nil
	}

	req := &elbv2sdk.ModifyIpPoolsInput{
		RemoveIpamPools: []elbv2types.RemoveIpamPoolEnum{elbv2types.RemoveIpamPoolEnumIpv4},
	}

	_, err := m.elbv2Client.ModifyIPPoolsWithContext(ctx, req)
	return err
}

func (m *defaultLoadBalancerManager) addIPAMPools(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	// No IPAM pool to set, this case should be handled by removeIPAMPools
	if resLB.Spec.IPv4IPAMPool == nil {
		return nil
	}

	// IPAM pool is already correctly set
	if sdkLB.LoadBalancer.IpamPools != nil && sdkLB.LoadBalancer.IpamPools.Ipv4IpamPoolId != nil {
		if *sdkLB.LoadBalancer.IpamPools.Ipv4IpamPoolId == *resLB.Spec.IPv4IPAMPool {
			return nil
		}
	}

	req := &elbv2sdk.ModifyIpPoolsInput{
		IpamPools: &elbv2types.IpamPools{
			Ipv4IpamPoolId: resLB.Spec.IPv4IPAMPool,
		},
	}

	_, err := m.elbv2Client.ModifyIPPoolsWithContext(ctx, req)
	return err
}

func buildSDKCreateLoadBalancerInput(lbSpec elbv2model.LoadBalancerSpec) (*elbv2sdk.CreateLoadBalancerInput, error) {
	sdkObj := &elbv2sdk.CreateLoadBalancerInput{}
	sdkObj.Name = awssdk.String(lbSpec.Name)
	sdkObj.Type = elbv2types.LoadBalancerTypeEnum(lbSpec.Type)
	sdkObj.Scheme = elbv2types.LoadBalancerSchemeEnum(lbSpec.Scheme)
	sdkObj.IpAddressType = elbv2types.IpAddressType(lbSpec.IPAddressType)

	sdkObj.SubnetMappings = buildSDKSubnetMappings(lbSpec.SubnetMappings)
	if sdkSecurityGroups, err := buildSDKSecurityGroups(lbSpec.SecurityGroups); err != nil {
		return nil, err
	} else {
		sdkObj.SecurityGroups = sdkSecurityGroups
	}

	if lbSpec.EnablePrefixForIpv6SourceNat != "" {
		sdkObj.EnablePrefixForIpv6SourceNat = elbv2types.EnablePrefixForIpv6SourceNatEnum(lbSpec.EnablePrefixForIpv6SourceNat)
	}

	if lbSpec.IPv4IPAMPool != nil && *lbSpec.IPv4IPAMPool != "" {
		sdkObj.IpamPools = &elbv2types.IpamPools{
			Ipv4IpamPoolId: lbSpec.IPv4IPAMPool,
		}
	}

	sdkObj.CustomerOwnedIpv4Pool = lbSpec.CustomerOwnedIPv4Pool
	return sdkObj, nil
}

func buildSDKSubnetMappings(modelSubnetMappings []elbv2model.SubnetMapping) []elbv2types.SubnetMapping {
	var sdkSubnetMappings []elbv2types.SubnetMapping
	if len(modelSubnetMappings) != 0 {
		sdkSubnetMappings = make([]elbv2types.SubnetMapping, 0, len(modelSubnetMappings))
		for _, modelSubnetMapping := range modelSubnetMappings {
			sdkSubnetMappings = append(sdkSubnetMappings, buildSDKSubnetMapping(modelSubnetMapping))
		}
	}
	return sdkSubnetMappings
}

func buildSDKSecurityGroups(modelSecurityGroups []coremodel.StringToken) ([]string, error) {
	ctx := context.Background()
	var sdkSecurityGroups []string
	if len(modelSecurityGroups) != 0 {
		sdkSecurityGroups = make([]string, 0, len(modelSecurityGroups))
		for _, modelSecurityGroup := range modelSecurityGroups {
			token, err := modelSecurityGroup.Resolve(ctx)
			if err != nil {
				return nil, err
			}
			sdkSecurityGroups = append(sdkSecurityGroups, token)
		}
	}
	return sdkSecurityGroups, nil
}

func buildSDKSubnetMapping(modelSubnetMapping elbv2model.SubnetMapping) elbv2types.SubnetMapping {
	return elbv2types.SubnetMapping{
		AllocationId:        modelSubnetMapping.AllocationID,
		PrivateIPv4Address:  modelSubnetMapping.PrivateIPv4Address,
		IPv6Address:         modelSubnetMapping.IPv6Address,
		SubnetId:            awssdk.String(modelSubnetMapping.SubnetID),
		SourceNatIpv6Prefix: modelSubnetMapping.SourceNatIpv6Prefix,
	}
}

func buildResLoadBalancerStatus(sdkLB LoadBalancerWithTags) elbv2model.LoadBalancerStatus {
	return elbv2model.LoadBalancerStatus{
		LoadBalancerARN: awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn),
		DNSName:         awssdk.ToString(sdkLB.LoadBalancer.DNSName),
	}
}

func isEnforceSGInboundRulesOnPrivateLinkUpdated(resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) (bool, string, string) {

	if resLB.Spec.Type != elbv2model.LoadBalancerTypeNetwork || resLB.Spec.SecurityGroupsInboundRulesOnPrivateLink == nil {
		return false, "", ""
	}

	desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic := string(*resLB.Spec.SecurityGroupsInboundRulesOnPrivateLink)

	var currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic string

	if sdkLB.LoadBalancer.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic != nil {
		currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic = awssdk.ToString(sdkLB.LoadBalancer.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic)
	}

	if desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic == currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic {
		return false, "", ""
	}

	return true, currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic, desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic

}

func isIPv4ToDualstackUpdate(resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) bool {
	if &resLB.Spec.IPAddressType == nil {
		return false
	}
	desiredIPAddressType := string(resLB.Spec.IPAddressType)
	currentIPAddressType := sdkLB.LoadBalancer.IpAddressType
	isIPAddressTypeUpdated := desiredIPAddressType != string(currentIPAddressType)
	return isIPAddressTypeUpdated &&
		resLB.Spec.Type == elbv2model.LoadBalancerTypeNetwork &&
		desiredIPAddressType == string(elbv2model.IPAddressTypeDualStack)
}
