package model

import (
	"context"
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
	"strconv"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/certs"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TODO: Add more relevant info like TLS settings and hostnames later wherever applicable
type gwListenerConfig struct {
	protocol  elbv2model.Protocol
	hostnames []string
}

type listenerBuilder interface {
	buildListeners(ctx context.Context, stack core.Stack, lb *elbv2model.LoadBalancer, securityGroups securityGroupOutput, subnets buildLoadBalancerSubnetsOutput, gw *gwv1.Gateway, routes map[int32][]routeutils.RouteDescriptor, lbConf elbv2gw.LoadBalancerConfiguration) error
	buildListenerSpec(ctx context.Context, lb *elbv2model.LoadBalancer, gw *gwv1.Gateway, port int32, lbCfg elbv2gw.LoadBalancerConfiguration, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*elbv2model.ListenerSpec, error)
	buildL7ListenerSpec(ctx context.Context, lb *elbv2model.LoadBalancer, subnets buildLoadBalancerSubnetsOutput, gw *gwv1.Gateway, lbCfg elbv2gw.LoadBalancerConfiguration, port int32, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*elbv2model.ListenerSpec, error)
	buildL4ListenerSpec(ctx context.Context, stack core.Stack, lb *elbv2model.LoadBalancer, securityGroups securityGroupOutput, gw *gwv1.Gateway, lbCfg elbv2gw.LoadBalancerConfiguration, port int32, routes []routeutils.RouteDescriptor, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*elbv2model.ListenerSpec, error)
}

type listenerBuilderImpl struct {
	elbv2Client      services.ELBV2
	subnetsResolver  networking.SubnetsResolver
	loadBalancerType elbv2model.LoadBalancerType
	clusterName      string
	tagHelper        tagHelper
	tgBuilder        targetGroupBuilder
	defaultSSLPolicy string
	certDiscovery    certs.CertDiscovery
	logger           logr.Logger
}

func (l listenerBuilderImpl) buildListeners(ctx context.Context, stack core.Stack, lb *elbv2model.LoadBalancer, securityGroups securityGroupOutput, subnets buildLoadBalancerSubnetsOutput, gw *gwv1.Gateway, routes map[int32][]routeutils.RouteDescriptor, lbCfg elbv2gw.LoadBalancerConfiguration) error {
	gwLsCfgs, err := mapGatewayListenerConfigsByPort(gw)
	if err != nil {
		return err
	}
	gwLsPorts := sets.Int32KeySet(gwLsCfgs)
	portsWithRoutes := sets.Int32KeySet(routes)
	// Materialise the listener only if listener has associated routes
	if len(gwLsPorts.Intersection(portsWithRoutes).List()) != 0 {
		lbLsCfgs := mapLoadBalancerListenerConfigsByPort(lbCfg)
		for _, port := range gwLsPorts.Intersection(portsWithRoutes).List() {
			ls, err := l.buildListener(ctx, stack, lb, securityGroups, subnets, gw, port, routes[port], lbCfg, gwLsCfgs[port], lbLsCfgs[port])
			if err != nil {
				return err
			}

			if ls == nil {
				continue
			}

			// build rules only for L7 gateways
			if l.loadBalancerType == elbv2model.LoadBalancerTypeApplication {
				if err := l.buildListenerRules(stack, ls, lb.Spec.IPAddressType, securityGroups, gw, port, lbCfg, routes); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (l listenerBuilderImpl) buildListener(ctx context.Context, stack core.Stack, lb *elbv2model.LoadBalancer, securityGroups securityGroupOutput, subnets buildLoadBalancerSubnetsOutput, gw *gwv1.Gateway, port int32, routes []routeutils.RouteDescriptor, lbCfg elbv2gw.LoadBalancerConfiguration, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*elbv2model.Listener, error) {
	var listenerSpec *elbv2model.ListenerSpec

	var err error
	if l.loadBalancerType == elbv2model.LoadBalancerTypeApplication {
		listenerSpec, err = l.buildL7ListenerSpec(ctx, lb, subnets, gw, lbCfg, port, gwLsCfg, lbLsCfg)
	} else {
		listenerSpec, err = l.buildL4ListenerSpec(ctx, stack, lb, securityGroups, gw, lbCfg, port, routes, gwLsCfg, lbLsCfg)
	}
	if err != nil {
		return nil, err
	}

	if listenerSpec == nil {
		return nil, nil
	}

	lsResID := fmt.Sprintf("%v", port)
	return elbv2model.NewListener(stack, lsResID, *listenerSpec), nil
}

func (l listenerBuilderImpl) buildListenerSpec(ctx context.Context, lb *elbv2model.LoadBalancer, gw *gwv1.Gateway, port int32, lbCfg elbv2gw.LoadBalancerConfiguration, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*elbv2model.ListenerSpec, error) {
	tags, err := l.buildListenerTags(gw, port, lbCfg, lbLsCfg)
	if err != nil {
		return &elbv2model.ListenerSpec{}, err
	}
	lsAttributes, attributesErr := buildListenerAttributes(lbLsCfg)
	if attributesErr != nil {
		return &elbv2model.ListenerSpec{}, attributesErr
	}
	sslPolicy, sslPolicyErr := l.buildSSLPolicy(gwLsCfg, lbLsCfg)
	if sslPolicyErr != nil {
		return &elbv2model.ListenerSpec{}, sslPolicyErr
	}
	certificates, certsErr := l.buildCertificates(ctx, gw, port, gwLsCfg, lbLsCfg)
	if certsErr != nil {
		return &elbv2model.ListenerSpec{}, certsErr
	}
	listenerSpec := &elbv2model.ListenerSpec{
		LoadBalancerARN:    lb.LoadBalancerARN(),
		Port:               port,
		Protocol:           gwLsCfg.protocol,
		Certificates:       certificates,
		SSLPolicy:          sslPolicy,
		Tags:               tags,
		ListenerAttributes: lsAttributes,
	}
	return listenerSpec, nil
}

func (l listenerBuilderImpl) buildL7ListenerSpec(ctx context.Context, lb *elbv2model.LoadBalancer, subnets buildLoadBalancerSubnetsOutput, gw *gwv1.Gateway, lbCfg elbv2gw.LoadBalancerConfiguration, port int32, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*elbv2model.ListenerSpec, error) {
	listenerSpec, err := l.buildListenerSpec(ctx, lb, gw, port, lbCfg, gwLsCfg, lbLsCfg)
	if err != nil {
		return &elbv2model.ListenerSpec{}, err
	}
	listenerSpec.DefaultActions = buildL7ListenerDefaultActions()
	mutualAuth, err := l.buildMutualAuthenticationAttributes(ctx, l.subnetsResolver, subnets, gwLsCfg, lbLsCfg)
	if err != nil {
		return &elbv2model.ListenerSpec{}, err
	}
	listenerSpec.MutualAuthentication = mutualAuth
	return listenerSpec, nil
}

func (l listenerBuilderImpl) buildL4ListenerSpec(ctx context.Context, stack core.Stack, lb *elbv2model.LoadBalancer, securityGroups securityGroupOutput, gw *gwv1.Gateway, lbCfg elbv2gw.LoadBalancerConfiguration, port int32, routes []routeutils.RouteDescriptor, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*elbv2model.ListenerSpec, error) {
	listenerSpec, err := l.buildListenerSpec(ctx, lb, gw, port, lbCfg, gwLsCfg, lbLsCfg)
	if err != nil {
		return &elbv2model.ListenerSpec{}, err
	}
	alpnPolicy, err := buildListenerALPNPolicy(listenerSpec.Protocol, lbLsCfg)
	if err != nil {
		return &elbv2model.ListenerSpec{}, err
	}
	listenerSpec.ALPNPolicy = alpnPolicy

	// For L4 Gateways we will assume that each L4 gateway Listener will have a single L4 route and each route will only have a single backendRef as weighted tgs are not supported for NLBs.
	if len(routes) > 1 {
		return &elbv2model.ListenerSpec{}, errors.Errorf("multiple routes %+v are not supported for listener %v:%v for gateway %v", routes, listenerSpec.Protocol, port, k8s.NamespacedName(gw))
	}
	routeDescriptor := routes[0]
	if routeDescriptor.GetAttachedRules()[0].GetBackends() == nil || len(routeDescriptor.GetAttachedRules()[0].GetBackends()) == 0 {
		l.logger.Info("Skipping listener creation due to no backend ref found for route", "route", routeDescriptor.GetRouteNamespacedName(), "listener", fmt.Sprintf("%v:%v", listenerSpec.Protocol, port), "gateway", k8s.NamespacedName(gw))
		return nil, nil
	}
	if len(routeDescriptor.GetAttachedRules()[0].GetBackends()) > 1 {
		return &elbv2model.ListenerSpec{}, errors.Errorf("multiple backend refs found for route %v for listener on port:protocol %v:%v for gateway %v , only one must be specified", routeDescriptor.GetRouteNamespacedName(), port, listenerSpec.Protocol, k8s.NamespacedName(gw))
	}
	backend := routeDescriptor.GetAttachedRules()[0].GetBackends()[0]

	if backend.Weight == 0 {
		l.logger.Info("Ignoring NLB backend with 0 weight.", "route", routeDescriptor.GetRouteNamespacedName())
		return nil, nil
	}

	targetGroup, tgErr := l.tgBuilder.buildTargetGroup(stack, gw, lbCfg, lb.Spec.IPAddressType, routeDescriptor, backend, securityGroups.backendSecurityGroupToken)
	if tgErr != nil {
		return &elbv2model.ListenerSpec{}, tgErr
	}
	listenerSpec.DefaultActions = buildL4ListenerDefaultActions(targetGroup)
	return listenerSpec, nil
}

func (l listenerBuilderImpl) buildListenerRules(stack core.Stack, ls *elbv2model.Listener, ipAddressType elbv2model.IPAddressType, securityGroups securityGroupOutput, gw *gwv1.Gateway, port int32, lbCfg elbv2gw.LoadBalancerConfiguration, routes map[int32][]routeutils.RouteDescriptor) error {
	// sort all rules based on precedence
	rulesWithPrecedenceOrder := routeutils.SortAllRulesByPrecedence(routes[port])

	var albRules []elbv2model.Rule
	for _, ruleWithPrecedence := range rulesWithPrecedenceOrder {
		route := ruleWithPrecedence.CommonRulePrecedence.RouteDescriptor
		rule := ruleWithPrecedence.CommonRulePrecedence.Rule

		// Build Rule Conditions
		var conditionsList []elbv2model.RuleCondition
		var err error
		switch route.GetRouteKind() {
		case routeutils.HTTPRouteKind:
			conditionsList, err = routeutils.BuildHttpRuleConditions(ruleWithPrecedence)
		case routeutils.GRPCRouteKind:
			conditionsList, err = routeutils.BuildGrpcRuleConditions(ruleWithPrecedence)
		}
		if err != nil {
			return err
		}

		// set up for building routing actions
		var actions []elbv2model.Action
		var routingAction *elbv2gw.Action
		if rule.GetListenerRuleConfig() != nil {
			routingAction = getRoutingAction(rule.GetListenerRuleConfig())
		}
		targetGroupTuples := make([]elbv2model.TargetGroupTuple, 0, len(rule.GetBackends()))
		for _, backend := range rule.GetBackends() {
			targetGroup, tgErr := l.tgBuilder.buildTargetGroup(stack, gw, lbCfg, ipAddressType, route, backend, securityGroups.backendSecurityGroupToken)
			if tgErr != nil {
				return tgErr
			}
			// weighted target group support
			weight := int32(backend.Weight)
			targetGroupTuples = append(targetGroupTuples, elbv2model.TargetGroupTuple{
				TargetGroupARN: targetGroup.TargetGroupARN(),
				Weight:         &weight,
			})
		}
		// Build Rule Routing Actions
		actions, err = routeutils.BuildRuleRoutingActions(rule, route, routingAction, targetGroupTuples)
		if err != nil {
			return err
		}

		// TODO: build rule auth actions

		if len(actions) == 0 {
			l.logger.Info("Filling in no backend actions with fixed 503")
			actions = buildL7ListenerNoBackendActions()
		}

		tags, tagsErr := l.tagHelper.getGatewayTags(lbCfg)
		if tagsErr != nil {
			return tagsErr
		}

		albRules = append(albRules, elbv2model.Rule{
			Conditions: conditionsList,
			Actions:    actions,
			Tags:       tags,
		})

	}

	priority := int32(1)
	for _, rule := range albRules {
		ruleResID := fmt.Sprintf("%v:%v", port, priority)
		_ = elbv2model.NewListenerRule(stack, ruleResID, elbv2model.ListenerRuleSpec{
			ListenerARN: ls.ListenerARN(),
			Priority:    priority,
			Conditions:  rule.Conditions,
			Actions:     rule.Actions,
			Tags:        rule.Tags,
		})
		priority += 1
	}
	return nil
}

func (l listenerBuilderImpl) buildListenerTags(gw *gwv1.Gateway, port int32, lbCfg elbv2gw.LoadBalancerConfiguration, lbLsCfg *elbv2gw.ListenerConfiguration) (map[string]string, error) {
	// TODO Add proper gateway tags for listener
	return l.tagHelper.getGatewayTags(lbCfg)
}

func buildListenerAttributes(lsCfg *elbv2gw.ListenerConfiguration) ([]elbv2model.ListenerAttribute, error) {
	if lsCfg == nil || lsCfg.ListenerAttributes == nil || len(lsCfg.ListenerAttributes) == 0 {
		return []elbv2model.ListenerAttribute{}, nil
	}
	attributes := make([]elbv2model.ListenerAttribute, 0, len(lsCfg.ListenerAttributes))
	for _, attr := range lsCfg.ListenerAttributes {
		attributes = append(attributes, elbv2model.ListenerAttribute{
			Key:   attr.Key,
			Value: attr.Value,
		})
	}
	return attributes, nil
}

func (l listenerBuilderImpl) buildCertificates(ctx context.Context, gw *gwv1.Gateway, port int32, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) ([]elbv2model.Certificate, error) {
	if !isSecureProtocol(gwLsCfg.protocol) {
		return []elbv2model.Certificate{}, nil
	}
	certs := make([]elbv2model.Certificate, 0)
	// Build explict certs
	if lbLsCfg != nil {
		certs = append(certs, l.buildExplicitTLSCertARNs(ctx, *lbLsCfg)...)
	}
	// If any explicit certs are not found then build inferred certs using cert discovery
	if len(certs) == 0 {
		if len(gwLsCfg.hostnames) == 0 {
			return []elbv2model.Certificate{}, errors.Errorf("No hostnames found for TLS cert discovery for listener on gateway %s with protocol:port %s:%v", k8s.NamespacedName(gw), gwLsCfg.protocol, port)
		}
		discoveredCerts, err := l.buildInferredTLSCertARNs(ctx, gwLsCfg.hostnames)
		if err != nil {
			l.logger.Error(err, "Unable to discover certs for listener on gateway %s with protocol:port %s:%v\", k8s.NamespacedName(gw), gwLsCfg.protocol, port")
			return []elbv2model.Certificate{}, err
		}
		for _, cert := range discoveredCerts {
			certs = append(certs, elbv2model.Certificate{
				CertificateARN: &cert,
			})
		}
	}
	return certs, nil

}

func (l listenerBuilderImpl) buildExplicitTLSCertARNs(ctx context.Context, listener elbv2gw.ListenerConfiguration) []elbv2model.Certificate {
	var certs []elbv2model.Certificate
	if listener.DefaultCertificate != nil {
		certs = append(certs, elbv2model.Certificate{
			CertificateARN: listener.DefaultCertificate,
		})
	}

	if listener.Certificates != nil {
		for _, cert := range listener.Certificates {
			certs = append(certs, elbv2model.Certificate{
				CertificateARN: cert,
			})
		}
	}
	return certs
}

func (l listenerBuilderImpl) buildInferredTLSCertARNs(ctx context.Context, hostnames []string) ([]string, error) {
	hosts := sets.NewString()
	for _, hostname := range hostnames {
		hosts.Insert(hostname)
	}

	return l.certDiscovery.Discover(ctx, hosts.List())
}

// L7 listeners will always have 404 as default actions since we don't have dedicated backend
func buildL7ListenerDefaultActions() []elbv2model.Action {
	action404 := elbv2model.Action{
		Type: elbv2model.ActionTypeFixedResponse,
		FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
			ContentType: awssdk.String("text/plain"),
			StatusCode:  "404",
		},
	}
	return []elbv2model.Action{action404}
}

// returns 503 when no backends are configured
func buildL7ListenerNoBackendActions() []elbv2model.Action {
	action503 := elbv2model.Action{
		Type: elbv2model.ActionTypeFixedResponse,
		FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
			ContentType: awssdk.String("text/plain"),
			StatusCode:  "503",
		},
	}
	return []elbv2model.Action{action503}
}

func buildL4ListenerDefaultActions(targetGroup *elbv2model.TargetGroup) []elbv2model.Action {
	return []elbv2model.Action{
		{
			Type: elbv2model.ActionTypeForward,
			ForwardConfig: &elbv2model.ForwardActionConfig{
				TargetGroups: []elbv2model.TargetGroupTuple{
					{
						TargetGroupARN: targetGroup.TargetGroupARN(),
					},
				},
			},
		},
	}
}

func (l listenerBuilderImpl) buildMutualAuthenticationAttributes(ctx context.Context, subnetsResolver networking.SubnetsResolver, subnets buildLoadBalancerSubnetsOutput, gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*elbv2model.MutualAuthenticationAttributes, error) {
	// Skip mTLS configuration for non-secure protocols
	if !isSecureProtocol(gwLsCfg.protocol) {
		return nil, nil
	}
	// mTLS is not supported for Local Zone or Outposts AZs
	// We only need to check one of the subnets at this point since we have already resolved subnets for this LoadBalancer and we don't allow mix AZs
	isSubnetInLocalZoneOrOutpost, err := subnetsResolver.IsSubnetInLocalZoneOrOutpost(ctx, subnets.subnets[0].SubnetID)
	if err != nil {
		return nil, err
	}
	if isSubnetInLocalZoneOrOutpost {
		// Skip mTLS configuration for Local Zone or Outpost subnets
		l.logger.V(1).Info("skipping mutual authentication configuration as it is not supported in local zone or outpost")
		return nil, nil
	}

	// Default to "off" mode when no explicit mTLS configuration exists
	if lbLsCfg == nil || lbLsCfg.MutualAuthentication == nil {
		return &elbv2model.MutualAuthenticationAttributes{
			Mode: string(elbv2gw.MutualAuthenticationOffMode),
		}, nil
	}
	mode := string(lbLsCfg.MutualAuthentication.Mode)

	// Process trustStore information for verify mode
	var trustStoreArn *string
	if mode == string(elbv2model.MutualAuthenticationVerifyMode) {
		trustStoreName := awssdk.ToString(lbLsCfg.MutualAuthentication.TrustStore)
		if !strings.HasPrefix(trustStoreName, "arn:") {
			truststoreARNs, err := shared_utils.GetTrustStoreArnFromName(ctx, l.elbv2Client, []string{trustStoreName})
			if err != nil {
				return nil, errors.Wrapf(err, "failed to resolve trustStore ARN for name %s", trustStoreName)
			}
			trustStoreArn = truststoreARNs[trustStoreName]
		} else {
			// Already an ARN, use as-is
			trustStoreArn = awssdk.String(trustStoreName)
		}
	}

	// Initialize with empty default values
	var advertiseTrustStoreCaNames string
	if lbLsCfg.MutualAuthentication.AdvertiseTrustStoreCaNames != nil {
		advertiseTrustStoreCaNames = string(*lbLsCfg.MutualAuthentication.AdvertiseTrustStoreCaNames)
	}

	// Set default ignoreClientCert to false for verify mode if not specified
	ignoreClientCert := lbLsCfg.MutualAuthentication.IgnoreClientCertificateExpiry
	if mode == string(elbv2model.MutualAuthenticationVerifyMode) && ignoreClientCert == nil {
		ignoreClientCert = awssdk.Bool(false)
	}

	// Build the complete mutual authentication configuration
	return &elbv2model.MutualAuthenticationAttributes{
		Mode:                          mode,
		TrustStoreArn:                 trustStoreArn,
		IgnoreClientCertificateExpiry: ignoreClientCert,
		AdvertiseTrustStoreCaNames:    &advertiseTrustStoreCaNames,
	}, nil
}

func (l listenerBuilderImpl) buildSSLPolicy(gwLsCfg *gwListenerConfig, lbLsCfg *elbv2gw.ListenerConfiguration) (*string, error) {
	if !isSecureProtocol(gwLsCfg.protocol) {
		return nil, nil
	}
	if lbLsCfg == nil || lbLsCfg.SslPolicy == nil {
		return &l.defaultSSLPolicy, nil
	}
	return lbLsCfg.SslPolicy, nil
}

func isSecureProtocol(protocol elbv2model.Protocol) bool {
	return protocol == elbv2model.ProtocolHTTPS || protocol == elbv2model.ProtocolTLS
}

func buildListenerALPNPolicy(listenerProtocol elbv2model.Protocol, lbLsCfg *elbv2gw.ListenerConfiguration) ([]string, error) {
	if listenerProtocol != elbv2model.ProtocolTLS {
		return nil, nil
	}
	if lbLsCfg == nil || lbLsCfg.ALPNPolicy == nil {
		return []string{string(elbv2gw.ALPNPolicyNone)}, nil
	}
	rawALPNPolicy := *lbLsCfg.ALPNPolicy
	switch rawALPNPolicy {
	case elbv2gw.ALPNPolicyNone, elbv2gw.ALPNPolicyHTTP1Only, elbv2gw.ALPNPolicyHTTP2Only,
		elbv2gw.ALPNPolicyHTTP2Preferred, elbv2gw.ALPNPolicyHTTP2Optional:
		return []string{string(rawALPNPolicy)}, nil
	default:
		return nil, errors.Errorf("invalid ALPN policy %v, policy must be one of [%v, %v, %v, %v, %v]",
			string(rawALPNPolicy), elbv2gw.ALPNPolicyNone, elbv2gw.ALPNPolicyHTTP1Only, elbv2gw.ALPNPolicyHTTP2Only,
			elbv2gw.ALPNPolicyHTTP2Optional, elbv2gw.ALPNPolicyHTTP2Preferred)
	}

}

// mapGatewayListenerConfigsByPort creates a mapping of ports to listener configurations from the Gateway listeners.
func mapGatewayListenerConfigsByPort(gw *gwv1.Gateway) (map[int32]*gwListenerConfig, error) {
	gwListenerConfigs := make(map[int32]*gwListenerConfig)
	for _, listener := range gw.Spec.Listeners {
		port := int32(listener.Port)
		protocol := listener.Protocol
		if gwListenerConfigs[port] != nil && string(gwListenerConfigs[port].protocol) != string(protocol) {
			return nil, fmt.Errorf("invalid listeners on gateway, listeners with same ports cannot have different protocols")
		}
		if gwListenerConfigs[port] == nil {
			gwListenerConfigs[port] = &gwListenerConfig{
				protocol:  elbv2model.Protocol(protocol),
				hostnames: []string{},
			}
		}
		hostnames := gwListenerConfigs[port].hostnames
		if listener.Hostname != nil {
			hostnames = append(hostnames, string(*listener.Hostname))
			gwListenerConfigs[port].hostnames = hostnames
		}
	}
	return gwListenerConfigs, nil
}

// mapLoadBalancerListenerConfigsByPort creates a mapping of ports to their corresponding
// listener configurations from the LoadBalancer configuration.
func mapLoadBalancerListenerConfigsByPort(lbCfg elbv2gw.LoadBalancerConfiguration) map[int32]*elbv2gw.ListenerConfiguration {
	lbLsCfgs := make(map[int32]*elbv2gw.ListenerConfiguration)
	if lbCfg.Spec.ListenerConfigurations == nil {
		return lbLsCfgs
	}
	for _, lsCfg := range *lbCfg.Spec.ListenerConfigurations {
		port, _ := strconv.ParseInt(strings.Split(string(lsCfg.ProtocolPort), ":")[1], 10, 64)
		lbLsCfgs[int32(port)] = &lsCfg
	}
	return lbLsCfgs
}

func newListenerBuilder(ctx context.Context, loadBalancerType elbv2model.LoadBalancerType, tgBuilder targetGroupBuilder, tagHelper tagHelper, clusterName string, defaultSSLPolicy string, elbv2Client services.ELBV2, acmClient services.ACM, allowedCAARNs []string, subnetsResolver networking.SubnetsResolver, logger logr.Logger) listenerBuilder {
	certDiscovery := certs.NewACMCertDiscovery(acmClient, allowedCAARNs, logger)
	return &listenerBuilderImpl{
		subnetsResolver:  subnetsResolver,
		elbv2Client:      elbv2Client,
		loadBalancerType: loadBalancerType,
		tgBuilder:        tgBuilder,
		clusterName:      clusterName,
		tagHelper:        tagHelper,
		defaultSSLPolicy: defaultSSLPolicy,
		certDiscovery:    certDiscovery,
		logger:           logger,
	}
}

// getRoutingAction: returns routing action from listener rule configuration
// action will only be one of forward, fixed response or redirect
func getRoutingAction(config *elbv2gw.ListenerRuleConfiguration) *elbv2gw.Action {
	if config != nil && config.Spec.Actions != nil {
		for _, action := range config.Spec.Actions {
			if action.Type == elbv2gw.ActionTypeForward || action.Type == elbv2gw.ActionTypeFixedResponse || action.Type == elbv2gw.ActionTypeRedirect {
				return &action
			}
		}
	}
	return nil
}
