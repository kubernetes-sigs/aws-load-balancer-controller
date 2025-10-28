package shared_utils

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
)

// TagProvider defines an interface for objects that can provide tags
type TagProvider interface {
	GetTags() *map[string]string
}

// TagHelper provides common tag processing functionality
type TagHelper interface {
	ProcessTags(provider TagProvider) (map[string]string, error)
}

// TagHelperConfig holds configuration for tag processing
type TagHelperConfig struct {
	ExternalManagedTags               sets.Set[string]
	DefaultTags                       map[string]string
	AdditionalTagsOverrideDefaultTags bool
}

type tagHelperImpl struct {
	config TagHelperConfig
}

// NewTagHelper creates a new tag helper with the given configuration
func NewTagHelper(config TagHelperConfig) TagHelper {
	return &tagHelperImpl{
		config: config,
	}
}

// ProcessTags processes tags from a TagProvider, validates them, and merges with default tags
func (t *tagHelperImpl) ProcessTags(provider TagProvider) (map[string]string, error) {
	providerTags := make(map[string]string)

	if tags := provider.GetTags(); tags != nil {
		for k, v := range *tags {
			providerTags[k] = v
		}
	}

	if err := t.validateTagCollisionWithExternalManagedTags(providerTags); err != nil {
		return nil, err
	}

	if t.config.AdditionalTagsOverrideDefaultTags {
		return algorithm.MergeStringMap(providerTags, t.config.DefaultTags), nil
	}
	return algorithm.MergeStringMap(t.config.DefaultTags, providerTags), nil
}

func (t *tagHelperImpl) validateTagCollisionWithExternalManagedTags(tags map[string]string) error {
	for tagKey := range tags {
		if t.config.ExternalManagedTags.Has(tagKey) {
			return errors.Errorf("external managed tag key %v cannot be specified", tagKey)
		}
	}
	return nil
}
