package aga

import (
	"k8s.io/apimachinery/pkg/util/sets"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
)

type tagHelper interface {
	getAcceleratorTags(ga agaapi.GlobalAccelerator) (map[string]string, error)
}

type tagHelperImpl struct {
	sharedHelper shared_utils.TagHelper
}

func newTagHelper(externalManagedTags sets.Set[string], defaultTags map[string]string, additionalTagsOverrideDefaultTags bool) tagHelper {
	config := shared_utils.TagHelperConfig{
		ExternalManagedTags:               externalManagedTags,
		DefaultTags:                       defaultTags,
		AdditionalTagsOverrideDefaultTags: additionalTagsOverrideDefaultTags,
	}
	return &tagHelperImpl{
		sharedHelper: shared_utils.NewTagHelper(config),
	}
}

func (t *tagHelperImpl) getAcceleratorTags(ga agaapi.GlobalAccelerator) (map[string]string, error) {
	provider := NewGlobalAcceleratorTagProvider(ga)
	return t.sharedHelper.ProcessTags(provider)
}
