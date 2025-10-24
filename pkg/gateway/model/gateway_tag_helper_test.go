package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
)

func Test_tagHelperImpl_getLoadBalancerTags(t *testing.T) {
	tests := []struct {
		name                   string
		defaultTags            map[string]string
		specTags               map[string]string
		defaultTagsLowPriority bool
		want                   map[string]string
		wantErr                bool
	}{
		{
			name: "when defaultTagsLowPriority is false, default tags override spec tags",
			defaultTags: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
			specTags: map[string]string{
				"env": "dev",
				"app": "web",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"env":  "prod",
				"team": "platform",
				"app":  "web",
			},
		},
		{
			name: "when defaultTagsLowPriority is true, spec tags override default tags",
			defaultTags: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
			specTags: map[string]string{
				"env": "dev",
				"app": "web",
			},
			defaultTagsLowPriority: true,
			want: map[string]string{
				"env":  "dev",
				"team": "platform",
				"app":  "web",
			},
		},
		{
			name: "when no overlapping tags, order doesn't matter",
			defaultTags: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
			specTags: map[string]string{
				"app": "web",
				"env": "dev",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"team":        "platform",
				"cost-center": "123",
				"app":         "web",
				"env":         "dev",
			},
		},
		{
			name:        "when defaultTags is empty, all spec tags are used",
			defaultTags: map[string]string{},
			specTags: map[string]string{
				"app": "web",
				"env": "dev",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"app": "web",
				"env": "dev",
			},
		},
		{
			name: "when specTags is empty, all default tags are used",
			defaultTags: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
			specTags:               map[string]string{},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
		},
		{
			name: "when specTags contains external managed tag, returns error",
			defaultTags: map[string]string{
				"team": "platform",
			},
			specTags: map[string]string{
				"external-tag": "value",
			},
			defaultTagsLowPriority: false,
			want:                   nil,
			wantErr:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			externalManagedTags := sets.New("external-tag")
			h := newTagHelper(externalManagedTags, tt.defaultTags, tt.defaultTagsLowPriority)

			lbConf := elbv2gw.LoadBalancerConfiguration{}
			if len(tt.specTags) > 0 {
				lbConf.Spec.Tags = &tt.specTags
			}

			got, err := h.getLoadBalancerTags(lbConf)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_tagHelperImpl_getTargetGroupTags(t *testing.T) {
	tests := []struct {
		name                   string
		defaultTags            map[string]string
		specTags               map[string]string
		defaultTagsLowPriority bool
		want                   map[string]string
		wantErr                bool
		nilProps               bool
	}{
		{
			name: "when defaultTagsLowPriority is false, default tags override spec tags",
			defaultTags: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
			specTags: map[string]string{
				"env": "dev",
				"app": "web",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"env":  "prod",
				"team": "platform",
				"app":  "web",
			},
		},
		{
			name: "when defaultTagsLowPriority is true, spec tags override default tags",
			defaultTags: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
			specTags: map[string]string{
				"env": "dev",
				"app": "web",
			},
			defaultTagsLowPriority: true,
			want: map[string]string{
				"env":  "dev",
				"team": "platform",
				"app":  "web",
			},
		},
		{
			name: "when no overlapping tags, order doesn't matter",
			defaultTags: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
			specTags: map[string]string{
				"app": "web",
				"env": "dev",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"team":        "platform",
				"cost-center": "123",
				"app":         "web",
				"env":         "dev",
			},
		},
		{
			name:        "when defaultTags is empty, all spec tags are used",
			defaultTags: map[string]string{},
			specTags: map[string]string{
				"app": "web",
				"env": "dev",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"app": "web",
				"env": "dev",
			},
		},
		{
			name: "when specTags is empty, all default tags are used",
			defaultTags: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
			specTags:               map[string]string{},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
		},
		{
			name: "when specTags contains external managed tag, returns error",
			defaultTags: map[string]string{
				"team": "platform",
			},
			specTags: map[string]string{
				"external-tag": "value",
			},
			defaultTagsLowPriority: false,
			want:                   nil,
			wantErr:                true,
		},
		{
			name: "when tgProps is nil, only default tags are used",
			defaultTags: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
			nilProps:               true,
			defaultTagsLowPriority: false,
			want: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			externalManagedTags := sets.New("external-tag")
			h := newTagHelper(externalManagedTags, tt.defaultTags, tt.defaultTagsLowPriority)

			var tgProps *elbv2gw.TargetGroupProps
			if !tt.nilProps {
				tgProps = &elbv2gw.TargetGroupProps{}
				if len(tt.specTags) > 0 {
					tgProps.Tags = &tt.specTags
				}
			}

			got, err := h.getTargetGroupTags(tgProps)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_tagHelperImpl_getListenerRuleTags(t *testing.T) {
	tests := []struct {
		name                   string
		defaultTags            map[string]string
		specTags               map[string]string
		defaultTagsLowPriority bool
		want                   map[string]string
		wantErr                bool
		nilConf                bool
	}{
		{
			name: "when defaultTagsLowPriority is false, default tags override spec tags",
			defaultTags: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
			specTags: map[string]string{
				"env": "dev",
				"app": "web",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"env":  "prod",
				"team": "platform",
				"app":  "web",
			},
		},
		{
			name: "when defaultTagsLowPriority is true, spec tags override default tags",
			defaultTags: map[string]string{
				"env":  "prod",
				"team": "platform",
			},
			specTags: map[string]string{
				"env": "dev",
				"app": "web",
			},
			defaultTagsLowPriority: true,
			want: map[string]string{
				"env":  "dev",
				"team": "platform",
				"app":  "web",
			},
		},
		{
			name: "when no overlapping tags, order doesn't matter",
			defaultTags: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
			specTags: map[string]string{
				"app": "web",
				"env": "dev",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"team":        "platform",
				"cost-center": "123",
				"app":         "web",
				"env":         "dev",
			},
		},
		{
			name:        "when defaultTags is empty, all spec tags are used",
			defaultTags: map[string]string{},
			specTags: map[string]string{
				"app": "web",
				"env": "dev",
			},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"app": "web",
				"env": "dev",
			},
		},
		{
			name: "when specTags is empty, all default tags are used",
			defaultTags: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
			specTags:               map[string]string{},
			defaultTagsLowPriority: false,
			want: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
		},
		{
			name: "when specTags contains external managed tag, returns error",
			defaultTags: map[string]string{
				"team": "platform",
			},
			specTags: map[string]string{
				"external-tag": "value",
			},
			defaultTagsLowPriority: false,
			want:                   nil,
			wantErr:                true,
		},
		{
			name: "when lrConf is nil, only default tags are used",
			defaultTags: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
			nilConf:                true,
			defaultTagsLowPriority: false,
			want: map[string]string{
				"team":        "platform",
				"cost-center": "123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			externalManagedTags := sets.New("external-tag")
			h := newTagHelper(externalManagedTags, tt.defaultTags, tt.defaultTagsLowPriority)

			var lrConf *elbv2gw.ListenerRuleConfiguration
			if !tt.nilConf {
				lrConf = &elbv2gw.ListenerRuleConfiguration{}
				if len(tt.specTags) > 0 {
					lrConf.Spec.Tags = &tt.specTags
				}
			}

			got, err := h.getListenerRuleTags(lrConf)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
