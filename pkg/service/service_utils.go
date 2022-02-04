package service

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

// ServiceUtils to check if the service is supported by the controller
type ServiceUtils interface {
	// IsServiceSupported returns true if the service is supported by the controller
	IsServiceSupported(service *corev1.Service) bool

	// IsLoadBalancerTypeAnnotationSupported returns true if the combination of aws-load-balancer-type/aws-load-balancer-nlb-target-type
	// annotations are supported by this controller
	IsLoadBalancerTypeAnnotationSupported(service *corev1.Service) bool
}

func NewServiceUtils(annotationsParser annotations.Parser, serviceFinalizer string, loadBalancerClass string) *defaultServiceUtils {
	return &defaultServiceUtils{
		annotationParser:  annotationsParser,
		serviceFinalizer:  serviceFinalizer,
		loadBalancerClass: loadBalancerClass,
	}
}

var _ ServiceUtils = (*defaultServiceUtils)(nil)

type defaultServiceUtils struct {
	annotationParser  annotations.Parser
	serviceFinalizer  string
	loadBalancerClass string
}

// IsServiceSupported returns true if the service is supported by the controller
func (u *defaultServiceUtils) IsServiceSupported(service *corev1.Service) bool {
	if k8s.HasFinalizer(service, u.serviceFinalizer) {
		return true
	}
	if service.Spec.LoadBalancerClass != nil && *service.Spec.LoadBalancerClass == u.loadBalancerClass {
		return true
	}
	return u.checkAWSLoadBalancerTypeAnnotation(service)
}

func (u *defaultServiceUtils) IsLoadBalancerTypeAnnotationSupported(service *corev1.Service) bool {
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
