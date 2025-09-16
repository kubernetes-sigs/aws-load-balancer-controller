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

	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"

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
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
)

// Build target groups for weighted target groups
func (t *defaultModelBuildTask) buildTargetGroupTuples(ctx context.Context, listenerProtocol elbv2model.Protocol, baseTgPort corev1.ServicePort, baseTgProtocol elbv2model.Protocol, action Action, scheme elbv2model.LoadBalancerScheme) ([]elbv2model.TargetGroupTuple, error) {
	var targetGroupTuples []elbv2model.TargetGroupTuple

	// Build target group for the base service
	baseSvcWeight := action.ForwardConfig.BaseServiceWeight
	baseSvcAnnotations := t.service.Annotations
	// Build target group for the service that owns the load balancer (base service)
	tg, err := t.buildTargetGroup(ctx, nil, t.service, baseSvcAnnotations, baseTgPort, baseTgProtocol, scheme)
	if err != nil {
		return []elbv2model.TargetGroupTuple{}, err
	}
	targetGroupTuples = append(targetGroupTuples, elbv2model.TargetGroupTuple{
		TargetGroupARN: tg.TargetGroupARN(),
		Weight:         baseSvcWeight,
	})

	// Build other target groups
	for _, tgt := range action.ForwardConfig.TargetGroups {
		var tgARN core.StringToken
		if tgt.TargetGroupARN != nil {
			tgARN = core.LiteralStringToken(*tgt.TargetGroupARN)
		} else {
			svcKey := types.NamespacedName{
				Namespace: t.service.Namespace,
				Name:      awssdk.ToString(tgt.ServiceName),
			}
			svc := t.backendServices[svcKey]

			// Get the protocol of the target group, default to listener protocol
			var tgProtocol elbv2model.Protocol
			if listenerProtocol == elbv2model.ProtocolTLS && tgt.Decrypt != nil && *tgt.Decrypt {
				tgProtocol = elbv2model.ProtocolTCP
			} else {
				tgProtocol = listenerProtocol
			}

			port, err := k8s.LookupServicePort(svc, *tgt.ServicePort)
			if err != nil {
				return []elbv2model.TargetGroupTuple{}, err
			}

			// This service doesn't have its own annotations, use the base service annotations
			// Pass the base service (t.service) to ensure unique target group names for shared backend services (svc)
			tg, err = t.buildTargetGroup(ctx, t.service, svc, baseSvcAnnotations, port, tgProtocol, scheme)
			if err != nil {
				return []elbv2model.TargetGroupTuple{}, err
			}
			tgARN = tg.TargetGroupARN()
		}
		targetGroupTuples = append(targetGroupTuples, elbv2model.TargetGroupTuple{
			TargetGroupARN: tgARN,
			Weight:         tgt.Weight,
		})
	}

	return targetGroupTuples, nil
}

// The service and annotations are passed in separately to accommodate weighted target group creation where services don't have their own annotations.
// In that case we will apply the annotations of the base service to all services.
// baseSvc is the service that owns the load balancer, which may be different from svc for weighted target groups
func (t *defaultModelBuildTask) buildTargetGroup(ctx context.Context, baseSvc *corev1.Service, svc *corev1.Service, baseSvcAnnotations map[string]string, port corev1.ServicePort, tgProtocol elbv2model.Protocol, scheme elbv2model.LoadBalancerScheme) (*elbv2model.TargetGroup, error) {
	svcPort := intstr.FromInt(int(port.Port))
	tgResourceID := t.buildTargetGroupResourceID(k8s.NamespacedName(svc), svcPort)
	if targetGroup, exists := t.tgByResID[tgResourceID]; exists {
		return targetGroup, nil
	}
	targetType, err := t.buildTargetType(ctx, svc, baseSvcAnnotations, port)
	if err != nil {
		return nil, err
	}
	healthCheckConfig, err := t.buildTargetGroupHealthCheckConfig(ctx, svc, baseSvcAnnotations, targetType)
	if err != nil {
		return nil, err
	}
	tgAttrs, err := t.buildTargetGroupAttributes(ctx, baseSvcAnnotations, port)
	if err != nil {
		return nil, err
	}
	t.preserveClientIP, err = t.buildPreserveClientIPFlag(ctx, targetType, tgAttrs)
	if err != nil {
		return nil, err
	}
	tgSpec, err := t.buildTargetGroupSpec(ctx, baseSvc, svc, tgProtocol, targetType, port, healthCheckConfig, tgAttrs)
	if err != nil {
		return nil, err
	}
	targetGroup := elbv2model.NewTargetGroup(t.stack, tgResourceID, tgSpec)
	_, err = t.buildTargetGroupBinding(ctx, svc, baseSvcAnnotations, targetGroup, port, healthCheckConfig, scheme)
	if err != nil {
		return nil, err
	}
	t.tgByResID[tgResourceID] = targetGroup
	return targetGroup, nil
}

func (t *defaultModelBuildTask) buildTargetGroupSpec(ctx context.Context, baseSvc *corev1.Service, svc *corev1.Service, tgProtocol elbv2model.Protocol, targetType elbv2model.TargetType,
	port corev1.ServicePort, healthCheckConfig *elbv2model.TargetGroupHealthCheckConfig, tgAttrs []elbv2model.TargetGroupAttribute) (elbv2model.TargetGroupSpec, error) {
	tags, err := t.buildTargetGroupTags(ctx)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	targetPort := t.buildTargetGroupPort(ctx, targetType, port)
	tgName := t.buildTargetGroupName(ctx, baseSvc, svc, intstr.FromInt(int(port.Port)), targetPort, targetType, tgProtocol, healthCheckConfig)
	ipAddressType, err := t.buildTargetGroupIPAddressType(ctx, svc)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}

	if targetPort == 0 {
		if targetType == elbv2model.TargetTypeIP {
			return elbv2model.TargetGroupSpec{}, errors.Errorf("TargetGroup port is empty. Are you using the correct service type?")
		}
		return elbv2model.TargetGroupSpec{}, errors.Errorf("TargetGroup port is empty. When using Instance targets, your service be must of type 'NodePort' or 'LoadBalancer'")
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

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckConfig(ctx context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string, targetType elbv2model.TargetType) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	if targetType == elbv2model.TargetTypeInstance && svc.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal &&
		svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return t.buildTargetGroupHealthCheckConfigForInstanceModeLocal(ctx, svc, baseSvcAnnotations, targetType)
	}
	return t.buildTargetGroupHealthCheckConfigDefault(ctx, svc, baseSvcAnnotations, targetType)
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckConfigDefault(ctx context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string, targetType elbv2model.TargetType) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	healthCheckProtocol, err := t.buildTargetGroupHealthCheckProtocol(ctx, baseSvcAnnotations, t.defaultHealthCheckProtocol)
	if err != nil {
		return nil, err
	}
	healthCheckPathPtr := t.buildTargetGroupHealthCheckPath(ctx, baseSvcAnnotations, t.defaultHealthCheckPath, healthCheckProtocol)
	healthCheckMatcherPtr := t.buildTargetGroupHealthCheckMatcher(ctx, baseSvcAnnotations, healthCheckProtocol)
	healthCheckPort, err := t.buildTargetGroupHealthCheckPort(ctx, svc, baseSvcAnnotations, t.defaultHealthCheckPort, targetType)
	if err != nil {
		return nil, err
	}
	intervalSeconds, err := t.buildTargetGroupHealthCheckIntervalSeconds(ctx, baseSvcAnnotations, t.defaultHealthCheckInterval)
	if err != nil {
		return nil, err
	}
	healthCheckTimeoutSecondsPtr, err := t.buildTargetGroupHealthCheckTimeoutSeconds(ctx, baseSvcAnnotations, t.defaultHealthCheckTimeout)
	if err != nil {
		return nil, err
	}

	healthyThresholdCount, err := t.buildTargetGroupHealthCheckHealthyThresholdCount(ctx, baseSvcAnnotations, t.defaultHealthCheckHealthyThreshold)
	if err != nil {
		return nil, err
	}
	unhealthyThresholdCount, err := t.buildTargetGroupHealthCheckUnhealthyThresholdCount(ctx, baseSvcAnnotations, t.defaultHealthCheckUnhealthyThreshold)
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

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckConfigForInstanceModeLocal(ctx context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string, targetType elbv2model.TargetType) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	healthCheckProtocol, err := t.buildTargetGroupHealthCheckProtocol(ctx, baseSvcAnnotations, t.defaultHealthCheckProtocolForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	healthCheckPathPtr := t.buildTargetGroupHealthCheckPath(ctx, baseSvcAnnotations, t.defaultHealthCheckPathForInstanceModeLocal, healthCheckProtocol)
	healthCheckMatcherPtr := t.buildTargetGroupHealthCheckMatcher(ctx, baseSvcAnnotations, healthCheckProtocol)
	healthCheckPort, err := t.buildTargetGroupHealthCheckPort(ctx, svc, baseSvcAnnotations, t.defaultHealthCheckPortForInstanceModeLocal, targetType)
	if err != nil {
		return nil, err
	}
	intervalSeconds, err := t.buildTargetGroupHealthCheckIntervalSeconds(ctx, baseSvcAnnotations, t.defaultHealthCheckIntervalForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	healthCheckTimeoutSecondsPtr, err := t.buildTargetGroupHealthCheckTimeoutSeconds(ctx, baseSvcAnnotations, t.defaultHealthCheckTimeoutForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	healthyThresholdCount, err := t.buildTargetGroupHealthCheckHealthyThresholdCount(ctx, baseSvcAnnotations, t.defaultHealthCheckHealthyThresholdForInstanceModeLocal)
	if err != nil {
		return nil, err
	}
	unhealthyThresholdCount, err := t.buildTargetGroupHealthCheckUnhealthyThresholdCount(ctx, baseSvcAnnotations, t.defaultHealthCheckUnhealthyThresholdForInstanceModeLocal)
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

func (t *defaultModelBuildTask) buildTargetGroupName(_ context.Context, baseSvc *corev1.Service, svc *corev1.Service, svcPort intstr.IntOrString, tgPort int32,
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
	if baseSvc != nil {
		_, _ = uuidHash.Write([]byte(baseSvc.UID))
	}
	_, _ = uuidHash.Write([]byte(svc.UID))
	_, _ = uuidHash.Write([]byte(strconv.Itoa(int(tgPort))))
	_, _ = uuidHash.Write([]byte(svcPort.String()))
	_, _ = uuidHash.Write([]byte(targetType))
	_, _ = uuidHash.Write([]byte(tgProtocol))
	_, _ = uuidHash.Write([]byte(healthCheckProtocol))
	_, _ = uuidHash.Write([]byte(healthCheckInterval))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidTargetGroupNamePattern.ReplaceAllString(svc.Namespace, "")
	sanitizedName := invalidTargetGroupNamePattern.ReplaceAllString(svc.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (t *defaultModelBuildTask) buildPortSpecificTargetGroupAttributes(_ context.Context, port corev1.ServicePort) (map[string]string, error) {
	attributes := make(map[string]string)

	// Parse and validate base attributes first
	baseAttrs, err := t.validateAndParseAttributes(annotations.SvcLBSuffixTargetGroupAttributes)
	if err != nil {
		return nil, err
	}

	for k, v := range baseAttrs {
		attributes[k] = v
	}

	// Parse and validate port-specific attributes
	portAnnotation := fmt.Sprintf("%s.%d", annotations.SvcLBSuffixTargetGroupAttributes, port.Port)
	portAttrs, err := t.validateAndParseAttributes(portAnnotation)
	if err != nil {
		return nil, err
	}

	for k, v := range portAttrs {
		// If port-specific value is empty, keep the base value
		if v != "" {
			attributes[k] = v
		}
	}

	return attributes, nil
}

func (t *defaultModelBuildTask) validateAndParseAttributes(annotation string) (map[string]string, error) {
	var attrs map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotation, &attrs, t.service.Annotations); err != nil {
		return nil, err
	}
	if attrs == nil {
		return nil, nil
	}

	// Validate special attributes
	for k, v := range attrs {
		if k == shared_constants.TGAttributePreserveClientIPEnabled {
			if _, err := strconv.ParseBool(v); err != nil {
				return nil, errors.Wrapf(err, "failed to parse attribute %v=%v", k, v)
			}
		}
	}
	return attrs, nil
}

func (t *defaultModelBuildTask) buildTargetGroupAttributes(ctx context.Context, baseSvcAnnotations map[string]string, port corev1.ServicePort) ([]elbv2model.TargetGroupAttribute, error) {
	// Start with defaults
	rawAttributes := make(map[string]string)
	rawAttributes[shared_constants.TGAttributeProxyProtocolV2Enabled] = strconv.FormatBool(t.defaultProxyProtocolV2Enabled)

	// Get base and port-specific attributes
	baseAndPortAttributes, err := t.buildPortSpecificTargetGroupAttributes(ctx, port)
	if err != nil {
		return nil, err
	}

	for k, v := range baseAndPortAttributes {
		rawAttributes[k] = v
	}

	// Handle proxy protocol settings - these override any previous settings
	currentPortStr := strconv.FormatInt(int64(port.Port), 10)

	// Check proxy protocol per target group first
	var proxyProtocolPerTG string
	if t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixProxyProtocolPerTargetGroup, &proxyProtocolPerTG, baseSvcAnnotations) {
		ports := strings.Split(proxyProtocolPerTG, ",")
		enabledPorts := make(map[string]struct{})
		for _, p := range ports {
			trimmedPort := strings.TrimSpace(p)
			if trimmedPort != "" {
				if _, err := strconv.Atoi(trimmedPort); err != nil {
					return nil, errors.Errorf("invalid port number in proxy-protocol-per-target-group: %v", trimmedPort)
				}
				enabledPorts[trimmedPort] = struct{}{}
			}
		}

		if _, enabled := enabledPorts[currentPortStr]; enabled {
			rawAttributes[shared_constants.TGAttributeProxyProtocolV2Enabled] = "true"
		} else {
			rawAttributes[shared_constants.TGAttributeProxyProtocolV2Enabled] = "false"
		}
	}

	// Global proxy protocol override takes precedence over per-target group settings
	proxyV2Annotation := ""
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixProxyProtocol, &proxyV2Annotation, baseSvcAnnotations); exists {
		if proxyV2Annotation != "*" {
			return []elbv2model.TargetGroupAttribute{}, errors.Errorf("invalid value %v for Load Balancer proxy protocol v2 annotation, only value currently supported is *", proxyV2Annotation)
		}
		rawAttributes[shared_constants.TGAttributeProxyProtocolV2Enabled] = "true"
	}

	// Convert map to sorted array of attributes
	attributes := make([]elbv2model.TargetGroupAttribute, 0, len(rawAttributes))
	for attrKey, attrValue := range rawAttributes {
		// Special handling for empty values:
		// - Skip empty proxy protocol attribute (use default)
		// - Preserve other empty values as they may be intentional overrides
		if attrValue == "" && attrKey == shared_constants.TGAttributeProxyProtocolV2Enabled {
			continue
		}
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
		if attr.Key == shared_constants.TGAttributePreserveClientIPEnabled {
			preserveClientIP, err := strconv.ParseBool(attr.Value)
			if err != nil {
				return false, errors.Wrapf(err, "failed to parse attribute %v=%v", shared_constants.TGAttributePreserveClientIPEnabled, attr.Value)
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

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckPort(_ context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string, defaultHealthCheckPort string, targetType elbv2model.TargetType) (intstr.IntOrString, error) {
	rawHealthCheckPort := defaultHealthCheckPort
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPort, &rawHealthCheckPort, baseSvcAnnotations)
	if rawHealthCheckPort == shared_constants.HealthCheckPortTrafficPort {
		return intstr.FromString(rawHealthCheckPort), nil
	}
	healthCheckPort := intstr.Parse(rawHealthCheckPort)
	if healthCheckPort.Type == intstr.Int {
		return healthCheckPort, nil
	}

	svcPort, err := k8s.LookupServicePort(svc, healthCheckPort)
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

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckProtocol(_ context.Context, baseSvcAnnotations map[string]string, defaultHealthCheckProtocol elbv2model.Protocol) (elbv2model.Protocol, error) {
	rawHealthCheckProtocol := string(defaultHealthCheckProtocol)
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCProtocol, &rawHealthCheckProtocol, baseSvcAnnotations)
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

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckPath(_ context.Context, baseSvcAnnotations map[string]string, defaultHealthCheckPath string, hcProtocol elbv2model.Protocol) *string {
	if hcProtocol == elbv2model.ProtocolTCP {
		return nil
	}
	healthCheckPath := defaultHealthCheckPath
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPath, &healthCheckPath, baseSvcAnnotations)
	return &healthCheckPath
}
func (t *defaultModelBuildTask) buildTargetGroupHealthCheckMatcher(_ context.Context, baseSvcAnnotations map[string]string, hcProtocol elbv2model.Protocol) *elbv2model.HealthCheckMatcher {
	if hcProtocol == elbv2model.ProtocolTCP || !t.featureGates.Enabled(config.NLBHealthCheckAdvancedConfig) {
		return nil
	}
	rawHealthCheckMatcherSuccessCodes := t.defaultHealthCheckMatcherHTTPCode
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCSuccessCodes, &rawHealthCheckMatcherSuccessCodes, baseSvcAnnotations)
	return &elbv2model.HealthCheckMatcher{
		HTTPCode: &rawHealthCheckMatcherSuccessCodes,
	}
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckIntervalSeconds(_ context.Context, baseSvcAnnotations map[string]string, defaultHealthCheckInterval int32) (int32, error) {
	intervalSeconds := defaultHealthCheckInterval
	if _, err := t.annotationParser.ParseInt32Annotation(annotations.SvcLBSuffixHCInterval, &intervalSeconds, baseSvcAnnotations); err != nil {
		return 0, err
	}
	return intervalSeconds, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckTimeoutSeconds(_ context.Context, baseSvcAnnotations map[string]string, defaultHealthCheckTimeout int32) (*int32, error) {
	timeoutSeconds := defaultHealthCheckTimeout
	if !t.featureGates.Enabled(config.NLBHealthCheckAdvancedConfig) {
		return awssdk.Int32(timeoutSeconds), nil
	}
	if _, err := t.annotationParser.ParseInt32Annotation(annotations.SvcLBSuffixHCTimeout, &timeoutSeconds, baseSvcAnnotations); err != nil {
		return nil, err
	}
	return awssdk.Int32(timeoutSeconds), nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckHealthyThresholdCount(_ context.Context, baseSvcAnnotations map[string]string, defaultHealthCheckHealthyThreshold int32) (int32, error) {
	healthyThresholdCount := defaultHealthCheckHealthyThreshold
	if _, err := t.annotationParser.ParseInt32Annotation(annotations.SvcLBSuffixHCHealthyThreshold, &healthyThresholdCount, baseSvcAnnotations); err != nil {
		return 0, err
	}
	return healthyThresholdCount, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckUnhealthyThresholdCount(_ context.Context, baseSvcAnnotations map[string]string, defaultHealthCheckUnhealthyThreshold int32) (int32, error) {
	unhealthyThresholdCount := defaultHealthCheckUnhealthyThreshold
	if _, err := t.annotationParser.ParseInt32Annotation(annotations.SvcLBSuffixHCUnhealthyThreshold, &unhealthyThresholdCount, baseSvcAnnotations); err != nil {
		return 0, err
	}
	return unhealthyThresholdCount, nil
}

func (t *defaultModelBuildTask) buildTargetType(_ context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string, port corev1.ServicePort) (elbv2model.TargetType, error) {
	svcType := svc.Spec.Type
	var lbType string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixLoadBalancerType, &lbType, baseSvcAnnotations)
	var lbTargetType string
	lbTargetType = string(t.defaultTargetType)
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixTargetType, &lbTargetType, baseSvcAnnotations)
	if lbTargetType == LoadBalancerTargetTypeIP && !t.enableIPTargetType {
		return "", errors.Errorf("unsupported targetType: %v when EnableIPTargetType is %v", lbTargetType, t.enableIPTargetType)
	}
	if lbType == LoadBalancerTypeNLBIP || lbTargetType == LoadBalancerTargetTypeIP {
		return elbv2model.TargetTypeIP, nil
	}
	if svcType == corev1.ServiceTypeClusterIP {
		return "", errors.Errorf("unsupported service type \"%v\" for load balancer target type \"%v\"", svcType, lbTargetType)
	}
	if port.NodePort == 0 && svc.Spec.AllocateLoadBalancerNodePorts != nil && !*svc.Spec.AllocateLoadBalancerNodePorts {
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

func (t *defaultModelBuildTask) buildTargetGroupBinding(ctx context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string, targetGroup *elbv2model.TargetGroup,
	port corev1.ServicePort, hc *elbv2model.TargetGroupHealthCheckConfig, scheme elbv2model.LoadBalancerScheme) (*elbv2modelk8s.TargetGroupBindingResource, error) {
	tgbSpec, err := t.buildTargetGroupBindingSpec(ctx, svc, baseSvcAnnotations, targetGroup, port, hc, scheme)
	if err != nil {
		return nil, err
	}
	return elbv2modelk8s.NewTargetGroupBindingResource(t.stack, targetGroup.ID(), tgbSpec), nil
}

func (t *defaultModelBuildTask) buildTargetGroupBindingSpec(ctx context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string, targetGroup *elbv2model.TargetGroup,
	port corev1.ServicePort, hc *elbv2model.TargetGroupHealthCheckConfig, scheme elbv2model.LoadBalancerScheme) (elbv2modelk8s.TargetGroupBindingResourceSpec, error) {
	nodeSelector, err := t.buildTargetGroupBindingNodeSelector(ctx, baseSvcAnnotations, targetGroup.Spec.TargetType)
	if err != nil {
		return elbv2modelk8s.TargetGroupBindingResourceSpec{}, err
	}
	targetPort := port.TargetPort
	targetType := elbv2api.TargetType(targetGroup.Spec.TargetType)
	if targetType == elbv2api.TargetTypeInstance {
		targetPort = intstr.FromInt(int(port.NodePort))
	}
	var tgbNetworking *elbv2modelk8s.TargetGroupBindingNetworking
	if len(t.loadBalancer.Spec.SecurityGroups) == 0 {
		tgbNetworking, err = t.buildTargetGroupBindingNetworkingLegacy(ctx, svc, baseSvcAnnotations, targetPort, targetGroup.Spec.Protocol, *hc.Port, scheme, targetGroup.Spec.IPAddressType)
	} else {
		tgbNetworking, err = t.buildTargetGroupBindingNetworking(ctx, targetPort, *hc.Port, targetGroup.Spec.Protocol)
	}
	if err != nil {
		return elbv2modelk8s.TargetGroupBindingResourceSpec{}, err
	}

	multiTg, err := t.buildTargetGroupBindingMultiClusterFlag(baseSvcAnnotations)
	if err != nil {
		return elbv2modelk8s.TargetGroupBindingResourceSpec{}, err
	}

	return elbv2modelk8s.TargetGroupBindingResourceSpec{
		Template: elbv2modelk8s.TargetGroupBindingTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: svc.Namespace,
				Name:      targetGroup.Spec.Name,
			},
			Spec: elbv2modelk8s.TargetGroupBindingSpec{
				TargetGroupARN: targetGroup.TargetGroupARN(),
				TargetType:     &targetType,
				ServiceRef: elbv2api.ServiceReference{
					Name: svc.Name,
					Port: intstr.FromInt(int(port.Port)),
				},
				Networking:              tgbNetworking,
				NodeSelector:            nodeSelector,
				IPAddressType:           elbv2api.TargetGroupIPAddressType(targetGroup.Spec.IPAddressType),
				VpcID:                   t.vpcID,
				MultiClusterTargetGroup: multiTg,
				TargetGroupProtocol:     &targetGroup.Spec.Protocol,
			},
		},
	}, nil
}

func (t *defaultModelBuildTask) buildTargetGroupBindingNetworking(_ context.Context, tgPort intstr.IntOrString,
	hcPort intstr.IntOrString, tgProtocol elbv2model.Protocol) (*elbv2modelk8s.TargetGroupBindingNetworking, error) {

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
		if tgProtocol == elbv2model.ProtocolUDP || tgProtocol == elbv2model.ProtocolTCP_UDP || tgProtocol == elbv2model.ProtocolQUIC || tgProtocol == elbv2model.ProtocolTCP_QUIC {
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolUDP,
				Port:     nil,
			})
		}
	} else {
		switch tgProtocol {
		case elbv2model.ProtocolTLS:
			fallthrough
		case elbv2model.ProtocolTCP:
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolTCP,
				Port:     &tgPort,
			})
		case elbv2model.ProtocolQUIC:
			fallthrough
		case elbv2model.ProtocolTCP_QUIC:
			fallthrough
		case elbv2model.ProtocolTCP_UDP:
			fallthrough
		case elbv2model.ProtocolUDP:
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolUDP,
				Port:     &tgPort,
			})
			if tgProtocol == elbv2model.ProtocolTCP_UDP || tgProtocol == elbv2model.ProtocolTCP_QUIC || hcPort.String() == shared_constants.HealthCheckPortTrafficPort || (hcPort.Type == intstr.Int && hcPort.IntValue() == tgPort.IntValue()) {
				ports = append(ports, elbv2api.NetworkingPort{
					Protocol: &protocolTCP,
					Port:     &tgPort,
				})
			}
		}

		if hcPort.String() != shared_constants.HealthCheckPortTrafficPort && (hcPort.Type == intstr.Int && hcPort.IntValue() != tgPort.IntValue()) {
			ports = append(ports, elbv2api.NetworkingPort{
				Protocol: &protocolTCP,
				Port:     &hcPort,
			})
		}
	}
	return &elbv2modelk8s.TargetGroupBindingNetworking{
		Ingress: []elbv2modelk8s.NetworkingIngressRule{
			{
				From: []elbv2modelk8s.NetworkingPeer{
					{
						SecurityGroup: &elbv2modelk8s.SecurityGroup{
							GroupID: t.backendSGIDToken,
						},
					},
				},
				Ports: ports,
			},
		},
	}, nil
}

func (t *defaultModelBuildTask) getLoadBalancerSourceRanges(_ context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string) []string {
	var sourceRanges []string
	for _, cidr := range svc.Spec.LoadBalancerSourceRanges {
		sourceRanges = append(sourceRanges, cidr)
	}
	if len(sourceRanges) == 0 {
		t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSourceRanges, &sourceRanges, baseSvcAnnotations)
	}
	return sourceRanges
}

func (t *defaultModelBuildTask) buildPeersFromSourceRangeCIDRs(_ context.Context, sourceRanges []string) []elbv2modelk8s.NetworkingPeer {
	var peers []elbv2modelk8s.NetworkingPeer
	for _, cidr := range sourceRanges {
		peers = append(peers, elbv2modelk8s.NetworkingPeer{
			IPBlock: &elbv2api.IPBlock{
				CIDR: cidr,
			},
		})
	}
	return peers
}

func (t *defaultModelBuildTask) buildTargetGroupBindingNetworkingLegacy(ctx context.Context, svc *corev1.Service, baseSvcAnnotations map[string]string, tgPort intstr.IntOrString, tgProtocol elbv2model.Protocol,
	hcPort intstr.IntOrString, scheme elbv2model.LoadBalancerScheme, targetGroupIPAddressType elbv2model.TargetGroupIPAddressType) (*elbv2modelk8s.TargetGroupBindingNetworking, error) {
	manageBackendSGRules, err := t.buildManageSecurityGroupRulesFlagLegacy(ctx, baseSvcAnnotations)
	if err != nil {
		return nil, err
	}
	if !manageBackendSGRules {
		return nil, nil
	}
	healthCheckProtocol := elbv2api.NetworkingProtocolTCP

	loadBalancerSubnetCIDRs := t.getLoadBalancerSubnetsSourceRanges(targetGroupIPAddressType)
	trafficSource := loadBalancerSubnetCIDRs
	defaultRangeUsed := false
	var trafficPorts []elbv2api.NetworkingPort

	/*
		https://docs.aws.amazon.com/elasticloadbalancing/latest/network/edit-target-group-attributes.html#client-ip-preservation
		By default, client IP preservation is enabled (and can't be disabled) for instance and IP type target groups with UDP, QUIC, TCP_QUIC, and TCP_UDP protocols.
		However, you can enable or disable client IP preservation for TCP and TLS target groups using the preserve_client_ip.enabled target group attribute.
	*/

	if tgProtocol == elbv2model.ProtocolTCP_UDP || tgProtocol == elbv2model.ProtocolTCP_QUIC || tgProtocol == elbv2model.ProtocolUDP || tgProtocol == elbv2model.ProtocolQUIC || t.preserveClientIP {
		trafficSource = t.getLoadBalancerSourceRanges(ctx, svc, baseSvcAnnotations)
		if len(trafficSource) == 0 {
			trafficSource, err = t.getDefaultIPSourceRanges(ctx, targetGroupIPAddressType, tgProtocol, scheme)
			if err != nil {
				return nil, err
			}
			defaultRangeUsed = true
		}
	}

	if tgProtocol == elbv2model.ProtocolTCP_UDP || tgProtocol == elbv2model.ProtocolTCP_QUIC {
		tcpProtocol := elbv2api.NetworkingProtocolTCP
		udpProtocol := elbv2api.NetworkingProtocolUDP
		trafficPorts = []elbv2api.NetworkingPort{
			{
				Port:     &tgPort,
				Protocol: &tcpProtocol,
			},
			{
				Port:     &tgPort,
				Protocol: &udpProtocol,
			},
		}
	} else {
		networkingProtocol := elbv2api.NetworkingProtocolTCP
		if tgProtocol == elbv2model.ProtocolUDP || tgProtocol == elbv2model.ProtocolQUIC {
			networkingProtocol = elbv2api.NetworkingProtocolUDP
		}

		trafficPorts = []elbv2api.NetworkingPort{
			{
				Port:     &tgPort,
				Protocol: &networkingProtocol,
			},
		}
	}
	tgbNetworking := &elbv2modelk8s.TargetGroupBindingNetworking{
		Ingress: []elbv2modelk8s.NetworkingIngressRule{
			{
				From:  t.buildPeersFromSourceRangeCIDRs(ctx, trafficSource),
				Ports: trafficPorts,
			},
		},
	}
	if healthCheckSourceCIDRs := t.buildHealthCheckSourceCIDRs(trafficSource, loadBalancerSubnetCIDRs, tgPort, hcPort,
		tgProtocol, defaultRangeUsed); len(healthCheckSourceCIDRs) > 0 {
		networkingHealthCheckPort := hcPort
		if hcPort.String() == shared_constants.HealthCheckPortTrafficPort {
			networkingHealthCheckPort = tgPort
		}
		tgbNetworking.Ingress = append(tgbNetworking.Ingress, elbv2modelk8s.NetworkingIngressRule{
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
	tgProtocol elbv2model.Protocol, scheme elbv2model.LoadBalancerScheme) ([]string, error) {
	defaultSourceRanges := t.defaultIPv4SourceRanges
	if targetGroupIPAddressType == elbv2model.TargetGroupIPAddressTypeIPv6 {
		defaultSourceRanges = t.defaultIPv6SourceRanges
	}
	if (tgProtocol == elbv2model.ProtocolQUIC || tgProtocol == elbv2model.ProtocolTCP_QUIC || tgProtocol == elbv2model.ProtocolTCP_UDP || tgProtocol == elbv2model.ProtocolUDP || t.preserveClientIP) && scheme == elbv2model.LoadBalancerSchemeInternal {
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
		if elbv2model.IPAddressTypeDualStack != t.loadBalancer.Spec.IPAddressType { // todo khelle
			return "", errors.New("unsupported IPv6 configuration, lb not dual-stack")
		}
		return elbv2model.TargetGroupIPAddressTypeIPv6, nil
	}
	return elbv2model.TargetGroupIPAddressTypeIPv4, nil
}

func (t *defaultModelBuildTask) buildTargetGroupBindingNodeSelector(_ context.Context, baseSvcAnnotations map[string]string, targetType elbv2model.TargetType) (*metav1.LabelSelector, error) {
	if targetType != elbv2model.TargetTypeInstance {
		return nil, nil
	}
	var targetNodeLabels map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixTargetNodeLabels, &targetNodeLabels, baseSvcAnnotations); err != nil {
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
	tgProtocol elbv2model.Protocol, defaultRangeUsed bool) []string {
	if tgProtocol != elbv2model.ProtocolUDP &&
		(hcPort.String() == shared_constants.HealthCheckPortTrafficPort || hcPort.IntValue() == tgPort.IntValue()) {
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

func (t *defaultModelBuildTask) buildManageSecurityGroupRulesFlagLegacy(_ context.Context, baseSvcAnnotations map[string]string) (bool, error) {
	var rawEnabled bool
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixManageSGRules, &rawEnabled, baseSvcAnnotations)
	if err != nil {
		return true, err
	}
	if exists {
		return rawEnabled, nil
	}
	return true, nil
}

func (t *defaultModelBuildTask) buildTargetGroupBindingMultiClusterFlag(baseSvcAnnotations map[string]string) (bool, error) {
	var rawEnabled bool
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixMultiClusterTargetGroup, &rawEnabled, baseSvcAnnotations)
	if err != nil {
		return false, err
	}
	if exists {
		return rawEnabled, nil
	}
	return false, nil
}
