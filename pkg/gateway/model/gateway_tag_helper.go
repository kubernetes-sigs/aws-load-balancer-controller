package model

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
)

type tagHelper interface {
	getGatewayTags(lbConf elbv2gw.LoadBalancerConfiguration) (map[string]string, error)
}

type tagHelperImpl struct {
	externalManagedTags               sets.Set[string]
	defaultTags                       map[string]string
	additionalTagsOverrideDefaultTags bool
}

func newTagHelper(externalManagedTags sets.Set[string], defaultTags map[string]string, additionalTagsOverrideDefaultTags bool) tagHelper {
	return &tagHelperImpl{
		externalManagedTags:               externalManagedTags,
		defaultTags:                       defaultTags,
		additionalTagsOverrideDefaultTags: additionalTagsOverrideDefaultTags,
	}
}

func (t *tagHelperImpl) getGatewayTags(lbConf elbv2gw.LoadBalancerConfiguration) (map[string]string, error) {
	annotationTags := make(map[string]string)

	if lbConf.Spec.Tags != nil {
		for k, v := range *lbConf.Spec.Tags {
			annotationTags[k] = v
		}
	}

	if err := t.validateTagCollisionWithExternalManagedTags(annotationTags); err != nil {
		return nil, err
	}

	if t.additionalTagsOverrideDefaultTags {
		return algorithm.MergeStringMap(annotationTags, t.defaultTags), nil
	}
	return algorithm.MergeStringMap(t.defaultTags, annotationTags), nil
}

func (t *tagHelperImpl) validateTagCollisionWithExternalManagedTags(tags map[string]string) error {
	for tagKey := range tags {
		if t.externalManagedTags.Has(tagKey) {
			return errors.Errorf("external managed tag key %v cannot be specified", tagKey)
		}
	}
	return nil
}
