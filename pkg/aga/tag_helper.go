package aga

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
)

type tagHelper interface {
	getAcceleratorTags(ga *agaapi.GlobalAccelerator) (map[string]string, error)
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

func (t *tagHelperImpl) getAcceleratorTags(ga *agaapi.GlobalAccelerator) (map[string]string, error) {
	gaTags := make(map[string]string)

	if ga.Spec.Tags != nil {
		for k, v := range *ga.Spec.Tags {
			gaTags[k] = v
		}
	}

	if err := t.validateTagCollisionWithExternalManagedTags(gaTags); err != nil {
		return nil, err
	}

	return algorithm.MergeStringMap(gaTags, t.defaultTags), nil
}

func (t *tagHelperImpl) validateTagCollisionWithExternalManagedTags(tags map[string]string) error {
	for tagKey := range tags {
		if t.externalManagedTags.Has(tagKey) {
			return errors.Errorf("external managed tag key %v cannot be specified", tagKey)
		}
	}
	return nil
}
