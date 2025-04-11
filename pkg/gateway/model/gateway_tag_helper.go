package model

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
)

type tagHelper interface {
	getGatewayTags(lbConf *elbv2gw.LoadBalancerConfiguration) (map[string]string, error)
}

type tagHelperImpl struct {
	externalManagedTags sets.Set[string]
	defaultTags         map[string]string
}

func newTagHelper(externalManagedTags sets.Set[string], defaultTags map[string]string) tagHelper {
	return &tagHelperImpl{
		externalManagedTags: externalManagedTags,
		defaultTags:         defaultTags,
	}
}

func (t *tagHelperImpl) getGatewayTags(lbConf *elbv2gw.LoadBalancerConfiguration) (map[string]string, error) {
	var annotationTags map[string]string

	if lbConf != nil {
		annotationTags = t.convertTagsToMap(lbConf.Spec.Tags)
	}

	if err := t.validateTagCollisionWithExternalManagedTags(annotationTags); err != nil {
		return nil, err
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

func (t *tagHelperImpl) convertTagsToMap(tags []elbv2gw.AWSTag) map[string]string {
	m := make(map[string]string)

	for _, tag := range tags {
		m[tag.Key] = tag.Value
	}
	return m
}
