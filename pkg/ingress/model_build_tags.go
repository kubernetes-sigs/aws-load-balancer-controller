package ingress

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

// buildIngressGroupResourceTags builds the AWS Tags used for a group of Ingress. e.g. LoadBalancer, SecurityGroup, Listener
func (t *defaultModelBuildTask) buildIngressGroupResourceTags(ingList []ClassifiedIngress) (map[string]string, error) {
	ingGroupTags := make(map[string]string)
	for _, ing := range ingList {
		ingTags, err := t.buildIngressResourceTags(ing)
		if err != nil {
			return nil, err
		}

		// All these Ingresses are semantically have equal status, so we should check for conflicts on Tags.
		for tagKey, tagValue := range ingTags {
			if existingTagValue, exists := ingGroupTags[tagKey]; exists && existingTagValue != tagValue {
				return nil, errors.Errorf("conflicting tag %v: %v | %v", tagKey, existingTagValue, tagValue)
			}
			ingGroupTags[tagKey] = tagValue
		}
	}
	return ingGroupTags, nil
}

// buildIngressResourceTags builds the AWS Tags used for a single Ingress. e.g. ListenerRule
// Note: the Tags specified via IngressClass takes higher priority than tags specified via annotation on Ingress or Service.
func (t *defaultModelBuildTask) buildIngressResourceTags(ing ClassifiedIngress) (map[string]string, error) {
	var annotationTags map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTags, &annotationTags, ing.Ing.Annotations); err != nil {
		return nil, err
	}
	if err := t.validateTagCollisionWithExternalManagedTags(annotationTags); err != nil {
		return nil, errors.Wrapf(err, "failed build tags for Ingress %v",
			k8s.NamespacedName(ing.Ing).String())
	}

	ingClassTags, err := t.buildIngressClassResourceTags(ing.IngClassConfig)
	if err != nil {
		return nil, err
	}
	return algorithm.MergeStringMap(ingClassTags, annotationTags), nil
}

// buildIngressBackendResourceTags builds the AWS Tags used for a single Ingress and Backend. e.g. TargetGroup.
// Note: the Tags specified via IngressClass takes higher priority than tags specified via annotation on Ingress or Service.
//		 the target group will have the merged tags specified by the annotations of both Ingress and Service
// 		 the Tags annotation of Service takes higher priority if there is conflict between the tags of Ingress and Service
func (t *defaultModelBuildTask) buildIngressBackendResourceTags(ing ClassifiedIngress, backend *corev1.Service) (map[string]string, error) {
	var backendAnnotationTags map[string]string
	var ingressAnnotationTags map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTags, &backendAnnotationTags, backend.Annotations); err != nil {
		return nil, err
	}
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixTags, &ingressAnnotationTags, ing.Ing.Annotations); err != nil {
		return nil, err
	}
	mergedAnnotationTags := algorithm.MergeStringMap(backendAnnotationTags, ingressAnnotationTags)
	if err := t.validateTagCollisionWithExternalManagedTags(mergedAnnotationTags); err != nil {
		return nil, errors.Wrapf(err, "failed build tags for Ingress %v and Service %v",
			k8s.NamespacedName(ing.Ing).String(), k8s.NamespacedName(backend).String())
	}

	ingClassTags, err := t.buildIngressClassResourceTags(ing.IngClassConfig)
	if err != nil {
		return nil, err
	}

	return algorithm.MergeStringMap(ingClassTags, mergedAnnotationTags), nil
}

// buildIngressClassResourceTags builds the AWS Tags for a IngressClass.
func (t *defaultModelBuildTask) buildIngressClassResourceTags(ingClassConfig ClassConfiguration) (map[string]string, error) {
	if ingClassConfig.IngClassParams == nil || len(ingClassConfig.IngClassParams.Spec.Tags) == 0 {
		return nil, nil
	}
	ingClassTags := make(map[string]string, len(ingClassConfig.IngClassParams.Spec.Tags))
	for _, tag := range ingClassConfig.IngClassParams.Spec.Tags {
		ingClassTags[tag.Key] = tag.Value
	}
	if err := t.validateTagCollisionWithExternalManagedTags(ingClassTags); err != nil {
		return nil, errors.Wrapf(err, "failed build tags for IngressClassParams %v",
			ingClassConfig.IngClassParams.Name)
	}

	return ingClassTags, nil
}

func (t *defaultModelBuildTask) validateTagCollisionWithExternalManagedTags(tags map[string]string) error {
	for tagKey := range tags {
		if t.externalManagedTags.Has(tagKey) {
			return errors.Errorf("external managed tag key %v cannot be specified", tagKey)
		}
	}
	return nil
}
