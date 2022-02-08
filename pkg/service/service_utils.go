package service

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

// ServiceUtils to check if the service is supported by the controller
type ServiceUtils interface {
	// IsServiceSupported returns true if the service is supported by the controller
	IsServiceSupported(service *corev1.Service) bool

	// IsServicePendingFinalization returns true if the service contains the aws-load-balancer-controller finalizer
	IsServicePendingFinalization(service *corev1.Service) bool
}

func NewServiceUtils(annotationsParser annotations.Parser, serviceFinalizer string, loadBalancerClass string,
	featureGates config.FeatureGates) *defaultServiceUtils {
	return &defaultServiceUtils{
		annotationParser:  annotationsParser,
		serviceFinalizer:  serviceFinalizer,
		loadBalancerClass: loadBalancerClass,
		featureGates:      featureGates,
	}
}

var _ ServiceUtils = (*defaultServiceUtils)(nil)

type defaultServiceUtils struct {
	annotationParser  annotations.Parser
	serviceFinalizer  string
	loadBalancerClass string
	featureGates      config.FeatureGates
}

// IsServicePendingFinalization returns true if service has the aws-load-balancer-controller finalizer
func (u *defaultServiceUtils) IsServicePendingFinalization(service *corev1.Service) bool {
	if k8s.HasFinalizer(service, u.serviceFinalizer) {
		return true
	}
	return false
}

// IsServiceSupported returns true if the service is supported by the controller
func (u *defaultServiceUtils) IsServiceSupported(service *corev1.Service) bool {
	if !service.DeletionTimestamp.IsZero() {
		return false
	}
	if u.featureGates.Enabled(config.ServiceTypeLoadBalancerOnly) && service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	if service.Spec.LoadBalancerClass != nil {
		if *service.Spec.LoadBalancerClass == u.loadBalancerClass {
			return true
		} else {
			return false
		}
	}
	return u.checkAWSLoadBalancerTypeAnnotation(service)
}

func (u *defaultServiceUtils) checkAWSLoadBalancerTypeAnnotation(service *corev1.Service) bool {
	lbType := ""
	_ = u.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixLoadBalancerType, &lbType, service.Annotations)
	if lbType == LoadBalancerTypeNLBIP {
		return true
	}
	var lbTargetType string
	_ = u.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixTargetType, &lbTargetType, service.Annotations)
	if lbType == LoadBalancerTypeExternal && (lbTargetType == LoadBalancerTargetTypeIP ||
		lbTargetType == LoadBalancerTargetTypeInstance) {
		return true
	}
	return false
}
