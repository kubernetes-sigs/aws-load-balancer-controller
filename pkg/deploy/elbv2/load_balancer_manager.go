package elbv2

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

// LoadBalancerManager is responsible for create/update/delete LoadBalancer resources.
type LoadBalancerManager interface {
	Create(ctx context.Context, resLB *elbv2model.LoadBalancer) (elbv2model.LoadBalancerStatus, error)

	Update(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) (elbv2model.LoadBalancerStatus, error)

	Delete(ctx context.Context, sdkLB LoadBalancerWithTags) error
}

// NewDefaultLoadBalancerManager constructs new defaultLoadBalancerManager.
func NewDefaultLoadBalancerManager(elbv2Client services.ELBV2, trackingProvider tracking.Provider,
	taggingManager TaggingManager, externalManagedTags []string, logger logr.Logger) *defaultLoadBalancerManager {
	return &defaultLoadBalancerManager{
		elbv2Client:          elbv2Client,
		trackingProvider:     trackingProvider,
		taggingManager:       taggingManager,
		attributesReconciler: NewDefaultLoadBalancerAttributeReconciler(elbv2Client, logger),
		externalManagedTags:  externalManagedTags,
		logger:               logger,
	}
}

var _ LoadBalancerManager = &defaultLoadBalancerManager{}

// defaultLoadBalancerManager implement LoadBalancerManager
type defaultLoadBalancerManager struct {
	elbv2Client          services.ELBV2
	trackingProvider     tracking.Provider
	taggingManager       TaggingManager
	attributesReconciler LoadBalancerAttributeReconciler
	externalManagedTags  []string

	logger logr.Logger
}

func (m *defaultLoadBalancerManager) Create(ctx context.Context, resLB *elbv2model.LoadBalancer) (elbv2model.LoadBalancerStatus, error) {
	req, err := buildSDKCreateLoadBalancerInput(resLB.Spec)
	if err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	lbTags := m.trackingProvider.ResourceTags(resLB.Stack(), resLB, resLB.Spec.Tags)
	req.Tags = convertTagsToSDKTags(lbTags)

	m.logger.Info("creating loadBalancer",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID())
	resp, err := m.elbv2Client.CreateLoadBalancerWithContext(ctx, req)
	if err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}
	sdkLB := LoadBalancerWithTags{
		LoadBalancer: resp.LoadBalancers[0],
		Tags:         lbTags,
	}
	m.logger.Info("created loadBalancer",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn))
	if err := m.attributesReconciler.Reconcile(ctx, resLB, sdkLB); err != nil {
		return elbv2model.LoadBalancerStatus{}, err
	}

	if resLB.Spec.Type == elbv2model.LoadBalancerTypeNetwork && resLB.Spec.SecurityGroupsInboundRulesOnPrivateLink != nil {
		if err := m.updateSDKLoadBalancerWithSecurityGroups(ctx, resLB, sdkLB); err != nil {
			return elbv2model.LoadBalancerStatus{}, err
		}
	}

	return buildResLoadBalancerStatus(sdkLB), nil
}

func (m *defaultLoadBalancerManager) Update(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) (elbv2model.LoadBalancerStatus, error) {
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
	return buildResLoadBalancerStatus(sdkLB), nil
}

func (m *defaultLoadBalancerManager) Delete(ctx context.Context, sdkLB LoadBalancerWithTags) error {
	req := &elbv2sdk.DeleteLoadBalancerInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
	}
	m.logger.Info("deleting loadBalancer",
		"arn", awssdk.StringValue(req.LoadBalancerArn))
	if _, err := m.elbv2Client.DeleteLoadBalancerWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("deleted loadBalancer",
		"arn", awssdk.StringValue(req.LoadBalancerArn))
	return nil
}

func (m *defaultLoadBalancerManager) updateSDKLoadBalancerWithIPAddressType(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	if resLB.Spec.IPAddressType == nil {
		return nil
	}
	desiredIPAddressType := string(*resLB.Spec.IPAddressType)
	currentIPAddressType := awssdk.StringValue(sdkLB.LoadBalancer.IpAddressType)
	if desiredIPAddressType == currentIPAddressType {
		return nil
	}

	req := &elbv2sdk.SetIpAddressTypeInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
		IpAddressType:   awssdk.String(desiredIPAddressType),
	}
	changeDesc := fmt.Sprintf("%v => %v", currentIPAddressType, desiredIPAddressType)
	m.logger.Info("modifying loadBalancer ipAddressType",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn),
		"change", changeDesc)
	if _, err := m.elbv2Client.SetIpAddressTypeWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified loadBalancer ipAddressType",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn))

	return nil
}

func (m *defaultLoadBalancerManager) updateSDKLoadBalancerWithSubnetMappings(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	desiredSubnets := sets.NewString()
	for _, mapping := range resLB.Spec.SubnetMappings {
		desiredSubnets.Insert(mapping.SubnetID)
	}
	currentSubnets := sets.NewString()
	for _, az := range sdkLB.LoadBalancer.AvailabilityZones {
		currentSubnets.Insert(awssdk.StringValue(az.SubnetId))
	}
	if desiredSubnets.Equal(currentSubnets) {
		return nil
	}

	req := &elbv2sdk.SetSubnetsInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
		SubnetMappings:  buildSDKSubnetMappings(resLB.Spec.SubnetMappings),
	}
	changeDesc := fmt.Sprintf("%v => %v", currentSubnets.List(), desiredSubnets.List())
	m.logger.Info("modifying loadBalancer subnetMappings",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn),
		"change", changeDesc)
	if _, err := m.elbv2Client.SetSubnetsWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified loadBalancer subnetMappings",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn))

	return nil
}

func (m *defaultLoadBalancerManager) updateSDKLoadBalancerWithSecurityGroups(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	securityGroups, err := buildSDKSecurityGroups(resLB.Spec.SecurityGroups)
	if err != nil {
		return err
	}
	desiredSecurityGroups := sets.NewString(awssdk.StringValueSlice(securityGroups)...)
	currentSecurityGroups := sets.NewString(awssdk.StringValueSlice(sdkLB.LoadBalancer.SecurityGroups)...)

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
		req.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic = &desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic
	}

	if _, err := m.elbv2Client.SetSecurityGroupsWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified loadBalancer securityGroups",
		"stackID", resLB.Stack().StackID(),
		"resourceID", resLB.ID(),
		"arn", awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn),
		"changeSecurityGroups", changeDescriptions,
	)

	return nil
}

func (m *defaultLoadBalancerManager) checkSDKLoadBalancerWithCOIPv4Pool(_ context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	if awssdk.StringValue(resLB.Spec.CustomerOwnedIPv4Pool) != awssdk.StringValue(sdkLB.LoadBalancer.CustomerOwnedIpv4Pool) {
		m.logger.Info("loadBalancer has drifted CustomerOwnedIPv4Pool setting",
			"desired", awssdk.StringValue(resLB.Spec.CustomerOwnedIPv4Pool),
			"current", awssdk.StringValue(sdkLB.LoadBalancer.CustomerOwnedIpv4Pool))
	}
	return nil
}

func (m *defaultLoadBalancerManager) updateSDKLoadBalancerWithTags(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	desiredLBTags := m.trackingProvider.ResourceTags(resLB.Stack(), resLB, resLB.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn), desiredLBTags,
		WithCurrentTags(sdkLB.Tags),
		WithIgnoredTagKeys(m.trackingProvider.LegacyTagKeys()),
		WithIgnoredTagKeys(m.externalManagedTags))
}

func buildSDKCreateLoadBalancerInput(lbSpec elbv2model.LoadBalancerSpec) (*elbv2sdk.CreateLoadBalancerInput, error) {
	sdkObj := &elbv2sdk.CreateLoadBalancerInput{}
	sdkObj.Name = awssdk.String(lbSpec.Name)
	sdkObj.Type = awssdk.String(string(lbSpec.Type))

	if lbSpec.Scheme != nil {
		sdkObj.Scheme = (*string)(lbSpec.Scheme)
	} else {
		sdkObj.Scheme = nil
	}

	if lbSpec.IPAddressType != nil {
		sdkObj.IpAddressType = (*string)(lbSpec.IPAddressType)
	} else {
		sdkObj.IpAddressType = nil
	}

	sdkObj.SubnetMappings = buildSDKSubnetMappings(lbSpec.SubnetMappings)
	if sdkSecurityGroups, err := buildSDKSecurityGroups(lbSpec.SecurityGroups); err != nil {
		return nil, err
	} else {
		sdkObj.SecurityGroups = sdkSecurityGroups
	}

	sdkObj.CustomerOwnedIpv4Pool = lbSpec.CustomerOwnedIPv4Pool
	return sdkObj, nil
}

func buildSDKSubnetMappings(modelSubnetMappings []elbv2model.SubnetMapping) []*elbv2sdk.SubnetMapping {
	var sdkSubnetMappings []*elbv2sdk.SubnetMapping
	if len(modelSubnetMappings) != 0 {
		sdkSubnetMappings = make([]*elbv2sdk.SubnetMapping, 0, len(modelSubnetMappings))
		for _, modelSubnetMapping := range modelSubnetMappings {
			sdkSubnetMappings = append(sdkSubnetMappings, buildSDKSubnetMapping(modelSubnetMapping))
		}
	}
	return sdkSubnetMappings
}

func buildSDKSecurityGroups(modelSecurityGroups []coremodel.StringToken) ([]*string, error) {
	ctx := context.Background()
	var sdkSecurityGroups []*string
	if len(modelSecurityGroups) != 0 {
		sdkSecurityGroups = make([]*string, 0, len(modelSecurityGroups))
		for _, modelSecurityGroup := range modelSecurityGroups {
			token, err := modelSecurityGroup.Resolve(ctx)
			if err != nil {
				return nil, err
			}
			sdkSecurityGroups = append(sdkSecurityGroups, awssdk.String(token))
		}
	}
	return sdkSecurityGroups, nil
}

func buildSDKSubnetMapping(modelSubnetMapping elbv2model.SubnetMapping) *elbv2sdk.SubnetMapping {
	return &elbv2sdk.SubnetMapping{
		AllocationId:       modelSubnetMapping.AllocationID,
		PrivateIPv4Address: modelSubnetMapping.PrivateIPv4Address,
		IPv6Address:        modelSubnetMapping.IPv6Address,
		SubnetId:           awssdk.String(modelSubnetMapping.SubnetID),
	}
}

func buildResLoadBalancerStatus(sdkLB LoadBalancerWithTags) elbv2model.LoadBalancerStatus {
	return elbv2model.LoadBalancerStatus{
		LoadBalancerARN: awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn),
		DNSName:         awssdk.StringValue(sdkLB.LoadBalancer.DNSName),
	}
}

func isEnforceSGInboundRulesOnPrivateLinkUpdated(resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) (bool, string, string) {

	if resLB.Spec.Type != elbv2model.LoadBalancerTypeNetwork || resLB.Spec.SecurityGroupsInboundRulesOnPrivateLink == nil {
		return false, "", ""
	}

	desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic := string(*resLB.Spec.SecurityGroupsInboundRulesOnPrivateLink)

	var currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic string

	if sdkLB.LoadBalancer.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic != nil {
		currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic = awssdk.StringValue(sdkLB.LoadBalancer.EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic)
	}

	if desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic == currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic {
		return false, "", ""
	}

	return true, currentEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic, desiredEnforceSecurityGroupInboundRulesOnPrivateLinkTraffic

}
