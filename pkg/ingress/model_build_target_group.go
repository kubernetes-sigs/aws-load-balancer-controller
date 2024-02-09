package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

const (
	healthCheckPortTrafficPort = "traffic-port"
)

func (t *defaultModelBuildTask) buildTargetGroup(ctx context.Context,
	ing ClassifiedIngress, svc *corev1.Service, port intstr.IntOrString) (*elbv2model.TargetGroup, error) {
	tgResID := t.buildTargetGroupResourceID(k8s.NamespacedName(ing.Ing), k8s.NamespacedName(svc), port)
	if tg, exists := t.tgByResID[tgResID]; exists {
		return tg, nil
	}
	svcPort, err := k8s.LookupServicePort(svc, port)
	if err != nil {
		return nil, err
	}
	tgSpec, err := t.buildTargetGroupSpec(ctx, ing, svc, port, svcPort)
	if err != nil {
		return nil, err
	}
	nodeSelector, err := t.buildTargetGroupBindingNodeSelector(ctx, ing, svc, tgSpec.TargetType)
	if err != nil {
		return nil, err
	}
	tg := elbv2model.NewTargetGroup(t.stack, tgResID, tgSpec)
	t.tgByResID[tgResID] = tg
	_ = t.buildTargetGroupBinding(ctx, tg, svc, port, svcPort, nodeSelector)
	return tg, nil
}

func (t *defaultModelBuildTask) buildTargetGroupBinding(ctx context.Context, tg *elbv2model.TargetGroup, svc *corev1.Service, port intstr.IntOrString, svcPort corev1.ServicePort, nodeSelector *metav1.LabelSelector) *elbv2model.TargetGroupBindingResource {
	tgbSpec := t.buildTargetGroupBindingSpec(ctx, tg, svc, port, svcPort, nodeSelector)
	tgb := elbv2model.NewTargetGroupBindingResource(t.stack, tg.ID(), tgbSpec)
	return tgb
}

func (t *defaultModelBuildTask) buildTargetGroupBindingSpec(ctx context.Context, tg *elbv2model.TargetGroup, svc *corev1.Service, port intstr.IntOrString, svcPort corev1.ServicePort, nodeSelector *metav1.LabelSelector) elbv2model.TargetGroupBindingResourceSpec {
	targetType := elbv2api.TargetType(tg.Spec.TargetType)
	targetPort := svcPort.TargetPort
	if targetType == elbv2api.TargetTypeInstance {
		targetPort = intstr.FromInt(int(svcPort.NodePort))
	}
	tgbNetworking := t.buildTargetGroupBindingNetworking(ctx, targetPort, *tg.Spec.HealthCheckConfig.Port)
	return elbv2model.TargetGroupBindingResourceSpec{
		Template: elbv2model.TargetGroupBindingTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: svc.Namespace,
				Name:      tg.Spec.Name,
			},
			Spec: elbv2model.TargetGroupBindingSpec{
				TargetGroupARN: tg.TargetGroupARN(),
				TargetType:     &targetType,
				ServiceRef: elbv2api.ServiceReference{
					Name: svc.Name,
					Port: port,
				},
				Networking:    tgbNetworking,
				NodeSelector:  nodeSelector,
				IPAddressType: (*elbv2api.TargetGroupIPAddressType)(tg.Spec.IPAddressType),
			},
		},
	}
}

func (t *defaultModelBuildTask) buildTargetGroupBindingNetworking(ctx context.Context, targetPort intstr.IntOrString, healthCheckPort intstr.IntOrString) *elbv2model.TargetGroupBindingNetworking {
	if t.backendSGIDToken == nil {
		return nil
	}
	protocolTCP := elbv2api.NetworkingProtocolTCP
	if t.disableRestrictedSGRules {
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
					Ports: []elbv2api.NetworkingPort{
						{
							Protocol: &protocolTCP,
							Port:     nil,
						},
					},
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
	if healthCheckPort.String() != healthCheckPortTrafficPort {
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
						GroupID: t.backendSGIDToken,
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

func (t *defaultModelBuildTask) buildTargetGroupSpec(ctx context.Context,
	ing ClassifiedIngress, svc *corev1.Service, port intstr.IntOrString, svcPort corev1.ServicePort) (elbv2model.TargetGroupSpec, error) {
	svcAndIngAnnotations := algorithm.MergeStringMap(svc.Annotations, ing.Ing.Annotations)
	targetType, err := t.buildTargetGroupTargetType(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgProtocol, err := t.buildTargetGroupProtocol(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgProtocolVersion, err := t.buildTargetGroupProtocolVersion(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	healthCheckConfig, err := t.buildTargetGroupHealthCheckConfig(ctx, svc, svcAndIngAnnotations, targetType, tgProtocol, tgProtocolVersion)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgAttributes, err := t.buildTargetGroupAttributes(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tags, err := t.buildTargetGroupTags(ctx, ing, svc)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	ipAddressType, err := t.buildTargetGroupIPAddressType(ctx, svc)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgPort := t.buildTargetGroupPort(ctx, targetType, svcPort)
	name := t.buildTargetGroupName(ctx, k8s.NamespacedName(ing.Ing), svc, port, tgPort, targetType, tgProtocol, tgProtocolVersion)
	return elbv2model.TargetGroupSpec{
		Name:                  name,
		TargetType:            targetType,
		Port:                  tgPort,
		Protocol:              tgProtocol,
		ProtocolVersion:       &tgProtocolVersion,
		IPAddressType:         &ipAddressType,
		HealthCheckConfig:     &healthCheckConfig,
		TargetGroupAttributes: tgAttributes,
		Tags:                  tags,
	}, nil
}

var invalidTargetGroupNamePattern = regexp.MustCompile("[[:^alnum:]]")

// buildTargetGroupName will calculate the targetGroup's name.
func (t *defaultModelBuildTask) buildTargetGroupName(_ context.Context,
	ingKey types.NamespacedName, svc *corev1.Service, port intstr.IntOrString, tgPort int64,
	targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol, tgProtocolVersion elbv2model.ProtocolVersion) string {
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.ingGroup.ID.String()))
	_, _ = uuidHash.Write([]byte(ingKey.Namespace))
	_, _ = uuidHash.Write([]byte(ingKey.Name))
	_, _ = uuidHash.Write([]byte(svc.UID))
	_, _ = uuidHash.Write([]byte(port.String()))
	_, _ = uuidHash.Write([]byte(strconv.Itoa(int(tgPort))))
	_, _ = uuidHash.Write([]byte(targetType))
	_, _ = uuidHash.Write([]byte(tgProtocol))
	_, _ = uuidHash.Write([]byte(tgProtocolVersion))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidTargetGroupNamePattern.ReplaceAllString(svc.Namespace, "")
	sanitizedName := invalidTargetGroupNamePattern.ReplaceAllString(svc.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (t *defaultModelBuildTask) buildTargetGroupTargetType(_ context.Context, svcAndIngAnnotations map[string]string) (elbv2model.TargetType, error) {
	rawTargetType := string(t.defaultTargetType)
	_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixTargetType, &rawTargetType, svcAndIngAnnotations)
	switch rawTargetType {
	case string(elbv2model.TargetTypeInstance):
		return elbv2model.TargetTypeInstance, nil
	case string(elbv2model.TargetTypeIP):
		if !t.enableIPTargetType {
			return "", errors.Errorf("unsupported targetType: %v when EnableIPTargetType is %v", rawTargetType, t.enableIPTargetType)
		}
		return elbv2model.TargetTypeIP, nil
	default:
		return "", errors.Errorf("unknown targetType: %v", rawTargetType)
	}
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
		if *t.loadBalancer.Spec.IPAddressType != elbv2model.IPAddressTypeDualStack {
			return "", errors.New("unsupported IPv6 configuration, lb not dual-stack")
		}
		return elbv2model.TargetGroupIPAddressTypeIPv6, nil
	}
	return elbv2model.TargetGroupIPAddressTypeIPv4, nil
}

// buildTargetGroupPort constructs the TargetGroup's port.
// Note: TargetGroup's port is not in the data path as we always register targets with port specified.
// so this settings don't really matter to our controller, and we do our best to use the most appropriate port as targetGroup's port to avoid UX confusing.
func (t *defaultModelBuildTask) buildTargetGroupPort(_ context.Context, targetType elbv2model.TargetType, svcPort corev1.ServicePort) int64 {
	if targetType == elbv2model.TargetTypeInstance {
		return int64(svcPort.NodePort)
	}
	if svcPort.TargetPort.Type == intstr.Int {
		return int64(svcPort.TargetPort.IntValue())
	}

	// when a literal targetPort is used, we just use a fixed 1 here as this setting is not in the data path.
	// also, under extreme edge case, it can actually be different ports for different pods.
	return 1
}

func (t *defaultModelBuildTask) buildTargetGroupProtocol(_ context.Context, svcAndIngAnnotations map[string]string) (elbv2model.Protocol, error) {
	rawBackendProtocol := string(t.defaultBackendProtocol)
	_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixBackendProtocol, &rawBackendProtocol, svcAndIngAnnotations)
	switch rawBackendProtocol {
	case string(elbv2model.ProtocolHTTP):
		return elbv2model.ProtocolHTTP, nil
	case string(elbv2model.ProtocolHTTPS):
		return elbv2model.ProtocolHTTPS, nil
	default:
		return "", errors.Errorf("backend protocol must be within [%v, %v]: %v", elbv2model.ProtocolHTTP, elbv2model.ProtocolHTTPS, rawBackendProtocol)
	}
}

func (t *defaultModelBuildTask) buildTargetGroupProtocolVersion(_ context.Context, svcAndIngAnnotations map[string]string) (elbv2model.ProtocolVersion, error) {
	rawBackendProtocolVersion := string(t.defaultBackendProtocolVersion)
	_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixBackendProtocolVersion, &rawBackendProtocolVersion, svcAndIngAnnotations)
	switch rawBackendProtocolVersion {
	case string(elbv2model.ProtocolVersionHTTP1):
		return elbv2model.ProtocolVersionHTTP1, nil
	case string(elbv2model.ProtocolVersionHTTP2):
		return elbv2model.ProtocolVersionHTTP2, nil
	case string(elbv2model.ProtocolVersionGRPC):
		return elbv2model.ProtocolVersionGRPC, nil
	default:
		return "", errors.Errorf("backend protocol version must be within [%v, %v, %v]: %v", elbv2model.ProtocolVersionHTTP1, elbv2model.ProtocolVersionHTTP2, elbv2model.ProtocolVersionGRPC, rawBackendProtocolVersion)
	}
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckConfig(ctx context.Context, svc *corev1.Service, svcAndIngAnnotations map[string]string, targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol, tgProtocolVersion elbv2model.ProtocolVersion) (elbv2model.TargetGroupHealthCheckConfig, error) {
	healthCheckPort, err := t.buildTargetGroupHealthCheckPort(ctx, svc, svcAndIngAnnotations, targetType)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckProtocol, err := t.buildTargetGroupHealthCheckProtocol(ctx, svcAndIngAnnotations, tgProtocol)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckPath := t.buildTargetGroupHealthCheckPath(ctx, svcAndIngAnnotations, tgProtocolVersion)
	healthCheckMatcher := t.buildTargetGroupHealthCheckMatcher(ctx, svcAndIngAnnotations, tgProtocolVersion)
	healthCheckIntervalSeconds, err := t.buildTargetGroupHealthCheckIntervalSeconds(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckTimeoutSeconds, err := t.buildTargetGroupHealthCheckTimeoutSeconds(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckHealthyThresholdCount, err := t.buildTargetGroupHealthCheckHealthyThresholdCount(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckUnhealthyThresholdCount, err := t.buildTargetGroupHealthCheckUnhealthyThresholdCount(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	return elbv2model.TargetGroupHealthCheckConfig{
		Port:                    &healthCheckPort,
		Protocol:                &healthCheckProtocol,
		Path:                    &healthCheckPath,
		Matcher:                 &healthCheckMatcher,
		IntervalSeconds:         &healthCheckIntervalSeconds,
		TimeoutSeconds:          &healthCheckTimeoutSeconds,
		HealthyThresholdCount:   &healthCheckHealthyThresholdCount,
		UnhealthyThresholdCount: &healthCheckUnhealthyThresholdCount,
	}, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckPort(_ context.Context, svc *corev1.Service, svcAndIngAnnotations map[string]string, targetType elbv2model.TargetType) (intstr.IntOrString, error) {
	rawHealthCheckPort := ""
	if exist := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixHealthCheckPort, &rawHealthCheckPort, svcAndIngAnnotations); !exist {
		return intstr.FromString(healthCheckPortTrafficPort), nil
	}
	if rawHealthCheckPort == healthCheckPortTrafficPort {
		return intstr.FromString(healthCheckPortTrafficPort), nil
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

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckProtocol(_ context.Context, svcAndIngAnnotations map[string]string, tgProtocol elbv2model.Protocol) (elbv2model.Protocol, error) {
	rawHealthCheckProtocol := string(tgProtocol)
	_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixHealthCheckProtocol, &rawHealthCheckProtocol, svcAndIngAnnotations)
	switch rawHealthCheckProtocol {
	case string(elbv2model.ProtocolHTTP):
		return elbv2model.ProtocolHTTP, nil
	case string(elbv2model.ProtocolHTTPS):
		return elbv2model.ProtocolHTTPS, nil
	default:
		return "", errors.Errorf("healthCheckProtocol must be within [%v, %v]", elbv2model.ProtocolHTTP, elbv2model.ProtocolHTTPS)
	}
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckPath(_ context.Context, svcAndIngAnnotations map[string]string, tgProtocolVersion elbv2model.ProtocolVersion) string {
	var rawHealthCheckPath string
	switch tgProtocolVersion {
	case elbv2model.ProtocolVersionHTTP1, elbv2model.ProtocolVersionHTTP2:
		rawHealthCheckPath = t.defaultHealthCheckPathHTTP
	case elbv2model.ProtocolVersionGRPC:
		rawHealthCheckPath = t.defaultHealthCheckPathGRPC
	}
	_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixHealthCheckPath, &rawHealthCheckPath, svcAndIngAnnotations)
	return rawHealthCheckPath
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckMatcher(_ context.Context, svcAndIngAnnotations map[string]string, tgProtocolVersion elbv2model.ProtocolVersion) elbv2model.HealthCheckMatcher {
	var rawHealthCheckMatcherHTTPCode string
	switch tgProtocolVersion {
	case elbv2model.ProtocolVersionHTTP1, elbv2model.ProtocolVersionHTTP2:
		rawHealthCheckMatcherHTTPCode = t.defaultHealthCheckMatcherHTTPCode
	case elbv2model.ProtocolVersionGRPC:
		rawHealthCheckMatcherHTTPCode = t.defaultHealthCheckMatcherGRPCCode
	}

	_ = t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixSuccessCodes, &rawHealthCheckMatcherHTTPCode, svcAndIngAnnotations)
	if tgProtocolVersion == elbv2model.ProtocolVersionGRPC {
		return elbv2model.HealthCheckMatcher{
			GRPCCode: &rawHealthCheckMatcherHTTPCode,
		}
	}
	return elbv2model.HealthCheckMatcher{
		HTTPCode: &rawHealthCheckMatcherHTTPCode,
	}
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckIntervalSeconds(_ context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawHealthCheckIntervalSeconds := t.defaultHealthCheckIntervalSeconds
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.IngressSuffixHealthCheckIntervalSeconds,
		&rawHealthCheckIntervalSeconds, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawHealthCheckIntervalSeconds, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckTimeoutSeconds(_ context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawHealthCheckTimeoutSeconds := t.defaultHealthCheckTimeoutSeconds
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.IngressSuffixHealthCheckTimeoutSeconds,
		&rawHealthCheckTimeoutSeconds, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawHealthCheckTimeoutSeconds, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckHealthyThresholdCount(_ context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawHealthCheckHealthyThresholdCount := t.defaultHealthCheckHealthyThresholdCount
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.IngressSuffixHealthyThresholdCount,
		&rawHealthCheckHealthyThresholdCount, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawHealthCheckHealthyThresholdCount, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckUnhealthyThresholdCount(_ context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawHealthCheckUnhealthyThresholdCount := t.defaultHealthCheckUnhealthyThresholdCount
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.IngressSuffixUnhealthyThresholdCount,
		&rawHealthCheckUnhealthyThresholdCount, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawHealthCheckUnhealthyThresholdCount, nil
}

func (t *defaultModelBuildTask) buildTargetGroupAttributes(_ context.Context, svcAndIngAnnotations map[string]string) ([]elbv2model.TargetGroupAttribute, error) {
	var rawAttributes map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTargetGroupAttributes, &rawAttributes, svcAndIngAnnotations); err != nil {
		return nil, err
	}
	attributes := make([]elbv2model.TargetGroupAttribute, 0, len(rawAttributes))
	for attrKey, attrValue := range rawAttributes {
		attributes = append(attributes, elbv2model.TargetGroupAttribute{
			Key:   attrKey,
			Value: attrValue,
		})
	}
	return attributes, nil
}

func (t *defaultModelBuildTask) buildTargetGroupTags(_ context.Context, ing ClassifiedIngress, svc *corev1.Service) (map[string]string, error) {
	ingSvcTags, err := t.buildIngressBackendResourceTags(ing, svc)
	if err != nil {
		return nil, err
	}
	return algorithm.MergeStringMap(t.defaultTags, ingSvcTags), nil
}

func (t *defaultModelBuildTask) buildTargetGroupResourceID(ingKey types.NamespacedName, svcKey types.NamespacedName, port intstr.IntOrString) string {
	return fmt.Sprintf("%s/%s-%s:%s", ingKey.Namespace, ingKey.Name, svcKey.Name, port.String())
}

func (t *defaultModelBuildTask) buildTargetGroupBindingNodeSelector(_ context.Context, ing ClassifiedIngress, svc *corev1.Service, targetType elbv2model.TargetType) (*metav1.LabelSelector, error) {
	if targetType != elbv2model.TargetTypeInstance {
		return nil, nil
	}
	var targetNodeLabels map[string]string
	svcAndIngAnnotations := algorithm.MergeStringMap(svc.Annotations, ing.Ing.Annotations)

	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTargetNodeLabels, &targetNodeLabels, svcAndIngAnnotations); err != nil {
		return nil, err
	}
	if len(targetNodeLabels) == 0 {
		return nil, nil
	}
	return &metav1.LabelSelector{
		MatchLabels: targetNodeLabels,
	}, nil
}
