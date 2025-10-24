package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"regexp"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	invalidSecurityGroupNamePtn = regexp.MustCompile("[[:^alnum:]]")
)

const (
	resourceIDManagedSecurityGroup = "ManagedLBSecurityGroup"

	managedSGDescription = "[k8s] Managed SecurityGroup for LoadBalancer"
)

type securityGroupOutput struct {
	securityGroupTokens           []core.StringToken
	backendSecurityGroupToken     core.StringToken
	backendSecurityGroupAllocated bool
}

type securityGroupBuilder interface {
	buildSecurityGroups(ctx context.Context, stack core.Stack, lbConf elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, ipAddressType elbv2model.IPAddressType) (securityGroupOutput, error)
}

type securityGroupBuilderImpl struct {
	tagHelper         tagHelper
	clusterName       string
	sgResolver        networking.SecurityGroupResolver
	backendSGProvider networking.BackendSGProvider
	loadBalancerType  elbv2model.LoadBalancerType

	enableBackendSG bool
	logger          logr.Logger
}

func newSecurityGroupBuilder(tagHelper tagHelper, clusterName string, loadBalancerType elbv2model.LoadBalancerType, enableBackendSG bool, sgResolver networking.SecurityGroupResolver, backendSGProvider networking.BackendSGProvider, logger logr.Logger) securityGroupBuilder {
	return &securityGroupBuilderImpl{
		tagHelper:         tagHelper,
		clusterName:       clusterName,
		logger:            logger,
		enableBackendSG:   enableBackendSG,
		sgResolver:        sgResolver,
		backendSGProvider: backendSGProvider,
		loadBalancerType:  loadBalancerType,
	}
}

func (builder *securityGroupBuilderImpl) buildSecurityGroups(ctx context.Context, stack core.Stack, lbConf elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, ipAddressType elbv2model.IPAddressType) (securityGroupOutput, error) {
	var sgNameOrIds []string
	if lbConf.Spec.SecurityGroups != nil {
		sgNameOrIds = *lbConf.Spec.SecurityGroups
	}

	if lbConf.Spec.DisableSecurityGroup != nil && *lbConf.Spec.DisableSecurityGroup && builder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
		return securityGroupOutput{}, nil
	}

	if len(sgNameOrIds) == 0 {
		return builder.handleManagedSecurityGroup(ctx, stack, lbConf, gw, ipAddressType)
	}

	return builder.handleExplicitSecurityGroups(ctx, lbConf, gw, sgNameOrIds)
}

func (builder *securityGroupBuilderImpl) handleManagedSecurityGroup(ctx context.Context, stack core.Stack, lbConf elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, ipAddressType elbv2model.IPAddressType) (securityGroupOutput, error) {
	var lbSGTokens []core.StringToken
	managedSG, err := builder.buildManagedSecurityGroup(stack, lbConf, gw, ipAddressType)
	if err != nil {
		return securityGroupOutput{}, err
	}
	lbSGTokens = append(lbSGTokens, managedSG.GroupID())
	var backendSecurityGroupToken core.StringToken
	var backendSGAllocated bool
	if !builder.enableBackendSG {
		backendSecurityGroupToken = managedSG.GroupID()
	} else {
		backendSecurityGroupToken, err = builder.getBackendSecurityGroup(ctx, gw)
		if err != nil {
			return securityGroupOutput{}, err
		}
		backendSGAllocated = true
		lbSGTokens = append(lbSGTokens, backendSecurityGroupToken)
	}
	builder.logger.Info("Auto Create SG", "LB SGs", lbSGTokens, "backend SG", backendSecurityGroupToken)
	return securityGroupOutput{
		securityGroupTokens:           lbSGTokens,
		backendSecurityGroupToken:     backendSecurityGroupToken,
		backendSecurityGroupAllocated: backendSGAllocated,
	}, nil
}

func (builder *securityGroupBuilderImpl) handleExplicitSecurityGroups(ctx context.Context, lbConf elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, sgNameOrIds []string) (securityGroupOutput, error) {
	var lbSGTokens []core.StringToken
	manageBackendSGRules := lbConf.Spec.ManageBackendSecurityGroupRules
	frontendSGIDs, err := builder.sgResolver.ResolveViaNameOrID(ctx, sgNameOrIds)
	if err != nil {
		return securityGroupOutput{}, err
	}
	for _, sgID := range frontendSGIDs {
		lbSGTokens = append(lbSGTokens, core.LiteralStringToken(sgID))
	}

	var backendSecurityGroupToken core.StringToken
	var backendSGAllocated bool
	if manageBackendSGRules != nil && *manageBackendSGRules {
		if !builder.enableBackendSG {
			return securityGroupOutput{}, errors.New("backendSG feature is required to manage worker node SG rules when frontendSG manually specified")
		}
		backendSecurityGroupToken, err = builder.getBackendSecurityGroup(ctx, gw)
		if err != nil {
			return securityGroupOutput{}, err
		}
		backendSGAllocated = true
		lbSGTokens = append(lbSGTokens, backendSecurityGroupToken)
	}
	builder.logger.Info("SG configured via annotation", "LB SGs", lbSGTokens, "backend SG", backendSecurityGroupToken)
	return securityGroupOutput{
		securityGroupTokens:           lbSGTokens,
		backendSecurityGroupToken:     backendSecurityGroupToken,
		backendSecurityGroupAllocated: backendSGAllocated,
	}, nil
}

func (builder *securityGroupBuilderImpl) getBackendSecurityGroup(ctx context.Context, gw *gwv1.Gateway) (core.StringToken, error) {
	backendSGID, err := builder.backendSGProvider.Get(ctx, networking.ResourceTypeGateway, []types.NamespacedName{k8s.NamespacedName(gw)})
	if err != nil {
		return nil, err
	}
	return core.LiteralStringToken(backendSGID), nil
}

func (builder *securityGroupBuilderImpl) buildManagedSecurityGroup(stack core.Stack, lbConf elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, ipAddressType elbv2model.IPAddressType) (*ec2model.SecurityGroup, error) {
	name := builder.buildManagedSecurityGroupName(gw)
	tags, err := builder.tagHelper.getLoadBalancerTags(lbConf)
	if err != nil {
		return nil, err
	}

	ingressPermissions := builder.buildManagedSecurityGroupIngressPermissions(lbConf, gw, ipAddressType)
	return ec2model.NewSecurityGroup(stack, resourceIDManagedSecurityGroup, ec2model.SecurityGroupSpec{
		GroupName:   name,
		Description: managedSGDescription,
		Tags:        tags,
		Ingress:     ingressPermissions,
	}), nil
}

func (builder *securityGroupBuilderImpl) buildManagedSecurityGroupName(gw *gwv1.Gateway) string {
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(builder.clusterName))
	_, _ = uuidHash.Write([]byte(gw.Name))
	_, _ = uuidHash.Write([]byte(gw.Namespace))
	_, _ = uuidHash.Write([]byte(gw.UID))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidSecurityGroupNamePtn.ReplaceAllString(gw.Namespace, "")
	sanitizedName := invalidSecurityGroupNamePtn.ReplaceAllString(gw.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (builder *securityGroupBuilderImpl) buildManagedSecurityGroupIngressPermissions(lbConf elbv2gw.LoadBalancerConfiguration, gw *gwv1.Gateway, ipAddressType elbv2model.IPAddressType) []ec2model.IPPermission {
	var permissions []ec2model.IPPermission

	// Default to 0.0.0.0/0 and ::/0
	// If user specified actual ranges, then these values will be overridden.
	// TODO - Document this
	sourceRanges := []string{
		"0.0.0.0/0",
		"::/0",
	}
	var prefixes []string
	var enableICMP bool

	if lbConf.Spec.SourceRanges != nil {
		sourceRanges = *lbConf.Spec.SourceRanges
	}

	if lbConf.Spec.SecurityGroupPrefixes != nil {
		prefixes = *lbConf.Spec.SecurityGroupPrefixes
	}

	if lbConf.Spec.EnableICMP != nil && *lbConf.Spec.EnableICMP {
		enableICMP = true
	}

	includeIPv6 := isIPv6Supported(ipAddressType)

	//listener loop
	for _, listener := range gw.Spec.Listeners {
		port := int32(listener.Port)
		protocol := getSgRuleProtocol(listener.Protocol)
		// CIDR Loop
		for _, cidr := range sourceRanges {
			isIPv6 := isIPv6CIDR(cidr)

			if !isIPv6 {
				permissions = append(permissions, ec2model.IPPermission{
					IPProtocol: string(protocol),
					FromPort:   awssdk.Int32(int32(port)),
					ToPort:     awssdk.Int32(int32(port)),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: cidr,
						},
					},
				})

				if enableICMP {
					permissions = append(permissions, ec2model.IPPermission{
						IPProtocol: shared_constants.ICMPV4Protocol,
						FromPort:   awssdk.Int32(shared_constants.ICMPV4TypeForPathMtu),
						ToPort:     awssdk.Int32(shared_constants.ICMPV4CodeForPathMtu),
						IPRanges: []ec2model.IPRange{
							{
								CIDRIP: cidr,
							},
						},
					})
				}

			} else if includeIPv6 {
				permissions = append(permissions, ec2model.IPPermission{
					IPProtocol: string(protocol),
					FromPort:   awssdk.Int32(int32(port)),
					ToPort:     awssdk.Int32(int32(port)),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: cidr,
						},
					},
				})

				if enableICMP {
					permissions = append(permissions, ec2model.IPPermission{
						IPProtocol: shared_constants.ICMPV6Protocol,
						FromPort:   awssdk.Int32(shared_constants.ICMPV6TypeForPathMtu),
						ToPort:     awssdk.Int32(shared_constants.ICMPV6CodeForPathMtu),
						IPv6Range: []ec2model.IPv6Range{
							{
								CIDRIPv6: cidr,
							},
						},
					})
				}
			}
		} // CIDR Loop
		// PL loop
		for _, prefixID := range prefixes {
			permissions = append(permissions, ec2model.IPPermission{
				IPProtocol: string(protocol),
				FromPort:   awssdk.Int32(int32(port)),
				ToPort:     awssdk.Int32(int32(port)),
				PrefixLists: []ec2model.PrefixList{
					{
						ListID: prefixID,
					},
				},
			})
		} // PL loop
	} // listener loop
	return permissions
}

func getSgRuleProtocol(protocol gwv1.ProtocolType) ec2types.Protocol {
	if protocol == gwv1.UDPProtocolType {
		return ec2types.ProtocolUdp
	}
	return ec2types.ProtocolTcp
}
