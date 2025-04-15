package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"regexp"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strconv"
)

type buildTargetGroupOutput struct {
	targetGroupSpec elbv2model.TargetGroupSpec
	bindingSpec     elbv2model.TargetGroupBindingResourceSpec
}

const (
	tgAttrsProxyProtocolV2Enabled  = "proxy_protocol_v2.enabled"
	tgAttrsPreserveClientIPEnabled = "preserve_client_ip.enabled"
)

type targetGroupBuilder interface {
	buildTargetGroup(ctx context.Context,
		gw *gwv1.Gateway, lbConfig *elbv2gw.LoadBalancerConfiguration, targetGroupConfig *elbv2gw.TargetGroupConfiguration, routeDescriptor routeutils.Backend, backendDescriptor routeutils.Backend) (*elbv2model.TargetGroup, error)
}

type targetGroupBuilderImpl struct {
	loadBalancerType elbv2model.LoadBalancerType

	clusterName string
	vpcID       string

	tagHelper                tagHelper
	disableRestrictedSGRules bool

	defaultTargetType elbv2model.TargetType

	defaultProxyProtocolV2Enabled bool

	defaultHealthCheckProtocol elbv2model.Protocol

	defaultL7BackendProtocol        elbv2model.Protocol
	defaultL7BackendProtocolVersion elbv2model.ProtocolVersion

	defaultHealthCheckMatcherHTTPCode string
	defaultHealthCheckMatcherGRPCCode string

	defaultHealthCheckPathHTTP string
	defaultHealthCheckPathGRPC string

	defaultHealthCheckUnhealthyThresholdCount int32
	defaultHealthyThresholdCount              int32
	defaultHealthCheckTimeout                 int32
	defaultHealthCheckInterval                int32
}

func (builder *targetGroupBuilderImpl) buildTargetGroup(ctx context.Context, tgByResID *map[string]buildTargetGroupOutput,
	gw *gwv1.Gateway, targetGroupConfig *elbv2gw.TargetGroupConfiguration, lbConfig *elbv2gw.LoadBalancerConfiguration, lbIPType elbv2model.IPAddressType, routeDescriptor routeutils.RouteDescriptor, backend routeutils.Backend, backendSGIDToken core.StringToken) (buildTargetGroupOutput, error) {

	var targetGroupProps *elbv2gw.TargetGroupProps

	if targetGroupConfig != nil {
		routeNamespacedName := routeDescriptor.GetRouteNamespacedName()
		targetGroupProps = targetGroupConfig.GetTargetGroupConfigForRoute(routeNamespacedName.Name, routeNamespacedName.Namespace, routeDescriptor.GetRouteKind())
	}

	tgResID := builder.buildTargetGroupResourceID(k8s.NamespacedName(gw), k8s.NamespacedName(backend.Service), routeDescriptor.GetRouteNamespacedName(), backend.ServicePort.TargetPort)
	if tg, exists := (*tgByResID)[tgResID]; exists {
		return tg, nil
	}

	tgSpec, err := builder.buildTargetGroupSpec(gw, routeDescriptor, lbConfig, lbIPType, backend, targetGroupProps)
	if err != nil {
		return buildTargetGroupOutput{}, err
	}
	nodeSelector := builder.buildTargetGroupBindingNodeSelector(targetGroupProps, tgSpec.TargetType)
	bindingSpec := builder.buildTargetGroupBindingSpec(lbConfig, tgSpec, nodeSelector, backend, backendSGIDToken)

	output := buildTargetGroupOutput{
		targetGroupSpec: tgSpec,
		bindingSpec:     bindingSpec,
	}

	(*tgByResID)[tgResID] = output
	return output, nil
}

func (builder *targetGroupBuilderImpl) buildTargetGroupBindingSpec(lbConfig *elbv2gw.LoadBalancerConfiguration, tgSpec *elbv2model.TargetGroupSpec, nodeSelector *metav1.LabelSelector, backend routeutils.Backend, backendSGIDToken core.StringToken) elbv2model.TargetGroupBindingResourceSpec {
	targetType := elbv2api.TargetType(tgSpec.TargetType)
	targetPort := backend.ServicePort.TargetPort
	if targetType == elbv2api.TargetTypeInstance {
		targetPort = intstr.FromInt32(backend.ServicePort.NodePort)
	}
	tgbNetworking := builder.buildTargetGroupBindingNetworking(targetPort, *tgSpec.HealthCheckConfig.Port, *backend.ServicePort, backendSGIDToken)

	multiClusterEnabled := builder.buildTargetGroupBindingMultiClusterFlag(lbConfig)

	return elbv2model.TargetGroupBindingResourceSpec{
		Template: elbv2model.TargetGroupBindingTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: backend.Service.Namespace,
				Name:      tgSpec.Name,
			},
			Spec: elbv2model.TargetGroupBindingSpec{
				TargetGroupARN: nil, // This should get filled in later!
				TargetType:     &targetType,
				ServiceRef: elbv2api.ServiceReference{
					Name: backend.Service.Name,
					Port: backend.ServicePort.TargetPort,
				},
				Networking:              tgbNetworking,
				NodeSelector:            nodeSelector,
				IPAddressType:           elbv2api.TargetGroupIPAddressType(tgSpec.IPAddressType),
				VpcID:                   builder.vpcID,
				MultiClusterTargetGroup: multiClusterEnabled,
			},
		},
	}
}

func (builder *targetGroupBuilderImpl) buildTargetGroupBindingNetworking(targetPort intstr.IntOrString, healthCheckPort intstr.IntOrString, port corev1.ServicePort, backendSGIDToken core.StringToken) *elbv2model.TargetGroupBindingNetworking {
	if backendSGIDToken == nil {
		return nil
	}
	protocolTCP := elbv2api.NetworkingProtocolTCP
	protocolUDP := elbv2api.NetworkingProtocolUDP
	if builder.disableRestrictedSGRules {
		ports := []elbv2api.NetworkingPort{
			{
				Protocol: &protocolTCP,
				Port:     nil,
			},
		}

		if port.Protocol == corev1.ProtocolUDP {
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolUDP,
				Port:     nil,
			})
		}

		return &elbv2model.TargetGroupBindingNetworking{

			Ingress: []elbv2model.NetworkingIngressRule{
				{
					From: []elbv2model.NetworkingPeer{
						{
							SecurityGroup: &elbv2model.SecurityGroup{
								GroupID: backendSGIDToken,
							},
						},
					},
					Ports: ports,
				},
			},
		}
	}
	var networkingPorts []elbv2api.NetworkingPort
	var networkingRules []elbv2model.NetworkingIngressRule
	networkingPorts = append(networkingPorts, elbv2api.NetworkingPort{
		Protocol: &protocolTCP,
		Port:     &targetPort,
	})
	if healthCheckPort.String() != shared_constants.HealthCheckPortTrafficPort {
		networkingPorts = append(networkingPorts, elbv2api.NetworkingPort{
			Protocol: &protocolTCP,
			Port:     &healthCheckPort,
		})
	}
	for _, port := range networkingPorts {
		networkingRules = append(networkingRules, elbv2model.NetworkingIngressRule{
			From: []elbv2model.NetworkingPeer{
				{
					SecurityGroup: &elbv2model.SecurityGroup{
						GroupID: backendSGIDToken,
					},
				},
			},
			Ports: []elbv2api.NetworkingPort{port},
		})
	}
	return &elbv2model.TargetGroupBindingNetworking{
		Ingress: networkingRules,
	}
}

func (builder *targetGroupBuilderImpl) buildTargetGroupSpec(gw *gwv1.Gateway, route routeutils.RouteDescriptor, lbConfig *elbv2gw.LoadBalancerConfiguration, lbIPType elbv2model.IPAddressType, backend routeutils.Backend, targetGroupProps *elbv2gw.TargetGroupProps) (elbv2model.TargetGroupSpec, error) {
	targetType := builder.buildTargetGroupTargetType(targetGroupProps)
	tgProtocol, err := builder.buildTargetGroupProtocol(targetGroupProps)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgProtocolVersion := builder.buildTargetGroupProtocolVersion(targetGroupProps)

	healthCheckConfig, err := builder.buildTargetGroupHealthCheckConfig(targetGroupProps, tgProtocol, tgProtocolVersion, targetType, backend)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgAttributesMap := builder.buildTargetGroupAttributes(targetGroupProps)
	ipAddressType, err := builder.buildTargetGroupIPAddressType(backend.Service, lbIPType)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}

	tags, err := builder.tagHelper.getGatewayTags(lbConfig)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgPort := builder.buildTargetGroupPort(targetType, *backend.ServicePort)
	// TODO - backend.ServicePort.TargetPort might not be correct.
	name := builder.buildTargetGroupName(targetGroupProps, k8s.NamespacedName(gw), route.GetRouteNamespacedName(), k8s.NamespacedName(backend.Service), backend.ServicePort.TargetPort, tgPort, targetType, tgProtocol, tgProtocolVersion)
	return elbv2model.TargetGroupSpec{
		Name:                  name,
		TargetType:            targetType,
		Port:                  awssdk.Int32(tgPort),
		Protocol:              tgProtocol,
		ProtocolVersion:       tgProtocolVersion,
		IPAddressType:         ipAddressType,
		HealthCheckConfig:     &healthCheckConfig,
		TargetGroupAttributes: builder.convertMapToAttributes(tgAttributesMap),
		Tags:                  tags,
	}, nil
}

var invalidTargetGroupNamePattern = regexp.MustCompile("[[:^alnum:]]")

// buildTargetGroupName will calculate the targetGroup's name.
func (builder *targetGroupBuilderImpl) buildTargetGroupName(targetGroupProps *elbv2gw.TargetGroupProps,
	gwKey types.NamespacedName, routeKey types.NamespacedName, svcKey types.NamespacedName, port intstr.IntOrString, tgPort int32,
	targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol, tgProtocolVersion *elbv2model.ProtocolVersion) string {

	if targetGroupProps.TargetGroupName != "" {
		return targetGroupProps.TargetGroupName
	}

	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(builder.clusterName))
	_, _ = uuidHash.Write([]byte(gwKey.Namespace))
	_, _ = uuidHash.Write([]byte(gwKey.Name))
	_, _ = uuidHash.Write([]byte(routeKey.Namespace))
	_, _ = uuidHash.Write([]byte(routeKey.Name))
	_, _ = uuidHash.Write([]byte(svcKey.Namespace))
	_, _ = uuidHash.Write([]byte(svcKey.Name))
	_, _ = uuidHash.Write([]byte(port.String()))
	_, _ = uuidHash.Write([]byte(strconv.Itoa(int(tgPort))))
	_, _ = uuidHash.Write([]byte(targetType))
	_, _ = uuidHash.Write([]byte(tgProtocol))
	if tgProtocolVersion != nil {
		_, _ = uuidHash.Write([]byte(*tgProtocolVersion))
	}
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidTargetGroupNamePattern.ReplaceAllString(routeKey.Namespace, "")
	sanitizedName := invalidTargetGroupNamePattern.ReplaceAllString(routeKey.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (builder *targetGroupBuilderImpl) buildTargetGroupTargetType(targetGroupProps *elbv2gw.TargetGroupProps) elbv2model.TargetType {
	if targetGroupProps == nil || targetGroupProps.TargetType == nil {
		return builder.defaultTargetType
	}

	return elbv2model.TargetType(*targetGroupProps.TargetType)
}

func (builder *targetGroupBuilderImpl) buildTargetGroupIPAddressType(svc *corev1.Service, loadBalancerIPAddressType elbv2model.IPAddressType) (elbv2model.TargetGroupIPAddressType, error) {
	var ipv6Configured bool
	for _, ipFamily := range svc.Spec.IPFamilies {
		if ipFamily == corev1.IPv6Protocol {
			ipv6Configured = true
			break
		}
	}
	if ipv6Configured {
		if !isIPv6Supported(loadBalancerIPAddressType) {
			return "", errors.New("unsupported IPv6 configuration, lb not dual-stack")
		}
		return elbv2model.TargetGroupIPAddressTypeIPv6, nil
	}
	return elbv2model.TargetGroupIPAddressTypeIPv4, nil
}

// buildTargetGroupPort constructs the TargetGroup's port.
// Note: TargetGroup's port is not in the data path as we always register targets with port specified.
// so this settings don't really matter to our controller, and we do our best to use the most appropriate port as targetGroup's port to avoid UX confusing.
func (builder *targetGroupBuilderImpl) buildTargetGroupPort(targetType elbv2model.TargetType, svcPort corev1.ServicePort) int32 {
	if targetType == elbv2model.TargetTypeInstance {
		// Maybe an error? Because the service has no node port, instance type targets don't work.
		if svcPort.NodePort == 0 {
			return 1
		}
		return svcPort.NodePort
	}
	if svcPort.TargetPort.Type == intstr.Int {
		return int32(svcPort.TargetPort.IntValue())
	}

	// when a literal targetPort is used, we just use a fixed 1 here as this setting is not in the data path.
	// also, under extreme edge case, it can actually be different ports for different pods.
	return 1
}

func (builder *targetGroupBuilderImpl) buildTargetGroupProtocol(targetGroupProps *elbv2gw.TargetGroupProps) (elbv2model.Protocol, error) {
	if builder.loadBalancerType == elbv2model.LoadBalancerTypeApplication {
		return builder.buildL7TargetGroupProtocol(targetGroupProps)
	}

	return builder.buildL4TargetGroupProtocol(targetGroupProps)
}

func (builder *targetGroupBuilderImpl) buildL7TargetGroupProtocol(targetGroupProps *elbv2gw.TargetGroupProps) (elbv2model.Protocol, error) {
	if targetGroupProps.Protocol == nil {
		return builder.defaultL7BackendProtocol, nil
	}
	switch string(*targetGroupProps.Protocol) {
	case string(elbv2model.ProtocolHTTP):
		return elbv2model.ProtocolHTTP, nil
	case string(elbv2model.ProtocolHTTPS):
		return elbv2model.ProtocolHTTPS, nil
	default:
		return "", errors.Errorf("backend protocol must be within [%v, %v]: %v", elbv2model.ProtocolHTTP, elbv2model.ProtocolHTTPS, *targetGroupProps.Protocol)
	}
}

func (builder *targetGroupBuilderImpl) buildL4TargetGroupProtocol(targetGroupProps *elbv2gw.TargetGroupProps) (elbv2model.Protocol, error) {
	if targetGroupProps.Protocol == nil {
		// infer this somehow!?
		// use the backend config to get the protocol type.
		return elbv2model.ProtocolTCP, nil
	}

	switch string(*targetGroupProps.Protocol) {
	case string(elbv2model.ProtocolTCP):
		return elbv2model.ProtocolTCP, nil
	case string(elbv2model.ProtocolTLS):
		return elbv2model.ProtocolTLS, nil
	case string(elbv2model.ProtocolUDP):
		return elbv2model.ProtocolUDP, nil
	case string(elbv2model.ProtocolTCP_UDP):
		return elbv2model.ProtocolTCP_UDP, nil
	default:
		return "", errors.Errorf("backend protocol must be within [%v, %v, %v, %v]: %v", elbv2model.ProtocolTCP, elbv2model.ProtocolUDP, elbv2model.ProtocolTCP_UDP, elbv2model.ProtocolTLS, *targetGroupProps.Protocol)
	}
}

func (builder *targetGroupBuilderImpl) buildTargetGroupProtocolVersion(targetGroupProps *elbv2gw.TargetGroupProps) *elbv2model.ProtocolVersion {
	// NLB doesn't support protocol version
	if builder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
		return nil
	}
	if targetGroupProps.ProtocolVersion != nil {
		pv := elbv2model.ProtocolVersion(*targetGroupProps.ProtocolVersion)
		return &pv
	}
	return &builder.defaultL7BackendProtocolVersion
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckConfig(targetGroupProps *elbv2gw.TargetGroupProps, tgProtocol elbv2model.Protocol, tgProtocolVersion *elbv2model.ProtocolVersion, targetType elbv2model.TargetType, backend routeutils.Backend) (elbv2model.TargetGroupHealthCheckConfig, error) {
	healthCheckPort, err := builder.buildTargetGroupHealthCheckPort(targetGroupProps, targetType, backend)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckProtocol := builder.buildTargetGroupHealthCheckProtocol(targetGroupProps)
	healthCheckPath := builder.buildTargetGroupHealthCheckPath(targetGroupProps, tgProtocolVersion, healthCheckProtocol)
	healthCheckMatcher := builder.buildTargetGroupHealthCheckMatcher(targetGroupProps)
	healthCheckIntervalSeconds := builder.buildTargetGroupHealthCheckIntervalSeconds(targetGroupProps)
	healthCheckTimeoutSeconds := builder.buildTargetGroupHealthCheckTimeoutSeconds(targetGroupProps)
	healthCheckHealthyThresholdCount := builder.buildTargetGroupHealthCheckHealthyThresholdCount(targetGroupProps)
	healthCheckUnhealthyThresholdCount := builder.buildTargetGroupHealthCheckUnhealthyThresholdCount(targetGroupProps)
	hcConfig := elbv2model.TargetGroupHealthCheckConfig{
		Port:                    &healthCheckPort,
		Protocol:                healthCheckProtocol,
		Path:                    healthCheckPath,
		Matcher:                 &healthCheckMatcher,
		IntervalSeconds:         awssdk.Int32(int32(healthCheckIntervalSeconds)),
		TimeoutSeconds:          awssdk.Int32(int32(healthCheckTimeoutSeconds)),
		HealthyThresholdCount:   awssdk.Int32(int32(healthCheckHealthyThresholdCount)),
		UnhealthyThresholdCount: awssdk.Int32(healthCheckUnhealthyThresholdCount),
	}

	return hcConfig, nil
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckPort(targetGroupProps *elbv2gw.TargetGroupProps, targetType elbv2model.TargetType, backend routeutils.Backend) (intstr.IntOrString, error) {

	if targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckPort == nil || *targetGroupProps.HealthCheckConfig.HealthCheckPort == shared_constants.HealthCheckPortTrafficPort {
		return intstr.FromString(shared_constants.HealthCheckPortTrafficPort), nil
	}

	healthCheckPort := intstr.Parse(*targetGroupProps.HealthCheckConfig.HealthCheckPort)
	if healthCheckPort.Type == intstr.Int {
		return healthCheckPort, nil
	}

	/* TODO - Zac revisit this? */
	svcPort, err := k8s.LookupServicePort(backend.Service, healthCheckPort)
	if err != nil {
		return intstr.IntOrString{}, errors.Wrap(err, "failed to resolve healthCheckPort")
	}
	if targetType == elbv2model.TargetTypeInstance {
		return intstr.FromInt(int(svcPort.NodePort)), nil
	}
	if svcPort.TargetPort.Type == intstr.Int {
		return svcPort.TargetPort, nil
	}
	return intstr.IntOrString{}, errors.New("cannot use named healthCheckPort for IP TargetType when service's targetPort is a named port")
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckProtocol(targetGroupProps *elbv2gw.TargetGroupProps) elbv2model.Protocol {
	if targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckProtocol == nil {
		return builder.defaultHealthCheckProtocol
	}

	switch *targetGroupProps.HealthCheckConfig.HealthCheckProtocol {
	case elbv2gw.TargetGroupHealthCheckProtocolTCP:
		return elbv2model.ProtocolTCP
	case elbv2gw.TargetGroupHealthCheckProtocolHTTP:
		return elbv2model.ProtocolHTTP
	case elbv2gw.TargetGroupHealthCheckProtocolHTTPS:
		return elbv2model.ProtocolHTTPS
	default:
		return ""
	}
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckPath(targetGroupProps *elbv2gw.TargetGroupProps, tgProtocolVersion *elbv2model.ProtocolVersion, hcProtocol elbv2model.Protocol) *string {
	if hcProtocol == elbv2model.ProtocolTCP {
		return nil
	}

	if targetGroupProps.HealthCheckConfig.HealthCheckPath != nil {
		return targetGroupProps.HealthCheckConfig.HealthCheckPath
	}

	if tgProtocolVersion != nil && *tgProtocolVersion == elbv2model.ProtocolVersionGRPC {
		return &builder.defaultHealthCheckPathGRPC
	}

	return &builder.defaultHealthCheckPathHTTP
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckMatcher(targetGroupProps *elbv2gw.TargetGroupProps) elbv2model.HealthCheckMatcher {
	if targetGroupProps.ProtocolVersion != nil && string(*targetGroupProps.ProtocolVersion) == string(elbv2model.ProtocolVersionGRPC) {
		matcher := builder.defaultHealthCheckMatcherGRPCCode
		if targetGroupProps.ProtocolVersion != nil && targetGroupProps.HealthCheckConfig != nil && targetGroupProps.HealthCheckConfig.Matcher != nil && targetGroupProps.HealthCheckConfig.Matcher.GRPCCode != nil {
			matcher = *targetGroupProps.HealthCheckConfig.Matcher.GRPCCode
		}
		return elbv2model.HealthCheckMatcher{
			GRPCCode: &matcher,
		}
	}
	matcher := builder.defaultHealthCheckMatcherHTTPCode
	if targetGroupProps.ProtocolVersion != nil && targetGroupProps.HealthCheckConfig != nil && targetGroupProps.HealthCheckConfig.Matcher != nil && targetGroupProps.HealthCheckConfig.Matcher.HTTPCode != nil {
		matcher = *targetGroupProps.HealthCheckConfig.Matcher.HTTPCode
	}
	return elbv2model.HealthCheckMatcher{
		HTTPCode: &matcher,
	}
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckIntervalSeconds(targetGroupProps *elbv2gw.TargetGroupProps) int32 {
	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckInterval == nil {
		return builder.defaultHealthCheckInterval
	}
	return *targetGroupProps.HealthCheckConfig.HealthCheckInterval
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckTimeoutSeconds(targetGroupProps *elbv2gw.TargetGroupProps) int32 {
	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckTimeout == nil {
		return builder.defaultHealthCheckTimeout
	}
	return *targetGroupProps.HealthCheckConfig.HealthCheckTimeout
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckHealthyThresholdCount(targetGroupProps *elbv2gw.TargetGroupProps) int32 {
	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthyThresholdCount == nil {
		return builder.defaultHealthyThresholdCount
	}
	return *targetGroupProps.HealthCheckConfig.HealthyThresholdCount
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckUnhealthyThresholdCount(targetGroupProps *elbv2gw.TargetGroupProps) int32 {
	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.UnhealthyThresholdCount == nil {
		return builder.defaultHealthCheckUnhealthyThresholdCount
	}
	return *targetGroupProps.HealthCheckConfig.UnhealthyThresholdCount
}

func (builder *targetGroupBuilderImpl) buildTargetGroupAttributes(targetGroupProps *elbv2gw.TargetGroupProps) map[string]string {
	attributeMap := make(map[string]string)

	for _, attr := range targetGroupProps.TargetGroupAttributes {
		attributeMap[attr.Key] = attr.Value
	}

	if builder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
		builder.buildL4TargetGroupAttributes(&attributeMap, targetGroupProps)
	}

	return attributeMap
}

func (builder *targetGroupBuilderImpl) convertMapToAttributes(attributeMap map[string]string) []elbv2model.TargetGroupAttribute {
	convertedAttributes := make([]elbv2model.TargetGroupAttribute, 0)
	for key, value := range attributeMap {
		convertedAttributes = append(convertedAttributes, elbv2model.TargetGroupAttribute{
			Key:   key,
			Value: value,
		})
	}
	return convertedAttributes
}

func (builder *targetGroupBuilderImpl) buildL4TargetGroupAttributes(attributeMap *map[string]string, targetGroupProps *elbv2gw.TargetGroupProps) {
	if _, ok := (*attributeMap)[tgAttrsProxyProtocolV2Enabled]; !ok {
		(*attributeMap)[tgAttrsProxyProtocolV2Enabled] = strconv.FormatBool(builder.defaultProxyProtocolV2Enabled)
	}

	if targetGroupProps.EnableProxyProtocolV2 != nil {
		(*attributeMap)[tgAttrsProxyProtocolV2Enabled] = strconv.FormatBool(*targetGroupProps.EnableProxyProtocolV2)
	}

	// TODO -- buildPreserveClientIPFlag
}

func (builder *targetGroupBuilderImpl) buildTargetGroupResourceID(gwKey types.NamespacedName, svcKey types.NamespacedName, routeKey types.NamespacedName, port intstr.IntOrString) string {
	return fmt.Sprintf("%s/%s:%s-%s:%s-%s:%s", gwKey.Namespace, gwKey.Name, routeKey.Namespace, routeKey.Name, svcKey.Namespace, svcKey.Name, port.String())
}

func (builder *targetGroupBuilderImpl) buildTargetGroupBindingNodeSelector(tgProps *elbv2gw.TargetGroupProps, targetType elbv2model.TargetType) *metav1.LabelSelector {
	if targetType != elbv2model.TargetTypeInstance {
		return nil
	}
	if tgProps == nil {
		return nil
	}
	return tgProps.NodeSelector
}

func (builder *targetGroupBuilderImpl) buildTargetGroupBindingMultiClusterFlag(lbConfig *elbv2gw.LoadBalancerConfiguration) bool {
	if lbConfig == nil {
		return false
	}
	return lbConfig.Spec.EnableMultiCluster
}
