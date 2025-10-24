package model

import (
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
)

type tagHelper interface {
	getLoadBalancerTags(lbConf elbv2gw.LoadBalancerConfiguration) (map[string]string, error)
	getTargetGroupTags(tgProps *elbv2gw.TargetGroupProps) (map[string]string, error)
	getListenerRuleTags(lrConf *elbv2gw.ListenerRuleConfiguration) (map[string]string, error)
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

func (t *tagHelperImpl) getLoadBalancerTags(lbConf elbv2gw.LoadBalancerConfiguration) (map[string]string, error) {
	provider := NewLoadBalancerConfigurationTagProvider(lbConf)
	return t.sharedHelper.ProcessTags(provider)
}

func (t *tagHelperImpl) getTargetGroupTags(tgProps *elbv2gw.TargetGroupProps) (map[string]string, error) {
	provider := NewTargetGroupPropsTagProvider(tgProps)
	return t.sharedHelper.ProcessTags(provider)
}

func (t *tagHelperImpl) getListenerRuleTags(lrConf *elbv2gw.ListenerRuleConfiguration) (map[string]string, error) {
	provider := NewListenerRuleConfigurationTagProvider(lrConf)
	return t.sharedHelper.ProcessTags(provider)
}
