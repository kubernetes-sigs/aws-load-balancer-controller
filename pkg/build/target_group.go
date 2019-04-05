package build

import (
	"context"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"strconv"
)

const (
	HealthCheckPortTrafficPort = "traffic-port"
)
const (
	DefaultHealthCheckPath                          = "/"
	DefaultHealthCheckPort                          = HealthCheckPortTrafficPort
	DefaultHealthCheckIntervalSeconds         int64 = 15
	DefaultHealthCheckTimeoutSeconds          int64 = 5
	DefaultHealthCheckHealthyThresholdCount   int64 = 2
	DefaultHealthCheckUnhealthyThresholdCount int64 = 2
	DefaultHealthCheckSuccessCodes                  = "200"
)

func (b *defaultBuilder) buildTargetGroup(ctx context.Context, stack *LoadBalancingStack, groupID ingress.GroupID, ing *extensions.Ingress, svc *corev1.Service, port intstr.IntOrString) (*api.TargetGroup, error) {
	tgID := b.buildTargetGroupID(k8s.NamespacedName(ing), extensions.IngressBackend{ServiceName: svc.Name, ServicePort: port})
	tg, ok := stack.FindTargetGroup(tgID)
	if !ok {
		tgSpec, err := b.buildTGSpec(ctx, groupID, ing, svc, port)
		if err != nil {
			return nil, err
		}
		tg := &api.TargetGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: tgID,
			},
			Spec: tgSpec,
		}
		stack.AddTargetGroup(tg)

		eb := &api.EndpointBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: stack.ID + ":" + tgID,
			},
			Spec: api.EndpointBindingSpec{
				TargetGroup: api.TargetGroupReference{
					TargetGroupRef: k8s.LocalObjectReference(tg),
				},
				TargetType:  tgSpec.TargetType,
				ServiceRef:  k8s.ObjectReference(svc),
				ServicePort: port,
			},
		}
		stack.AddEndpointBinding(eb)

		return tg, nil
	}
	return tg, nil
}

func (b *defaultBuilder) buildTGSpec(ctx context.Context, groupID ingress.GroupID, ing *extensions.Ingress, svc *corev1.Service, port intstr.IntOrString) (api.TargetGroupSpec, error) {
	ingAnnotations := ing.Annotations
	svcAnnotations := svc.Annotations

	rawTargetType := b.ingConfig.DefaultTargetType
	_ = b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixTargetGroupTargetType, (*string)(&rawTargetType), svcAnnotations, ingAnnotations)
	targetType, err := api.ParseTargetType(rawTargetType)
	if err != nil {
		return api.TargetGroupSpec{}, err
	}

	rawProtocol := b.ingConfig.DefaultBackendProtocol
	_ = b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixTargetGroupBackendProtocol, (*string)(&rawProtocol), svcAnnotations, ingAnnotations)
	protocol, err := api.ParseProtocol(rawProtocol)
	if err != nil {
		return api.TargetGroupSpec{}, err
	}
	if protocol != api.ProtocolHTTPS && protocol != api.ProtocolHTTP {
		return api.TargetGroupSpec{}, errors.Errorf("protocol must be %v or %v", api.ProtocolHTTPS, api.ProtocolHTTP)
	}

	healCheckCfg, err := b.buildTGHealthCheck(ctx, ingAnnotations, svc, targetType)
	if err != nil {
		return api.TargetGroupSpec{}, err
	}
	tgAttributes, err := b.buildTGAttributes(ctx, ingAnnotations, svcAnnotations)
	if err != nil {
		return api.TargetGroupSpec{}, err
	}

	tgName := b.nameTargetGroup(groupID, k8s.NamespacedName(ing), extensions.IngressBackend{ServiceName: svc.Name, ServicePort: port}, targetType, protocol)
	return api.TargetGroupSpec{
		TargetGroupName:   tgName,
		TargetType:        targetType,
		Port:              1,
		Protocol:          protocol,
		HealthCheckConfig: healCheckCfg,
		Attributes:        tgAttributes,
		Tags:              b.ingConfig.DefaultTags,
	}, nil
}

func (b *defaultBuilder) buildTGHealthCheck(ctx context.Context, ingAnnotations map[string]string, svc *corev1.Service, targetType api.TargetType) (api.HealthCheckConfig, error) {
	svcAnnotations := svc.Annotations

	healthCheckCfg := api.HealthCheckConfig{}
	rawProtocol := b.ingConfig.DefaultBackendProtocol
	_ = b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixHealthCheckProtocol, (*string)(&rawProtocol), svcAnnotations, ingAnnotations)
	protocol, err := api.ParseProtocol(rawProtocol)
	if err != nil {
		return api.HealthCheckConfig{}, nil
	}
	healthCheckCfg.Protocol = protocol

	{
		rawPort := ""
		if exists := b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixHealthCheckPort, &rawPort, svcAnnotations, ingAnnotations); exists {
			if rawPort == HealthCheckPortTrafficPort {
				healthCheckCfg.Port = intstr.FromString(rawPort)
			} else if port, err := strconv.ParseInt(rawPort, 10, 64); err == nil {
				healthCheckCfg.Port = intstr.FromInt(int(port))
			} else {
				return api.HealthCheckConfig{}, errors.Errorf("unsupported healthCheck port: %v", rawPort)
			}
		} else {
			healthCheckCfg.Port = intstr.FromString(DefaultHealthCheckPort)
		}

		healthCheckCfg.Port, err = b.resolveHealthCheckPort(healthCheckCfg.Port, svc, targetType)
		if err != nil {
			return api.HealthCheckConfig{}, errors.New("failed to solve healthCheckPort")
		}
	}

	path := DefaultHealthCheckPath
	_ = b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixHealthCheckPath, &path, svcAnnotations, ingAnnotations)
	healthCheckCfg.Path = path

	intervalSeconds := DefaultHealthCheckIntervalSeconds
	if _, err := b.annotationParser.ParseInt64Annotation(k8s.AnnotationSuffixHealthCheckIntervalSeconds, &intervalSeconds, svcAnnotations, ingAnnotations); err != nil {
		return api.HealthCheckConfig{}, err
	}
	healthCheckCfg.IntervalSeconds = intervalSeconds

	timeoutSeconds := DefaultHealthCheckTimeoutSeconds
	if _, err := b.annotationParser.ParseInt64Annotation(k8s.AnnotationSuffixHealthCheckTimeoutSeconds, &timeoutSeconds, svcAnnotations, ingAnnotations); err != nil {
		return api.HealthCheckConfig{}, err
	}
	healthCheckCfg.TimeoutSeconds = timeoutSeconds

	healthyThresholdCount := DefaultHealthCheckHealthyThresholdCount
	if _, err := b.annotationParser.ParseInt64Annotation(k8s.AnnotationSuffixHealthCheckHealthyThresholdCount, &healthyThresholdCount, svcAnnotations, ingAnnotations); err != nil {
		return api.HealthCheckConfig{}, err
	}
	healthCheckCfg.HealthyThresholdCount = healthyThresholdCount

	unhealthyThresholdCount := DefaultHealthCheckUnhealthyThresholdCount
	if _, err := b.annotationParser.ParseInt64Annotation(k8s.AnnotationSuffixHealthCheckUnhealthyThresholdCount, &unhealthyThresholdCount, svcAnnotations, ingAnnotations); err != nil {
		return api.HealthCheckConfig{}, err
	}
	healthCheckCfg.UnhealthyThresholdCount = unhealthyThresholdCount

	successCodes := DefaultHealthCheckSuccessCodes
	_ = b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixHealthCheckSuccessCodes, &successCodes, svcAnnotations, ingAnnotations)
	healthCheckCfg.Matcher.HTTPCode = successCodes

	return healthCheckCfg, nil
}

func (b *defaultBuilder) resolveHealthCheckPort(healthCheckPort intstr.IntOrString, svc *corev1.Service, targetType api.TargetType) (intstr.IntOrString, error) {
	if healthCheckPort.Type == intstr.Int {
		return healthCheckPort, nil
	}
	if healthCheckPort.String() == HealthCheckPortTrafficPort {
		return healthCheckPort, nil
	}

	resolvedServicePort, err := k8s.LookupServicePort(svc, healthCheckPort)
	if err != nil {
		return healthCheckPort, errors.Wrap(err, "failed to resolve healthcheck port for service")
	}

	if targetType == api.TargetTypeInstance {
		if resolvedServicePort.NodePort == 0 {
			return healthCheckPort, errors.Errorf("failed to find valid NodePort for service %s with port %s", svc.Name, resolvedServicePort.Name)
		}
		return intstr.FromInt(int(resolvedServicePort.NodePort)), nil
	}
	return resolvedServicePort.TargetPort, nil
}
