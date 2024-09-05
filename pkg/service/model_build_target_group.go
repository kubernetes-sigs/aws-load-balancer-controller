package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

const (
	tgAttrsProxyProtocolV2Enabled  = "proxy_protocol_v2.enabled"
	tgAttrsPreserveClientIPEnabled = "preserve_client_ip.enabled"
	healthCheckPortTrafficPort     = "traffic-port"
)

func (t *defaultModelBuildTask) buildTargetGroup(ctx context.Context, port corev1.ServicePort, tgProtocol elbv2model.Protocol, scheme elbv2model.LoadBalancerScheme) (*elbv2model.TargetGroup, error) {
	svcPort := intstr.FromInt(int(port.Port))
	tgResourceID := t.buildTargetGroupResourceID(k8s.NamespacedName(t.service), svcPort)
	if targetGroup, exists := t.tgByResID[tgResourceID]; exists {
		return targetGroup, nil
	}
	targetType, err := t.buildTargetType(ctx, port)
	if err != nil {
		return nil, err
	}
	healthCheckConfig, err := t.buildTargetGroupHealthCheckConfig(ctx, targetType)
	if err != nil {
		return nil, err
	}
	tgAttrs, err := t.buildTargetGroupAttributes(ctx)
	if err != nil {
		return nil, err
	}
	t.preserveClientIP, err = t.buildPreserveClientIPFlag(ctx, targetType, tgAttrs)
	if err != nil {
		return nil, err
	}
	tgSpec, err := t.buildTargetGroupSpec(ctx, tgProtocol, targetType, port, healthCheckConfig, tgAttrs)
	if err != nil {
		return nil, err
	}
	targetGroup := elbv2model.NewTargetGroup(t.stack, tgResourceID, tgSpec)
	_, err = t.buildTargetGroupBinding(ctx, targetGroup, port, healthCheckConfig, scheme)
	if err != nil {
		return nil, err
	}
	t.tgByResID[tgResourceID] = targetGroup
	return targetGroup, nil
}

func (t *defaultModelBuildTask) buildTargetGroupSpec(ctx context.Context, tgProtocol elbv2model.Protocol, targetType elbv2model.TargetType,
	port corev1.ServicePort, healthCheckConfig *elbv2model.TargetGroupHealthCheckConfig, tgAttrs []elbv2model.TargetGroupAttribute) (elbv2model.TargetGroupSpec, error) {
	tags, err := t.buildTargetGroupTags(ctx)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	targetPort := t.buildTargetGroupPort(ctx, targetType, port)
	tgName := t.buildTargetGroupName(ctx, intstr.FromInt(int(port.Port)), targetPort, targetType, tgProtocol, healthCheckConfig)
	ipAddressType, err := t.buildTargetGroupIPAddressType(ctx, t.service)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	return elbv2model.TargetGroupSpec{
		Name:                  tgName,
		TargetType:            targetType,
		Port:                  awssdk.Int32(targetPort),
		Protocol:              tgProtocol,
		IPAddressType:         ipAddressType,
		HealthCheckConfig:     healthCheckConfig,
		TargetGroupAttributes: tgAttrs,
		Tags:                  tags,
	}, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckConfig(ctx context.Context, targetType elbv2model.TargetType) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	if targetType == elbv2model.TargetTypeInstance && t.service.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal &&
		t.service.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return t.buildTargetGroupHealthCheckConfigForInstanceModeLocal(ctx, targetType)
	}
	return t.buildTargetGroupHealthCheckConfigDefault(ctx, targetType)
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckConfigDefault(ctx context.Context, targetType elbv2model.TargetType) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	healthCheckProtocol, err := t.buildTargetGroupHealthCheckProtocol(ctx, t.defaultHealthCheckProtocol)
	if err != nil {
		return nil, err
	}
	healthCheckPathPtr := t.buildTargetGroupHealthCheckPath(ctx, t.defaultHealthCheckPath, healthCheckProtocol)
	healthCheckMatcherPtr := t.buildTargetGroupHealthCheckMatcher(ctx, healthCheckProtocol)
	healthCheckPort, err := t.buildTargetGroupHealthCheckPort(ctx, t.defaultHealthCheckPort, targetType)
	if err != nil {
		return nil, err
	}
	intervalSeconds, err := t.buildTargetGroupHealthCheckIntervalSeconds(ctx, t.defaultHealthCheckInterval)
	if err != nil {
		return nil, err
	}
	healthCheckTimeoutSecondsPtr, err := t.buildTargetGroupHealthCheckTimeoutSeconds(ctx, t.defaultHealthCheckTimeout)
	if err != nil {
		return nil, err
	}

	healthyThresholdCount, err := t.buildTargetGroupHealthCheckHealthyThresholdCount(ctx, t.defaultHealthCheckHealthyThreshold)
	if err != nil {
		return nil, err
	}
	unhealthyThresholdCount, err := t.buildTargetGroupHealthCheckUnhealthyThresholdCount(ctx, t.defaultHealthCheckUnhealthyThreshold)
	if err != nil {
		return nil, err
	}
	return &elbv2model.TargetGroupHealthCheckConfig{
		Port:                    &healthCheckPort,
		Protocol:                healthCheckProtocol,
		Path:                    healthCheckPathPtr,
		Matcher:                 healthCheckMatcherPtr,
		IntervalSeconds:         &intervalSeconds,
		TimeoutSeconds:          healthCheckTimeoutSecondsPtr,
		HealthyThresholdCount:   &healthyThresholdCount,
		UnhealthyThresholdCount: &unhealthyThresholdCount,
	}, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckConfigForInstanceModeLocal(ctx context.Context, targetType elbv2model.TargetType) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	healthCheckProtocol, err := t.buildTargetGroupHealthCheckProtocol(ctx, t.defaultHealthCheckProtocolForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	healthCheckPathPtr := t.buildTargetGroupHealthCheckPath(ctx, t.defaultHealthCheckPathForInstanceModeLocal, healthCheckProtocol)
	healthCheckMatcherPtr := t.buildTargetGroupHealthCheckMatcher(ctx, healthCheckProtocol)
	healthCheckPort, err := t.buildTargetGroupHealthCheckPort(ctx, t.defaultHealthCheckPortForInstanceModeLocal, targetType)
	if err != nil {
		return nil, err
	}
	intervalSeconds, err := t.buildTargetGroupHealthCheckIntervalSeconds(ctx, t.defaultHealthCheckIntervalForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	healthCheckTimeoutSecondsPtr, err := t.buildTargetGroupHealthCheckTimeoutSeconds(ctx, t.defaultHealthCheckTimeoutForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	healthyThresholdCount, err := t.buildTargetGroupHealthCheckHealthyThresholdCount(ctx, t.defaultHealthCheckHealthyThresholdForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	unhealthyThresholdCount, err := t.buildTargetGroupHealthCheckUnhealthyThresholdCount(ctx, t.defaultHealthCheckUnhealthyThresholdForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	return &elbv2model.TargetGroupHealthCheckConfig{
		Port:                    &healthCheckPort,
		Protocol:                healthCheckProtocol,
		Path:                    healthCheckPathPtr,
		Matcher:                 healthCheckMatcherPtr,
		IntervalSeconds:         &intervalSeconds,
		TimeoutSeconds:          healthCheckTimeoutSecondsPtr,
		HealthyThresholdCount:   &healthyThresholdCount,
		UnhealthyThresholdCount: &unhealthyThresholdCount,
	}, nil
}

var invalidTargetGroupNamePattern = regexp.MustCompile("[[:^alnum:]]")

func (t *defaultModelBuildTask) buildTargetGroupName(_ context.Context, svcPort intstr.IntOrString, tgPort int32,
	targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol, hc *elbv2model.TargetGroupHealthCheckConfig) string {
	healthCheckProtocol := string(elbv2model.ProtocolTCP)
	healthCheckInterval := strconv.FormatInt(int64(t.defaultHealthCheckInterval), 10)
	if &hc.Protocol != nil {
		healthCheckProtocol = string(hc.Protocol)
	}
	if hc.IntervalSeconds != nil {
		healthCheckInterval = strconv.FormatInt(int64(*hc.IntervalSeconds), 10)
	}
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.service.UID))
	_, _ = uuidHash.Write([]byte(strconv.Itoa(int(tgPort))))
	_, _ = uuidHash.Write([]byte(svcPort.String()))
	_, _ = uuidHash.Write([]byte(targetType))
	_, _ = uuidHash.Write([]byte(tgProtocol))
	_, _ = uuidHash.Write([]byte(healthCheckProtocol))
	_, _ = uuidHash.Write([]byte(healthCheckInterval))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidTargetGroupNamePattern.ReplaceAllString(t.service.Namespace, "")
	sanitizedName := invalidTargetGroupNamePattern.ReplaceAllString(t.service.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (t *defaultModelBuildTask) buildTargetGroupAttributes(_ context.Context) ([]elbv2model.TargetGroupAttribute, error) {
	var rawAttributes map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixTargetGroupAttributes, &rawAttributes, t.service.Annotations); err != nil {
		return nil, err
	}
	if rawAttributes == nil {
		rawAttributes = make(map[string]string)
	}
	if _, ok := rawAttributes[tgAttrsProxyProtocolV2Enabled]; !ok {
		rawAttributes[tgAttrsProxyProtocolV2Enabled] = strconv.FormatBool(t.defaultProxyProtocolV2Enabled)
	}
	proxyV2Annotation := ""
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixProxyProtocol, &proxyV2Annotation, t.service.Annotations); exists {
		if proxyV2Annotation != "*" {
			return []elbv2model.TargetGroupAttribute{}, errors.Errorf("invalid value %v for Load Balancer proxy protocol v2 annotation, only value currently supported is *", proxyV2Annotation)
		}
		rawAttributes[tgAttrsProxyProtocolV2Enabled] = "true"
	}
	if rawPreserveIPEnabled, ok := rawAttributes[tgAttrsPreserveClientIPEnabled]; ok {
		_, err := strconv.ParseBool(rawPreserveIPEnabled)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse attribute %v=%v", tgAttrsPreserveClientIPEnabled, rawPreserveIPEnabled)
		}
	}
	attributes := make([]elbv2model.TargetGroupAttribute, 0, len(rawAttributes))
	for attrKey, attrValue := range rawAttributes {
		attributes = append(attributes, elbv2model.TargetGroupAttribute{
			Key:   attrKey,
			Value: attrValue,
		})
	}
	sort.Slice(attributes, func(i, j int) bool {
		return attributes[i].Key < attributes[j].Key
	})
	return attributes, nil
}

func (t *defaultModelBuildTask) buildPreserveClientIPFlag(_ context.Context, targetType elbv2model.TargetType, tgAttrs []elbv2model.TargetGroupAttribute) (bool, error) {
	for _, attr := range tgAttrs {
		if attr.Key == tgAttrsPreserveClientIPEnabled {
			preserveClientIP, err := strconv.ParseBool(attr.Value)
			if err != nil {
				return false, errors.Wrapf(err, "failed to parse attribute %v=%v", tgAttrsPreserveClientIPEnabled, attr.Value)
			}
			return preserveClientIP, nil
		}
	}
	switch targetType {
	case elbv2model.TargetTypeIP:
		return false, nil
	case elbv2model.TargetTypeInstance:
		return true, nil
	}
	return false, nil
}

// buildTargetGroupPort constructs the TargetGroup's port.
// Note: TargetGroup's port is not in the data path as we always register targets with port specified.
// so this setting don't really matter to our controller, and we do our best to use the most appropriate port as targetGroup's port to avoid UX confusion.
func (t *defaultModelBuildTask) buildTargetGroupPort(_ context.Context, targetType elbv2model.TargetType, svcPort corev1.ServicePort) int32 {
	if targetType == elbv2model.TargetTypeInstance {
		return svcPort.NodePort
	}
	if svcPort.TargetPort.Type == intstr.Int {
		return int32(svcPort.TargetPort.IntValue())
	}

	// when a literal targetPort is used, we just use a fixed 1 here as this setting is not in the data path.
	// also, under extreme edge case, it can actually be different ports for different pods.
	return 1
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckPort(_ context.Context, defaultHealthCheckPort string, targetType elbv2model.TargetType) (intstr.IntOrString, error) {
	rawHealthCheckPort := defaultHealthCheckPort
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPort, &rawHealthCheckPort, t.service.Annotations)
	if rawHealthCheckPort == healthCheckPortTrafficPort {
		return intstr.FromString(rawHealthCheckPort), nil
	}
	healthCheckPort := intstr.Parse(rawHealthCheckPort)
	if healthCheckPort.Type == intstr.Int {
		return healthCheckPort, nil
	}

	svcPort, err := k8s.LookupServicePort(t.service, healthCheckPort)
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

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckProtocol(_ context.Context, defaultHealthCheckProtocol elbv2model.Protocol) (elbv2model.Protocol, error) {
	rawHealthCheckProtocol := string(defaultHealthCheckProtocol)
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCProtocol, &rawHealthCheckProtocol, t.service.Annotations)
	switch strings.ToUpper(rawHealthCheckProtocol) {
	case string(elbv2model.ProtocolTCP):
		return elbv2model.ProtocolTCP, nil
	case string(elbv2model.ProtocolHTTP):
		return elbv2model.ProtocolHTTP, nil
	case string(elbv2model.ProtocolHTTPS):
		return elbv2model.ProtocolHTTPS, nil
	default:
		return "", errors.Errorf("unsupported health check protocol %v", rawHealthCheckProtocol)
	}
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckPath(_ context.Context, defaultHealthCheckPath string, hcProtocol elbv2model.Protocol) *string {
	if hcProtocol == elbv2model.ProtocolTCP {
		return nil
	}
	healthCheckPath := defaultHealthCheckPath
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPath, &healthCheckPath, t.service.Annotations)
	return &healthCheckPath
}
func (t *defaultModelBuildTask) buildTargetGroupHealthCheckMatcher(_ context.Context, hcProtocol elbv2model.Protocol) *elbv2model.HealthCheckMatcher {
	if hcProtocol == elbv2model.ProtocolTCP || !t.featureGates.Enabled(config.NLBHealthCheckAdvancedConfig) {
		return nil
	}
	rawHealthCheckMatcherSuccessCodes := t.defaultHealthCheckMatcherHTTPCode
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCSuccessCodes, &rawHealthCheckMatcherSuccessCodes, t.service.Annotations)
	return &elbv2model.HealthCheckMatcher{
		HTTPCode: &rawHealthCheckMatcherSuccessCodes,
	}
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckIntervalSeconds(_ context.Context, defaultHealthCheckInterval int32) (int32, error) {
	intervalSeconds := defaultHealthCheckInterval
	if _, err := t.annotationParser.ParseInt32Annotation(annotations.SvcLBSuffixHCInterval, &intervalSeconds, t.service.Annotations); err != nil {
		return 0, err
	}
	return intervalSeconds, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckTimeoutSeconds(_ context.Context, defaultHealthCheckTimeout int32) (*int32, error) {
	timeoutSeconds := defaultHealthCheckTimeout
	if !t.featureGates.Enabled(config.NLBHealthCheckAdvancedConfig) {
		return awssdk.Int32(timeoutSeconds), nil
	}
	if _, err := t.annotationParser.ParseInt32Annotation(annotations.SvcLBSuffixHCTimeout, &timeoutSeconds, t.service.Annotations); err != nil {
		return nil, err
	}
	return awssdk.Int32(timeoutSeconds), nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckHealthyThresholdCount(_ context.Context, defaultHealthCheckHealthyThreshold int32) (int32, error) {
	healthyThresholdCount := defaultHealthCheckHealthyThreshold
	if _, err := t.annotationParser.ParseInt32Annotation(annotations.SvcLBSuffixHCHealthyThreshold, &healthyThresholdCount, t.service.Annotations); err != nil {
		return 0, err
	}
	return healthyThresholdCount, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckUnhealthyThresholdCount(_ context.Context, defaultHealthCheckUnhealthyThreshold int32) (int32, error) {
	unhealthyThresholdCount := defaultHealthCheckUnhealthyThreshold
	if _, err := t.annotationParser.ParseInt32Annotation(annotations.SvcLBSuffixHCUnhealthyThreshold, &unhealthyThresholdCount, t.service.Annotations); err != nil {
		return 0, err
	}
	return unhealthyThresholdCount, nil
}

func (t *defaultModelBuildTask) buildTargetType(_ context.Context, port corev1.ServicePort) (elbv2model.TargetType, error) {
	svcType := t.service.Spec.Type
	var lbType string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixLoadBalancerType, &lbType, t.service.Annotations)
	var lbTargetType string
	lbTargetType = string(t.defaultTargetType)
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixTargetType, &lbTargetType, t.service.Annotations)
	if lbTargetType == LoadBalancerTargetTypeIP && !t.enableIPTargetType {
		return "", errors.Errorf("unsupported targetType: %v when EnableIPTargetType is %v", lbTargetType, t.enableIPTargetType)
	}
	if lbType == LoadBalancerTypeNLBIP || lbTargetType == LoadBalancerTargetTypeIP {
		return elbv2model.TargetTypeIP, nil
	}
	if svcType == corev1.ServiceTypeClusterIP {
		return "", errors.Errorf("unsupported service type \"%v\" for load balancer target type \"%v\"", svcType, lbTargetType)
	}
	if port.NodePort == 0 && t.service.Spec.AllocateLoadBalancerNodePorts != nil && !*t.service.Spec.AllocateLoadBalancerNodePorts {
		return "", errors.New("unable to support instance target type with an unallocated NodePort")
	}
	return elbv2model.TargetTypeInstance, nil
}

func (t *defaultModelBuildTask) buildTargetGroupResourceID(svcKey types.NamespacedName, port intstr.IntOrString) string {
	return fmt.Sprintf("%s/%s:%s", svcKey.Namespace, svcKey.Name, port.String())
}

func (t *defaultModelBuildTask) buildTargetGroupTags(ctx context.Context) (map[string]string, error) {
	return t.buildAdditionalResourceTags(ctx)
}

func (t *defaultModelBuildTask) buildTargetGroupBinding(ctx context.Context, targetGroup *elbv2model.TargetGroup,
	port corev1.ServicePort, hc *elbv2model.TargetGroupHealthCheckConfig, scheme elbv2model.LoadBalancerScheme) (*elbv2model.TargetGroupBindingResource, error) {
	tgbSpec, err := t.buildTargetGroupBindingSpec(ctx, targetGroup, port, hc, scheme)
	if err != nil {
		return nil, err
	}
	return elbv2model.NewTargetGroupBindingResource(t.stack, targetGroup.ID(), tgbSpec), nil
}

func (t *defaultModelBuildTask) buildTargetGroupBindingSpec(ctx context.Context, targetGroup *elbv2model.TargetGroup,
	port corev1.ServicePort, hc *elbv2model.TargetGroupHealthCheckConfig, scheme elbv2model.LoadBalancerScheme) (elbv2model.TargetGroupBindingResourceSpec, error) {
	nodeSelector, err := t.buildTargetGroupBindingNodeSelector(ctx, targetGroup.Spec.TargetType)
	if err != nil {
		return elbv2model.TargetGroupBindingResourceSpec{}, err
	}
	targetPort := port.TargetPort
	targetType := elbv2api.TargetType(targetGroup.Spec.TargetType)
	if targetType == elbv2api.TargetTypeInstance {
		targetPort = intstr.FromInt(int(port.NodePort))
	}
	var tgbNetworking *elbv2model.TargetGroupBindingNetworking
	if len(t.loadBalancer.Spec.SecurityGroups) == 0 {
		tgbNetworking, err = t.buildTargetGroupBindingNetworkingLegacy(ctx, targetPort, *hc.Port, port, scheme, targetGroup.Spec.IPAddressType)
	} else {
		tgbNetworking, err = t.buildTargetGroupBindingNetworking(ctx, targetPort, *hc.Port, port)
	}
	if err != nil {
		return elbv2model.TargetGroupBindingResourceSpec{}, err
	}
	return elbv2model.TargetGroupBindingResourceSpec{
		Template: elbv2model.TargetGroupBindingTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: t.service.Namespace,
				Name:      targetGroup.Spec.Name,
			},
			Spec: elbv2model.TargetGroupBindingSpec{
				TargetGroupARN: targetGroup.TargetGroupARN(),
				TargetType:     &targetType,
				ServiceRef: elbv2api.ServiceReference{
					Name: t.service.Name,
					Port: intstr.FromInt(int(port.Port)),
				},
				Networking:    tgbNetworking,
				NodeSelector:  nodeSelector,
				IPAddressType: elbv2api.TargetGroupIPAddressType(targetGroup.Spec.IPAddressType),
				VpcID:         t.vpcID,
			},
		},
	}, nil
}

func (t *defaultModelBuildTask) buildTargetGroupBindingNetworking(_ context.Context, tgPort intstr.IntOrString,
	hcPort intstr.IntOrString, port corev1.ServicePort) (*elbv2model.TargetGroupBindingNetworking, error) {
	if t.backendSGIDToken == nil {
		return nil, nil
	}
	protocolTCP := elbv2api.NetworkingProtocolTCP
	protocolUDP := elbv2api.NetworkingProtocolUDP

	var ports []elbv2api.NetworkingPort
	if t.disableRestrictedSGRules {
		ports = append(ports, elbv2api.NetworkingPort{
			Protocol: &protocolTCP,
			Port:     nil,
		})
		if port.Protocol == corev1.ProtocolUDP {
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolUDP,
				Port:     nil,
			})
		}
	} else {
		switch port.Protocol {
		case corev1.ProtocolTCP:
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolTCP,
				Port:     &tgPort,
			})
		case corev1.ProtocolUDP:
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolUDP,
				Port:     &tgPort,
			})
			if hcPort.String() == healthCheckPortTrafficPort || (hcPort.Type == intstr.Int && hcPort.IntValue() == tgPort.IntValue()) {
				ports = append(ports, elbv2api.NetworkingPort{
					Protocol: &protocolTCP,
					Port:     &tgPort,
				})
			}
		}

		if hcPort.String() != healthCheckPortTrafficPort && (hcPort.Type == intstr.Int && hcPort.IntValue() != tgPort.IntValue()) {
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolTCP,
				Port:     &hcPort,
			})
		}
	}
	return &elbv2model.TargetGroupBindingNetworking{
		Ingress: []elbv2model.NetworkingIngressRule{
			{
				From: []elbv2model.NetworkingPeer{
					{
						SecurityGroup: &elbv2model.SecurityGroup{
							GroupID: t.backendSGIDToken,
						},
					},
				},
				Ports: ports,
			},
		},
	}, nil
}

func (t *defaultModelBuildTask) getLoadBalancerSourceRanges(_ context.Context) []string {
	var sourceRanges []string
	for _, cidr := range t.service.Spec.LoadBalancerSourceRanges {
		sourceRanges = append(sourceRanges, cidr)
	}
	if len(sourceRanges) == 0 {
		t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSourceRanges, &sourceRanges, t.service.Annotations)
	}
	return sourceRanges
}

func (t *defaultModelBuildTask) buildPeersFromSourceRangeCIDRs(_ context.Context, sourceRanges []string) []elbv2model.NetworkingPeer {
	var peers []elbv2model.NetworkingPeer
	for _, cidr := range sourceRanges {
		peers = append(peers, elbv2model.NetworkingPeer{
			IPBlock: &elbv2api.IPBlock{
				CIDR: cidr,
			},
		})
	}
	return peers
}

func (t *defaultModelBuildTask) buildTargetGroupBindingNetworkingLegacy(ctx context.Context, tgPort intstr.IntOrString,
	hcPort intstr.IntOrString, port corev1.ServicePort, scheme elbv2model.LoadBalancerScheme, targetGroupIPAddressType elbv2model.TargetGroupIPAddressType) (*elbv2model.TargetGroupBindingNetworking, error) {
	manageBackendSGRules, err := t.buildManageSecurityGroupRulesFlagLegacy(ctx)
	if err != nil {
		return nil, err
	}
	if !manageBackendSGRules {
		return nil, nil
	}
	tgProtocol := port.Protocol
	networkingProtocol := elbv2api.NetworkingProtocolTCP
	healthCheckProtocol := elbv2api.NetworkingProtocolTCP
	if tgProtocol == corev1.ProtocolUDP {
		networkingProtocol = elbv2api.NetworkingProtocolUDP
	}
	loadBalancerSubnetCIDRs := t.getLoadBalancerSubnetsSourceRanges(targetGroupIPAddressType)
	trafficSource := loadBalancerSubnetCIDRs
	defaultRangeUsed := false
	if networkingProtocol == elbv2api.NetworkingProtocolUDP || t.preserveClientIP {
		trafficSource = t.getLoadBalancerSourceRanges(ctx)
		if len(trafficSource) == 0 {
			trafficSource, err = t.getDefaultIPSourceRanges(ctx, targetGroupIPAddressType, port.Protocol, scheme)
			if err != nil {
				return nil, err
			}
			defaultRangeUsed = true
		}
	}
	tgbNetworking := &elbv2model.TargetGroupBindingNetworking{
		Ingress: []elbv2model.NetworkingIngressRule{
			{
				From: t.buildPeersFromSourceRangeCIDRs(ctx, trafficSource),
				Ports: []elbv2api.NetworkingPort{
					{
						Port:     &tgPort,
						Protocol: &networkingProtocol,
					},
				},
			},
		},
	}
	if healthCheckSourceCIDRs := t.buildHealthCheckSourceCIDRs(trafficSource, loadBalancerSubnetCIDRs, tgPort, hcPort,
		tgProtocol, defaultRangeUsed); len(healthCheckSourceCIDRs) > 0 {
		networkingHealthCheckPort := hcPort
		if hcPort.String() == healthCheckPortTrafficPort {
			networkingHealthCheckPort = tgPort
		}
		tgbNetworking.Ingress = append(tgbNetworking.Ingress, elbv2model.NetworkingIngressRule{
			From: t.buildPeersFromSourceRangeCIDRs(ctx, healthCheckSourceCIDRs),
			Ports: []elbv2api.NetworkingPort{
				{
					Port:     &networkingHealthCheckPort,
					Protocol: &healthCheckProtocol,
				},
			},
		})
	}
	return tgbNetworking, nil
}

func (t *defaultModelBuildTask) getDefaultIPSourceRanges(ctx context.Context, targetGroupIPAddressType elbv2model.TargetGroupIPAddressType,
	protocol corev1.Protocol, scheme elbv2model.LoadBalancerScheme) ([]string, error) {
	defaultSourceRanges := t.defaultIPv4SourceRanges
	if targetGroupIPAddressType == elbv2model.TargetGroupIPAddressTypeIPv6 {
		defaultSourceRanges = t.defaultIPv6SourceRanges
	}
	if (protocol == corev1.ProtocolUDP || t.preserveClientIP) && scheme == elbv2model.LoadBalancerSchemeInternal {
		vpcInfo, err := t.vpcInfoProvider.FetchVPCInfo(ctx, t.vpcID, networking.FetchVPCInfoWithoutCache())
		if err != nil {
			return nil, err
		}
		if targetGroupIPAddressType == elbv2model.TargetGroupIPAddressTypeIPv4 {
			defaultSourceRanges = vpcInfo.AssociatedIPv4CIDRs()
		} else {
			defaultSourceRanges = vpcInfo.AssociatedIPv6CIDRs()
		}
	}
	return defaultSourceRanges, nil
}

func (t *defaultModelBuildTask) getLoadBalancerSubnetsSourceRanges(targetGroupIPAddressType elbv2model.TargetGroupIPAddressType) []string {
	var subnetCIDRs []string
	for _, subnet := range t.ec2Subnets {
		if targetGroupIPAddressType == elbv2model.TargetGroupIPAddressTypeIPv4 {
			subnetCIDRs = append(subnetCIDRs, awssdk.ToString(subnet.CidrBlock))
		} else {
			for _, ipv6CIDRBlockAssoc := range subnet.Ipv6CidrBlockAssociationSet {
				subnetCIDRs = append(subnetCIDRs, awssdk.ToString(ipv6CIDRBlockAssoc.Ipv6CidrBlock))
			}
		}
	}
	return subnetCIDRs
}

func (t *defaultModelBuildTask) buildTargetGroupIPAddressType(_ context.Context, svc *corev1.Service) (elbv2model.TargetGroupIPAddressType, error) {
	var ipv6Configured bool
	for _, ipFamily := range svc.Spec.IPFamilies {
		if ipFamily == corev1.IPv6Protocol {
			ipv6Configured = true
			break
		}
	}
	if ipv6Configured {
		if elbv2model.IPAddressTypeDualStack != t.loadBalancer.Spec.IPAddressType {
			return "", errors.New("unsupported IPv6 configuration, lb not dual-stack")
		}
		return elbv2model.TargetGroupIPAddressTypeIPv6, nil
	}
	return elbv2model.TargetGroupIPAddressTypeIPv4, nil
}

func (t *defaultModelBuildTask) buildTargetGroupBindingNodeSelector(_ context.Context, targetType elbv2model.TargetType) (*metav1.LabelSelector, error) {
	if targetType != elbv2model.TargetTypeInstance {
		return nil, nil
	}
	var targetNodeLabels map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixTargetNodeLabels, &targetNodeLabels, t.service.Annotations); err != nil {
		return nil, err
	}
	if len(targetNodeLabels) == 0 {
		return nil, nil
	}
	return &metav1.LabelSelector{
		MatchLabels: targetNodeLabels,
	}, nil
}

func (t *defaultModelBuildTask) buildHealthCheckSourceCIDRs(trafficSource, subnetCIDRs []string, tgPort, hcPort intstr.IntOrString,
	tgProtocol corev1.Protocol, defaultRangeUsed bool) []string {
	if tgProtocol != corev1.ProtocolUDP &&
		(hcPort.String() == healthCheckPortTrafficPort || hcPort.IntValue() == tgPort.IntValue()) {
		if !t.preserveClientIP {
			return nil
		}
		if defaultRangeUsed {
			return nil
		}
		for _, src := range trafficSource {
			if src == "0.0.0.0/0" || src == "::/0" {
				return nil
			}
		}
	}
	return subnetCIDRs
}

func (t *defaultModelBuildTask) buildManageSecurityGroupRulesFlagLegacy(_ context.Context) (bool, error) {
	var rawEnabled bool
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixManageSGRules, &rawEnabled, t.service.Annotations)
	if err != nil {
		return true, err
	}
	if exists {
		return rawEnabled, nil
	}
	return true, nil
}
