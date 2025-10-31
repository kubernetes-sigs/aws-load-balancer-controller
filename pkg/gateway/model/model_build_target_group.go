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
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strconv"
)

type buildTargetGroupOutput struct {
	targetGroupSpec elbv2model.TargetGroupSpec
	bindingSpec     elbv2modelk8s.TargetGroupBindingResourceSpec
}

type targetGroupBuilder interface {
	buildTargetGroup(stack core.Stack,
		gw *gwv1.Gateway, lbIPType elbv2model.IPAddressType, routeDescriptor routeutils.RouteDescriptor, backend routeutils.Backend) (core.StringToken, error)
}

type targetGroupBuilderImpl struct {
	loadBalancerType elbv2model.LoadBalancerType

	clusterName string
	vpcID       string

	tagHelper               tagHelper
	tgByResID               map[string]*elbv2model.TargetGroup
	tgPropertiesConstructor gateway.TargetGroupConfigConstructor

	tgbNetworkBuilder          targetGroupBindingNetworkBuilder
	targetGroupNameToArnMapper shared_utils.TargetGroupARNMapper

	defaultTargetType elbv2model.TargetType

	defaultHealthCheckMatcherHTTPCode string
	defaultHealthCheckMatcherGRPCCode string

	defaultHealthCheckPathHTTP string
	defaultHealthCheckPathGRPC string

	defaultHealthCheckUnhealthyThresholdCount int32
	defaultHealthyThresholdCount              int32
	defaultHealthCheckTimeout                 int32
	defaultHealthCheckInterval                int32

	// Default health check settings for NLB instance mode with spec.ExternalTrafficPolicy set to Local
	defaultHealthCheckProtocolForInstanceModeLocal           elbv2model.Protocol
	defaultHealthCheckPathForInstanceModeLocal               string
	defaultHealthCheckIntervalForInstanceModeLocal           int32
	defaultHealthCheckTimeoutForInstanceModeLocal            int32
	defaultHealthCheckHealthyThresholdForInstanceModeLocal   int32
	defaultHealthCheckUnhealthyThresholdForInstanceModeLocal int32
}

func newTargetGroupBuilder(clusterName string, vpcId string, tagHelper tagHelper, loadBalancerType elbv2model.LoadBalancerType, tgbNetworkBuilder targetGroupBindingNetworkBuilder, tgPropertiesConstructor gateway.TargetGroupConfigConstructor, defaultTargetType string, targetGroupNameToArnMapper shared_utils.TargetGroupARNMapper) targetGroupBuilder {
	return &targetGroupBuilderImpl{
		loadBalancerType:                          loadBalancerType,
		clusterName:                               clusterName,
		vpcID:                                     vpcId,
		tgbNetworkBuilder:                         tgbNetworkBuilder,
		tgPropertiesConstructor:                   tgPropertiesConstructor,
		targetGroupNameToArnMapper:                targetGroupNameToArnMapper,
		tgByResID:                                 make(map[string]*elbv2model.TargetGroup),
		tagHelper:                                 tagHelper,
		defaultTargetType:                         elbv2model.TargetType(defaultTargetType),
		defaultHealthCheckMatcherHTTPCode:         "200-399",
		defaultHealthCheckMatcherGRPCCode:         "12",
		defaultHealthCheckPathHTTP:                "/",
		defaultHealthCheckPathGRPC:                "/AWS.ALB/healthcheck",
		defaultHealthCheckUnhealthyThresholdCount: 3,
		defaultHealthyThresholdCount:              3,
		defaultHealthCheckTimeout:                 5,
		defaultHealthCheckInterval:                15,

		defaultHealthCheckProtocolForInstanceModeLocal:           elbv2model.ProtocolHTTP,
		defaultHealthCheckPathForInstanceModeLocal:               "/healthz",
		defaultHealthCheckIntervalForInstanceModeLocal:           10,
		defaultHealthCheckTimeoutForInstanceModeLocal:            6,
		defaultHealthCheckHealthyThresholdForInstanceModeLocal:   2,
		defaultHealthCheckUnhealthyThresholdForInstanceModeLocal: 2,
	}
}

func (builder *targetGroupBuilderImpl) buildTargetGroup(stack core.Stack,
	gw *gwv1.Gateway, lbIPType elbv2model.IPAddressType, routeDescriptor routeutils.RouteDescriptor, backend routeutils.Backend) (core.StringToken, error) {

	if backend.ServiceBackend != nil {
		tg, err := builder.buildTargetGroupFromService(stack, gw, lbIPType, routeDescriptor, *backend.ServiceBackend)
		if err != nil {
			return nil, err
		}
		return tg.TargetGroupARN(), nil
	}

	if backend.LiteralTargetGroup != nil {
		arn, err := builder.buildTargetGroupFromStaticName(*backend.LiteralTargetGroup)
		return arn, err
	}

	return nil, errors.New("Unknown backend type")
}

func (builder *targetGroupBuilderImpl) buildTargetGroupFromService(stack core.Stack,
	gw *gwv1.Gateway, lbIPType elbv2model.IPAddressType, routeDescriptor routeutils.RouteDescriptor, backendConfig routeutils.ServiceBackendConfig) (*elbv2model.TargetGroup, error) {
	targetGroupProps := backendConfig.ELBV2TargetGroupProps
	tgResID := builder.buildTargetGroupResourceID(k8s.NamespacedName(gw), k8s.NamespacedName(backendConfig.Service), routeDescriptor.GetRouteNamespacedName(), routeDescriptor.GetRouteKind(), backendConfig.ServicePort.TargetPort)
	if tg, exists := builder.tgByResID[tgResID]; exists {
		return tg, nil
	}

	tgSpec, err := builder.buildTargetGroupSpec(gw, routeDescriptor, lbIPType, backendConfig, targetGroupProps)
	if err != nil {
		return nil, err
	}
	nodeSelector := builder.buildTargetGroupBindingNodeSelector(targetGroupProps, tgSpec.TargetType)
	bindingSpec, err := builder.buildTargetGroupBindingSpec(gw, targetGroupProps, tgSpec, nodeSelector, backendConfig)

	if err != nil {
		return nil, err
	}

	tgOut := buildTargetGroupOutput{
		targetGroupSpec: tgSpec,
		bindingSpec:     bindingSpec,
	}
	tg := elbv2model.NewTargetGroup(stack, tgResID, tgOut.targetGroupSpec)
	tgOut.bindingSpec.Template.Spec.TargetGroupARN = tg.TargetGroupARN()
	elbv2modelk8s.NewTargetGroupBindingResource(stack, tg.ID(), tgOut.bindingSpec)
	builder.tgByResID[tgResID] = tg
	return tg, nil
}

func (builder *targetGroupBuilderImpl) buildTargetGroupFromStaticName(cfg routeutils.LiteralTargetGroupConfig) (core.StringToken, error) {

	tgArn, err := builder.targetGroupNameToArnMapper.GetArnByName(context.Background(), cfg.Name)

	if err != nil {
		return nil, err
	}

	return core.LiteralStringToken(tgArn), nil
}

func (builder *targetGroupBuilderImpl) buildTargetGroupBindingSpec(gw *gwv1.Gateway, tgProps *elbv2gw.TargetGroupProps, tgSpec elbv2model.TargetGroupSpec, nodeSelector *metav1.LabelSelector, backendConfig routeutils.ServiceBackendConfig) (elbv2modelk8s.TargetGroupBindingResourceSpec, error) {
	targetType := elbv2api.TargetType(tgSpec.TargetType)
	targetPort := backendConfig.ServicePort.TargetPort
	if targetType == elbv2api.TargetTypeInstance {
		targetPort = intstr.FromInt32(backendConfig.ServicePort.NodePort)
	}
	tgbNetworking, err := builder.tgbNetworkBuilder.buildTargetGroupBindingNetworking(tgSpec, targetPort)
	if err != nil {
		return elbv2modelk8s.TargetGroupBindingResourceSpec{}, err
	}

	multiClusterEnabled := builder.buildTargetGroupBindingMultiClusterFlag(tgProps)

	annotations := make(map[string]string)
	labels := make(map[string]string)

	if gw != nil && gw.Spec.Infrastructure != nil {
		if gw.Spec.Infrastructure.Annotations != nil {
			for k, v := range gw.Spec.Infrastructure.Annotations {
				annotations[string(k)] = string(v)
			}
		}

		if gw.Spec.Infrastructure.Labels != nil {
			for k, v := range gw.Spec.Infrastructure.Labels {
				labels[string(k)] = string(v)
			}
		}
	}

	return elbv2modelk8s.TargetGroupBindingResourceSpec{
		Template: elbv2modelk8s.TargetGroupBindingTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   backendConfig.Service.Namespace,
				Name:        tgSpec.Name,
				Annotations: annotations,
				Labels:      labels,
			},
			Spec: elbv2modelk8s.TargetGroupBindingSpec{
				TargetGroupARN: nil, // This should get filled in later!
				TargetType:     &targetType,
				ServiceRef: elbv2api.ServiceReference{
					Name: backendConfig.Service.Name,
					Port: intstr.FromInt32(backendConfig.ServicePort.Port),
				},
				Networking:              tgbNetworking,
				NodeSelector:            nodeSelector,
				IPAddressType:           elbv2api.TargetGroupIPAddressType(tgSpec.IPAddressType),
				VpcID:                   builder.vpcID,
				MultiClusterTargetGroup: multiClusterEnabled,
				TargetGroupProtocol:     &tgSpec.Protocol,
			},
		},
	}, nil
}

func (builder *targetGroupBuilderImpl) buildTargetGroupSpec(gw *gwv1.Gateway, route routeutils.RouteDescriptor, lbIPType elbv2model.IPAddressType, backendConfig routeutils.ServiceBackendConfig, targetGroupProps *elbv2gw.TargetGroupProps) (elbv2model.TargetGroupSpec, error) {
	targetType := builder.buildTargetGroupTargetType(targetGroupProps)
	tgProtocol, err := builder.buildTargetGroupProtocol(targetGroupProps, route)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgProtocolVersion := builder.buildTargetGroupProtocolVersion(targetGroupProps, route)

	healthCheckConfig, err := builder.buildTargetGroupHealthCheckConfig(targetGroupProps, tgProtocol, tgProtocolVersion, targetType, backendConfig)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgAttributesMap := builder.buildTargetGroupAttributes(targetGroupProps)
	ipAddressType, err := builder.buildTargetGroupIPAddressType(backendConfig.Service, lbIPType)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}

	tags, err := builder.tagHelper.getTargetGroupTags(targetGroupProps)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgPort := builder.buildTargetGroupPort(targetType, *backendConfig.ServicePort)
	name := builder.buildTargetGroupName(targetGroupProps, k8s.NamespacedName(gw), route.GetRouteNamespacedName(), route.GetRouteKind(), k8s.NamespacedName(backendConfig.Service), tgPort, targetType, tgProtocol, tgProtocolVersion)

	if tgPort == 0 {
		if targetType == elbv2model.TargetTypeIP {
			return elbv2model.TargetGroupSpec{}, errors.Errorf("TargetGroup port is empty. Are you using the correct service type?")
		}
		return elbv2model.TargetGroupSpec{}, errors.Errorf("TargetGroup port is empty. When using Instance targets, your service must be of type 'NodePort' or 'LoadBalancer'")
	}
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
	gwKey types.NamespacedName, routeKey types.NamespacedName, routeKind routeutils.RouteKind, svcKey types.NamespacedName, tgPort int32,
	targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol, tgProtocolVersion *elbv2model.ProtocolVersion) string {

	if targetGroupProps != nil && targetGroupProps.TargetGroupName != nil {
		return *targetGroupProps.TargetGroupName
	}

	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(builder.clusterName))
	_, _ = uuidHash.Write([]byte(gwKey.Namespace))
	_, _ = uuidHash.Write([]byte(gwKey.Name))
	_, _ = uuidHash.Write([]byte(routeKey.Namespace))
	_, _ = uuidHash.Write([]byte(routeKey.Name))
	_, _ = uuidHash.Write([]byte(routeKind))
	_, _ = uuidHash.Write([]byte(svcKey.Namespace))
	_, _ = uuidHash.Write([]byte(svcKey.Name))
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
// so these settings don't really matter to our controller,
// and we do our best to use the most appropriate port as targetGroup's port to avoid UX confusing.
func (builder *targetGroupBuilderImpl) buildTargetGroupPort(targetType elbv2model.TargetType, svcPort corev1.ServicePort) int32 {
	if targetType == elbv2model.TargetTypeInstance {
		return svcPort.NodePort
	}
	if svcPort.TargetPort.Type == intstr.Int {
		return int32(svcPort.TargetPort.IntValue())
	}
	// If all else fails, just return 1 as alluded to above, this setting doesn't really matter.
	return 1
}

func (builder *targetGroupBuilderImpl) buildTargetGroupProtocol(targetGroupProps *elbv2gw.TargetGroupProps, route routeutils.RouteDescriptor) (elbv2model.Protocol, error) {
	// TODO - Not convinced that this is good, maybe auto detect certs == HTTPS / TLS.
	if builder.loadBalancerType == elbv2model.LoadBalancerTypeApplication {
		return builder.buildL7TargetGroupProtocol(targetGroupProps, route)
	}

	return builder.buildL4TargetGroupProtocol(targetGroupProps, route)
}

func (builder *targetGroupBuilderImpl) buildL7TargetGroupProtocol(targetGroupProps *elbv2gw.TargetGroupProps, route routeutils.RouteDescriptor) (elbv2model.Protocol, error) {
	if targetGroupProps == nil || targetGroupProps.Protocol == nil {
		return builder.inferTargetGroupProtocolFromRoute(route), nil
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

func (builder *targetGroupBuilderImpl) buildL4TargetGroupProtocol(targetGroupProps *elbv2gw.TargetGroupProps, route routeutils.RouteDescriptor) (elbv2model.Protocol, error) {
	if targetGroupProps == nil || targetGroupProps.Protocol == nil {
		return builder.inferTargetGroupProtocolFromRoute(route), nil
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

func (builder *targetGroupBuilderImpl) inferTargetGroupProtocolFromRoute(route routeutils.RouteDescriptor) elbv2model.Protocol {
	switch route.GetRouteKind() {
	case routeutils.TCPRouteKind:
		return elbv2model.ProtocolTCP
	case routeutils.UDPRouteKind:
		return elbv2model.ProtocolUDP
	case routeutils.HTTPRouteKind:
		return elbv2model.ProtocolHTTP
	case routeutils.GRPCRouteKind:
		return elbv2model.ProtocolHTTP
	case routeutils.TLSRouteKind:
		if builder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
			return elbv2model.ProtocolTLS
		}
		return elbv2model.ProtocolHTTPS
	}
	// This should never happen.
	return elbv2model.ProtocolTCP
}

var (
	http1 = elbv2model.ProtocolVersionHTTP1
	grpc  = elbv2model.ProtocolVersionGRPC
)

func (builder *targetGroupBuilderImpl) buildTargetGroupProtocolVersion(targetGroupProps *elbv2gw.TargetGroupProps, route routeutils.RouteDescriptor) *elbv2model.ProtocolVersion {
	// NLB doesn't support protocol version
	if builder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
		return nil
	}
	if targetGroupProps != nil && targetGroupProps.ProtocolVersion != nil {
		// TODO - We can infer GRPC here from route
		pv := elbv2model.ProtocolVersion(*targetGroupProps.ProtocolVersion)
		return &pv
	}

	if route.GetRouteKind() == routeutils.GRPCRouteKind {
		return &grpc
	}

	return &http1
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckConfig(targetGroupProps *elbv2gw.TargetGroupProps, tgProtocol elbv2model.Protocol, tgProtocolVersion *elbv2model.ProtocolVersion, targetType elbv2model.TargetType, backendConfig routeutils.ServiceBackendConfig) (elbv2model.TargetGroupHealthCheckConfig, error) {
	// add ServiceExternalTrafficPolicyLocal support
	var isServiceExternalTrafficPolicyTypeLocal = false
	if targetType == elbv2model.TargetTypeInstance &&
		backendConfig.Service.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal &&
		builder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
		isServiceExternalTrafficPolicyTypeLocal = true
	}
	healthCheckPort, err := builder.buildTargetGroupHealthCheckPort(targetGroupProps, targetType, backendConfig.Service, isServiceExternalTrafficPolicyTypeLocal)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckProtocol := builder.buildTargetGroupHealthCheckProtocol(targetGroupProps, tgProtocol, isServiceExternalTrafficPolicyTypeLocal)                     //
	healthCheckPath := builder.buildTargetGroupHealthCheckPath(targetGroupProps, tgProtocolVersion, healthCheckProtocol, isServiceExternalTrafficPolicyTypeLocal) //

	healthCheckMatcher := builder.buildTargetGroupHealthCheckMatcher(targetGroupProps, tgProtocolVersion, healthCheckProtocol)                                  //
	healthCheckIntervalSeconds := builder.buildTargetGroupHealthCheckIntervalSeconds(targetGroupProps, isServiceExternalTrafficPolicyTypeLocal)                 //
	healthCheckTimeoutSeconds := builder.buildTargetGroupHealthCheckTimeoutSeconds(targetGroupProps, isServiceExternalTrafficPolicyTypeLocal)                   //
	healthCheckHealthyThresholdCount := builder.buildTargetGroupHealthCheckHealthyThresholdCount(targetGroupProps, isServiceExternalTrafficPolicyTypeLocal)     //
	healthCheckUnhealthyThresholdCount := builder.buildTargetGroupHealthCheckUnhealthyThresholdCount(targetGroupProps, isServiceExternalTrafficPolicyTypeLocal) //
	hcConfig := elbv2model.TargetGroupHealthCheckConfig{
		Port:                    &healthCheckPort,
		Protocol:                healthCheckProtocol,
		Path:                    healthCheckPath,
		Matcher:                 healthCheckMatcher,
		IntervalSeconds:         awssdk.Int32(healthCheckIntervalSeconds),
		TimeoutSeconds:          awssdk.Int32(healthCheckTimeoutSeconds),
		HealthyThresholdCount:   awssdk.Int32(healthCheckHealthyThresholdCount),
		UnhealthyThresholdCount: awssdk.Int32(healthCheckUnhealthyThresholdCount),
	}

	return hcConfig, nil
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckPort(targetGroupProps *elbv2gw.TargetGroupProps, targetType elbv2model.TargetType, svc *corev1.Service, isServiceExternalTrafficPolicyTypeLocal bool) (intstr.IntOrString, error) {

	portConfigNotExist := targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckPort == nil

	if portConfigNotExist && isServiceExternalTrafficPolicyTypeLocal {
		return intstr.FromInt32(svc.Spec.HealthCheckNodePort), nil
	}

	if portConfigNotExist || *targetGroupProps.HealthCheckConfig.HealthCheckPort == shared_constants.HealthCheckPortTrafficPort {
		return intstr.FromString(shared_constants.HealthCheckPortTrafficPort), nil
	}

	healthCheckPort := intstr.Parse(*targetGroupProps.HealthCheckConfig.HealthCheckPort)
	if healthCheckPort.Type == intstr.Int {
		return healthCheckPort, nil
	}
	hcSvcPort, err := k8s.LookupServicePort(svc, healthCheckPort)
	if err != nil {
		return intstr.FromString(""), err
	}

	if targetType == elbv2model.TargetTypeInstance {
		return intstr.FromInt(int(hcSvcPort.NodePort)), nil
	}

	if hcSvcPort.TargetPort.Type == intstr.Int {
		return hcSvcPort.TargetPort, nil
	}
	return intstr.IntOrString{}, errors.New("cannot use named healthCheckPort for IP TargetType when service's targetPort is a named port")
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckProtocol(targetGroupProps *elbv2gw.TargetGroupProps, tgProtocol elbv2model.Protocol, isServiceExternalTrafficPolicyTypeLocal bool) elbv2model.Protocol {

	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckProtocol == nil {
		if builder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork {
			if isServiceExternalTrafficPolicyTypeLocal {
				return builder.defaultHealthCheckProtocolForInstanceModeLocal
			}
			return elbv2model.ProtocolTCP
		}
		return tgProtocol
	}

	switch *targetGroupProps.HealthCheckConfig.HealthCheckProtocol {
	case elbv2gw.TargetGroupHealthCheckProtocolTCP:
		return elbv2model.ProtocolTCP
	case elbv2gw.TargetGroupHealthCheckProtocolHTTP:
		return elbv2model.ProtocolHTTP
	case elbv2gw.TargetGroupHealthCheckProtocolHTTPS:
		return elbv2model.ProtocolHTTPS
	default:
		// This should never happen, the CRD validation takes care of this.
		return elbv2model.ProtocolHTTP
	}
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckPath(targetGroupProps *elbv2gw.TargetGroupProps, tgProtocolVersion *elbv2model.ProtocolVersion, hcProtocol elbv2model.Protocol, isServiceExternalTrafficPolicyTypeLocal bool) *string {
	if hcProtocol == elbv2model.ProtocolTCP {
		return nil
	}

	if targetGroupProps != nil && targetGroupProps.HealthCheckConfig != nil && targetGroupProps.HealthCheckConfig.HealthCheckPath != nil {
		return targetGroupProps.HealthCheckConfig.HealthCheckPath
	}

	if tgProtocolVersion != nil && *tgProtocolVersion == elbv2model.ProtocolVersionGRPC {
		return &builder.defaultHealthCheckPathGRPC
	}

	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckPath == nil {
		if builder.loadBalancerType == elbv2model.LoadBalancerTypeNetwork && isServiceExternalTrafficPolicyTypeLocal {
			return &builder.defaultHealthCheckPathForInstanceModeLocal
		}
	}

	return &builder.defaultHealthCheckPathHTTP
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckMatcher(targetGroupProps *elbv2gw.TargetGroupProps, tgProtocolVersion *elbv2model.ProtocolVersion, hcProtocol elbv2model.Protocol) *elbv2model.HealthCheckMatcher {

	if hcProtocol == elbv2model.ProtocolTCP {
		return nil
	}

	useGRPC := tgProtocolVersion != nil && *tgProtocolVersion == elbv2model.ProtocolVersionGRPC

	if useGRPC {
		matcher := builder.defaultHealthCheckMatcherGRPCCode
		if targetGroupProps != nil && targetGroupProps.HealthCheckConfig != nil && targetGroupProps.HealthCheckConfig.Matcher != nil && targetGroupProps.HealthCheckConfig.Matcher.GRPCCode != nil {
			matcher = *targetGroupProps.HealthCheckConfig.Matcher.GRPCCode
		}
		return &elbv2model.HealthCheckMatcher{
			GRPCCode: &matcher,
		}
	}
	matcher := builder.defaultHealthCheckMatcherHTTPCode
	if targetGroupProps != nil && targetGroupProps.HealthCheckConfig != nil && targetGroupProps.HealthCheckConfig.Matcher != nil && targetGroupProps.HealthCheckConfig.Matcher.HTTPCode != nil {
		matcher = *targetGroupProps.HealthCheckConfig.Matcher.HTTPCode
	}
	return &elbv2model.HealthCheckMatcher{
		HTTPCode: &matcher,
	}
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckIntervalSeconds(targetGroupProps *elbv2gw.TargetGroupProps, isServiceExternalTrafficPolicyTypeLocal bool) int32 {
	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckInterval == nil {
		return map[bool]int32{
			true:  builder.defaultHealthCheckIntervalForInstanceModeLocal,
			false: builder.defaultHealthCheckInterval,
		}[isServiceExternalTrafficPolicyTypeLocal]
	}
	return *targetGroupProps.HealthCheckConfig.HealthCheckInterval
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckTimeoutSeconds(targetGroupProps *elbv2gw.TargetGroupProps, isServiceExternalTrafficPolicyTypeLocal bool) int32 {
	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthCheckTimeout == nil {
		return map[bool]int32{
			true:  builder.defaultHealthCheckTimeoutForInstanceModeLocal,
			false: builder.defaultHealthCheckTimeout,
		}[isServiceExternalTrafficPolicyTypeLocal]
	}
	return *targetGroupProps.HealthCheckConfig.HealthCheckTimeout
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckHealthyThresholdCount(targetGroupProps *elbv2gw.TargetGroupProps, isServiceExternalTrafficPolicyTypeLocal bool) int32 {
	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.HealthyThresholdCount == nil {
		return map[bool]int32{
			true:  builder.defaultHealthCheckHealthyThresholdForInstanceModeLocal,
			false: builder.defaultHealthyThresholdCount,
		}[isServiceExternalTrafficPolicyTypeLocal]
	}
	return *targetGroupProps.HealthCheckConfig.HealthyThresholdCount
}

func (builder *targetGroupBuilderImpl) buildTargetGroupHealthCheckUnhealthyThresholdCount(targetGroupProps *elbv2gw.TargetGroupProps, isServiceExternalTrafficPolicyTypeLocal bool) int32 {
	if targetGroupProps == nil || targetGroupProps.HealthCheckConfig == nil || targetGroupProps.HealthCheckConfig.UnhealthyThresholdCount == nil {
		return map[bool]int32{
			true:  builder.defaultHealthCheckUnhealthyThresholdForInstanceModeLocal,
			false: builder.defaultHealthCheckUnhealthyThresholdCount,
		}[isServiceExternalTrafficPolicyTypeLocal]
	}
	return *targetGroupProps.HealthCheckConfig.UnhealthyThresholdCount
}

func (builder *targetGroupBuilderImpl) buildTargetGroupAttributes(targetGroupProps *elbv2gw.TargetGroupProps) map[string]string {
	attributeMap := make(map[string]string)

	if targetGroupProps == nil || targetGroupProps.TargetGroupAttributes == nil {
		return attributeMap
	}

	for _, attr := range targetGroupProps.TargetGroupAttributes {
		attributeMap[attr.Key] = attr.Value
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

func (builder *targetGroupBuilderImpl) buildTargetGroupResourceID(gwKey types.NamespacedName, svcKey types.NamespacedName, routeKey types.NamespacedName, routeKind routeutils.RouteKind, port intstr.IntOrString) string {
	return fmt.Sprintf("%s/%s:%s-%s:%s-%s-%s:%s", gwKey.Namespace, gwKey.Name, routeKey.Namespace, routeKey.Name, routeKind, svcKey.Namespace, svcKey.Name, port.String())
}

func (builder *targetGroupBuilderImpl) buildTargetGroupBindingNodeSelector(tgProps *elbv2gw.TargetGroupProps, targetType elbv2model.TargetType) *metav1.LabelSelector {
	if targetType != elbv2model.TargetTypeInstance || tgProps == nil {
		return nil
	}
	return tgProps.NodeSelector
}

func (builder *targetGroupBuilderImpl) buildTargetGroupBindingMultiClusterFlag(tgProps *elbv2gw.TargetGroupProps) bool {
	if tgProps == nil || tgProps.EnableMultiCluster == nil {
		return false
	}
	return *tgProps.EnableMultiCluster
}
