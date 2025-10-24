package model

import (
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
)

// LoadBalancerConfigurationTagProvider adapts LoadBalancerConfiguration to implement TagProvider
type LoadBalancerConfigurationTagProvider struct {
	lbConf elbv2gw.LoadBalancerConfiguration
}

// NewLoadBalancerConfigurationTagProvider creates a new LoadBalancerConfigurationTagProvider
func NewLoadBalancerConfigurationTagProvider(lbConf elbv2gw.LoadBalancerConfiguration) shared_utils.TagProvider {
	return &LoadBalancerConfigurationTagProvider{lbConf: lbConf}
}

// GetTags returns the tags from the LoadBalancerConfiguration spec
func (l *LoadBalancerConfigurationTagProvider) GetTags() *map[string]string {
	return l.lbConf.Spec.Tags
}

// TargetGroupPropsTagProvider adapts TargetGroupProps to implement TagProvider
type TargetGroupPropsTagProvider struct {
	tgProps *elbv2gw.TargetGroupProps
}

// NewTargetGroupPropsTagProvider creates a new TargetGroupPropsTagProvider
func NewTargetGroupPropsTagProvider(tgProps *elbv2gw.TargetGroupProps) shared_utils.TagProvider {
	return &TargetGroupPropsTagProvider{tgProps: tgProps}
}

// GetTags returns the tags from the TargetGroupProps
func (t *TargetGroupPropsTagProvider) GetTags() *map[string]string {
	if t.tgProps == nil {
		return nil
	}
	return t.tgProps.Tags
}

// ListenerRuleConfigurationTagProvider adapts ListenerRuleConfiguration to implement TagProvider
type ListenerRuleConfigurationTagProvider struct {
	lrConf *elbv2gw.ListenerRuleConfiguration
}

// NewListenerRuleConfigurationTagProvider creates a new ListenerRuleConfigurationTagProvider
func NewListenerRuleConfigurationTagProvider(lrConf *elbv2gw.ListenerRuleConfiguration) shared_utils.TagProvider {
	return &ListenerRuleConfigurationTagProvider{lrConf: lrConf}
}

// GetTags returns the tags from the ListenerRuleConfiguration spec
func (l *ListenerRuleConfigurationTagProvider) GetTags() *map[string]string {
	if l.lrConf == nil {
		return nil
	}
	return l.lrConf.Spec.Tags
}
