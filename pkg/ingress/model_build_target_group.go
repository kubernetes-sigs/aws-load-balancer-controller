package ingress

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/algorithm"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
)

const (
	healthCheckPortTrafficPort = "traffic-port"
)

func (b *defaultModelBuilder) buildTargetGroup(ctx context.Context, stack core.Stack, ingGroupID GroupID,
	tgByID map[string]*elbv2model.TargetGroup, ing *networking.Ingress, svc *corev1.Service, port intstr.IntOrString) (*elbv2model.TargetGroup, error) {
	tgResID := b.buildTargetGroupResourceID(k8s.NamespacedName(ing), k8s.NamespacedName(svc), port)
	if tg, exists := tgByID[tgResID]; exists {
		return tg, nil
	}
	tgSpec, err := b.buildTargetGroupSpec(ctx, ingGroupID, ing, svc, port)
	if err != nil {
		return nil, err
	}
	tg := elbv2model.NewTargetGroup(stack, tgResID, tgSpec)
	tgByID[tgResID] = tg
	targetType := elbv2api.TargetType(tgSpec.TargetType)
	_ = elbv2model.NewTargetGroupBindingResource(stack, tgResID, elbv2model.TargetGroupBindingResourceSpec{
		TargetGroupARN: tg.TargetGroupARN(),
		Template: elbv2model.TargetGroupBindingTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: svc.Namespace,
				Name:      tgSpec.Name,
			},
			Spec: elbv2api.TargetGroupBindingSpec{
				TargetType: &targetType,
				ServiceRef: elbv2api.ServiceReference{
					Name: svc.Name,
					Port: port,
				},
			},
		},
	})
	return tg, nil
}

func (b *defaultModelBuilder) buildTargetGroupSpec(ctx context.Context, ingGroupID GroupID,
	ing *networking.Ingress, svc *corev1.Service, port intstr.IntOrString) (elbv2model.TargetGroupSpec, error) {
	svcAndIngAnnotations := algorithm.MergeStringMap(svc.Annotations, ing.Annotations)
	targetType, err := b.buildTargetGroupTargetType(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tgProtocol, err := b.buildTargetGroupProtocol(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	healthCheckConfig, err := b.buildTargetGroupHealthCheckConfig(ctx, svc, svcAndIngAnnotations, targetType, tgProtocol)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	targetGroupAttributes, err := b.buildTargetGroupAttributes(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	tags, err := b.buildTargetGroupTags(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupSpec{}, err
	}
	name := b.buildTargetGroupName(ctx, ingGroupID, k8s.NamespacedName(ing), k8s.NamespacedName(svc), port, targetType, tgProtocol)
	return elbv2model.TargetGroupSpec{
		Name:                  name,
		TargetType:            targetType,
		Port:                  1,
		Protocol:              tgProtocol,
		HealthCheckConfig:     &healthCheckConfig,
		TargetGroupAttributes: targetGroupAttributes,
		Tags:                  tags,
	}, nil
}

func (b *defaultModelBuilder) buildTargetGroupName(ctx context.Context, ingGroupID GroupID,
	ingKey types.NamespacedName, svcKey types.NamespacedName, port intstr.IntOrString,
	targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol) string {
	uuidHash := md5.New()
	_, _ = uuidHash.Write([]byte(b.clusterName))
	_, _ = uuidHash.Write([]byte(ingGroupID.String()))
	_, _ = uuidHash.Write([]byte(ingKey.Namespace))
	_, _ = uuidHash.Write([]byte(ingKey.Name))
	_, _ = uuidHash.Write([]byte(svcKey.Name))
	_, _ = uuidHash.Write([]byte(port.String()))
	_, _ = uuidHash.Write([]byte(targetType))
	_, _ = uuidHash.Write([]byte(tgProtocol))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", svcKey.Namespace, svcKey.Name, uuid)
}

func (b *defaultModelBuilder) buildTargetGroupTargetType(ctx context.Context, svcAndIngAnnotations map[string]string) (elbv2model.TargetType, error) {
	rawTargetType := string(b.defaultTargetType)
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixTargetType, &rawTargetType, svcAndIngAnnotations)
	switch rawTargetType {
	case string(elbv2model.TargetTypeInstance):
		return elbv2model.TargetTypeInstance, nil
	case string(elbv2model.TargetTypeIP):
		return elbv2model.TargetTypeIP, nil
	default:
		return "", errors.Errorf("unknown targetType: %v", rawTargetType)
	}
}

func (b *defaultModelBuilder) buildTargetGroupProtocol(ctx context.Context, svcAndIngAnnotations map[string]string) (elbv2model.Protocol, error) {
	rawBackendProtocol := string(b.defaultBackendProtocol)
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixBackendProtocol, &rawBackendProtocol, svcAndIngAnnotations)
	switch rawBackendProtocol {
	case string(elbv2model.ProtocolHTTP):
		return elbv2model.ProtocolHTTP, nil
	case string(elbv2model.ProtocolHTTPS):
		return elbv2model.ProtocolHTTPS, nil
	default:
		return "", errors.Errorf("backend protocol must be within [%v, %v]: %v", elbv2model.ProtocolHTTP, elbv2model.ProtocolHTTPS, rawBackendProtocol)
	}
}

func (b *defaultModelBuilder) buildTargetGroupHealthCheckConfig(ctx context.Context, svc *corev1.Service, svcAndIngAnnotations map[string]string, targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol) (elbv2model.TargetGroupHealthCheckConfig, error) {
	healthCheckPort, err := b.buildTargetGroupHealthCheckPort(ctx, svc, svcAndIngAnnotations, targetType)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckProtocol, err := b.buildTargetGroupHealthCheckProtocol(ctx, svcAndIngAnnotations, tgProtocol)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckPath := b.buildTargetGroupHealthCheckPath(ctx, svcAndIngAnnotations)
	healthCheckMatcher := b.buildTargetGroupHealthCheckMatcher(ctx, svcAndIngAnnotations)
	healthCheckIntervalSeconds, err := b.buildTargetGroupHealthCheckIntervalSeconds(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckTimeoutSeconds, err := b.buildTargetGroupHealthCheckTimeoutSeconds(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckHealthyThresholdCount, err := b.buildTargetGroupHealthCheckHealthyThresholdCount(ctx, svcAndIngAnnotations)
	if err != nil {
		return elbv2model.TargetGroupHealthCheckConfig{}, err
	}
	healthCheckUnhealthyThresholdCount, err := b.buildTargetGroupHealthCheckUnhealthyThresholdCount(ctx, svcAndIngAnnotations)
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

func (b *defaultModelBuilder) buildTargetGroupHealthCheckPort(ctx context.Context, svc *corev1.Service, svcAndIngAnnotations map[string]string, targetType elbv2model.TargetType) (intstr.IntOrString, error) {
	rawHealthCheckPort := ""
	if exist := b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixHealthCheckPort, &rawHealthCheckPort, svcAndIngAnnotations); !exist {
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

func (b *defaultModelBuilder) buildTargetGroupHealthCheckProtocol(ctx context.Context, svcAndIngAnnotations map[string]string, tgProtocol elbv2model.Protocol) (elbv2model.Protocol, error) {
	rawHealthCheckProtocol := string(tgProtocol)
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixHealthCheckProtocol, &rawHealthCheckProtocol, svcAndIngAnnotations)
	switch rawHealthCheckProtocol {
	case string(elbv2model.ProtocolHTTP):
		return elbv2model.ProtocolHTTP, nil
	case string(elbv2model.ProtocolHTTPS):
		return elbv2model.ProtocolHTTPS, nil
	default:
		return "", errors.Errorf("healthCheckProtocol must be within [%v, %v]", elbv2model.ProtocolHTTP, elbv2model.ProtocolHTTPS)
	}
}

func (b *defaultModelBuilder) buildTargetGroupHealthCheckPath(ctx context.Context, svcAndIngAnnotations map[string]string) string {
	rawHealthCheckPath := b.defaultHealthCheckPath
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixHealthCheckPath, &rawHealthCheckPath, svcAndIngAnnotations)
	return rawHealthCheckPath
}

func (b *defaultModelBuilder) buildTargetGroupHealthCheckMatcher(ctx context.Context, svcAndIngAnnotations map[string]string) elbv2model.HealthCheckMatcher {
	rawHealthCheckMatcherHTTPCode := b.defaultHealthCheckMatcherHTTPCode
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixSuccessCodes, &rawHealthCheckMatcherHTTPCode, svcAndIngAnnotations)
	return elbv2model.HealthCheckMatcher{
		HTTPCode: rawHealthCheckMatcherHTTPCode,
	}
}

func (b *defaultModelBuilder) buildTargetGroupHealthCheckIntervalSeconds(ctx context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawHealthCheckIntervalSeconds := b.defaultHealthCheckIntervalSeconds
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.IngressSuffixHealthCheckIntervalSeconds,
		&rawHealthCheckIntervalSeconds, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawHealthCheckIntervalSeconds, nil
}

func (b *defaultModelBuilder) buildTargetGroupHealthCheckTimeoutSeconds(ctx context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawHealthCheckTimeoutSeconds := b.defaultHealthCheckTimeoutSeconds
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.IngressSuffixHealthCheckTimeoutSeconds,
		&rawHealthCheckTimeoutSeconds, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawHealthCheckTimeoutSeconds, nil
}

func (b *defaultModelBuilder) buildTargetGroupHealthCheckHealthyThresholdCount(ctx context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawHealthCheckHealthyThresholdCount := b.defaultHealthCheckHealthyThresholdCount
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.IngressSuffixHealthyThresholdCount,
		&rawHealthCheckHealthyThresholdCount, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawHealthCheckHealthyThresholdCount, nil
}

func (b *defaultModelBuilder) buildTargetGroupHealthCheckUnhealthyThresholdCount(ctx context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawHealthCheckUnhealthyThresholdCount := b.defaultHealthCheckUnhealthyThresholdCount
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.IngressSuffixUnhealthyThresholdCount,
		&rawHealthCheckUnhealthyThresholdCount, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawHealthCheckUnhealthyThresholdCount, nil
}

func (b *defaultModelBuilder) buildTargetGroupAttributes(ctx context.Context, svcAndIngAnnotations map[string]string) ([]elbv2model.TargetGroupAttribute, error) {
	var rawAttributes map[string]string
	if _, err := b.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTargetGroupAttributes, &rawAttributes, svcAndIngAnnotations); err != nil {
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

func (b *defaultModelBuilder) buildTargetGroupTags(ctx context.Context, svcAndIngAnnotations map[string]string) (map[string]string, error) {
	var rawTags map[string]string
	if _, err := b.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTags, &rawTags, svcAndIngAnnotations); err != nil {
		return nil, err
	}
	return rawTags, nil
}

func (b *defaultModelBuilder) buildTargetGroupResourceID(ingKey types.NamespacedName, svcKey types.NamespacedName, port intstr.IntOrString) string {
	return fmt.Sprintf("%s/%s-%s:%s", ingKey.Namespace, ingKey.Name, svcKey.Name, port.String())
}
