package config

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
	"testing"
)

func TestControllerConfig_validateDefaultTagsCollisionWithTrackingTags(t *testing.T) {
	type fields struct {
		DefaultTags map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name: "default tags and tracking tags have no collision",
			fields: fields{
				DefaultTags: map[string]string{
					"tag-a": "value-a",
				},
			},
			wantErr: nil,
		},
		{
			name: "default tags and tracking tags have collision",
			fields: fields{
				DefaultTags: map[string]string{
					"elbv2.k8s.aws/cluster": "value-a",
				},
			},
			wantErr: errors.New("tag key elbv2.k8s.aws/cluster cannot be specified in default-tags flag"),
		},
		{
			name: "default tags is empty",
			fields: fields{
				DefaultTags: nil,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ControllerConfig{
				DefaultTags: tt.fields.DefaultTags,
			}
			trackingTagKeys := sets.New[string](
				"elbv2.k8s.aws/cluster",
				"elbv2.k8s.aws/resource",
				"ingress.k8s.aws/stack",
				"ingress.k8s.aws/resource",
				"service.k8s.aws/stack",
				"service.k8s.aws/resource",
			)
			err := cfg.validateDefaultTagsCollisionWithTrackingTags(trackingTagKeys)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestControllerConfig_validateExternalManagedTagsCollisionWithTrackingTags(t *testing.T) {
	type fields struct {
		ExternalManagedTags []string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name: "external managed tags and tracking tags have no collision",
			fields: fields{
				ExternalManagedTags: []string{"tag-a"},
			},
			wantErr: nil,
		},
		{
			name: "external managed tags and tracking tags have collision",
			fields: fields{
				ExternalManagedTags: []string{"elbv2.k8s.aws/cluster"},
			},
			wantErr: errors.New("tag key elbv2.k8s.aws/cluster cannot be specified in external-managed-tags flag"),
		},
		{
			name: "external managed tags is empty",
			fields: fields{
				ExternalManagedTags: nil,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ControllerConfig{
				ExternalManagedTags: tt.fields.ExternalManagedTags,
			}
			trackingTagKeys := sets.New[string](
				"elbv2.k8s.aws/cluster",
				"elbv2.k8s.aws/resource",
				"ingress.k8s.aws/stack",
				"ingress.k8s.aws/resource",
				"service.k8s.aws/stack",
				"service.k8s.aws/resource",
			)
			err := cfg.validateExternalManagedTagsCollisionWithTrackingTags(trackingTagKeys)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestControllerConfig_validateExternalManagedTagsCollisionWithDefaultTags(t *testing.T) {
	type fields struct {
		DefaultTags         map[string]string
		ExternalManagedTags []string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name: "default tags and external managed tags have no collision",
			fields: fields{
				DefaultTags: map[string]string{
					"tag-a": "value-a",
				},
				ExternalManagedTags: []string{"tag-b"},
			},
			wantErr: nil,
		},
		{
			name: "default tags and external managed tags have collision",
			fields: fields{
				DefaultTags: map[string]string{
					"tag-a": "value-a",
				},
				ExternalManagedTags: []string{"tag-a"},
			},
			wantErr: errors.New("tag key tag-a cannot be specified in both default-tags and external-managed-tags flag"),
		},
		{
			name: "empty default tags and non-empty external managed tags",
			fields: fields{
				DefaultTags:         nil,
				ExternalManagedTags: []string{"tag-a"},
			},
			wantErr: nil,
		},
		{
			name: "empty default tags and empty external managed tags",
			fields: fields{
				DefaultTags:         nil,
				ExternalManagedTags: nil,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ControllerConfig{
				DefaultTags:         tt.fields.DefaultTags,
				ExternalManagedTags: tt.fields.ExternalManagedTags,
			}
			err := cfg.validateExternalManagedTagsCollisionWithDefaultTags()
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestControllerConfig_validateResourcePrefixKeys(t *testing.T) {
	type fields struct {
		ResourcePrefix map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name: "resource prefix has all keys",
			fields: fields{
				ResourcePrefix: map[string]string{
					"clusterTagPrefix":         "elbv2.k8s.aws",
					"ingressTagPrefix":         "ingress.k8s.aws",
					"serviceTagPrefix":         "service.k8s.aws",
					"backendSGNamePrefix":      "k8s-traffic",
					"clusterSgRuleLabelPrefix": "elbv2.k8s.aws",
				},
			},
			wantErr: nil,
		},
		{
			name: "resource prefix has some invalid keys",
			fields: fields{
				ResourcePrefix: map[string]string{
					"clusterTagPrefix":    "elbv2.k8s.aws",
					"ingressTagPrefix":    "ingress.k8s.aws",
					"serviceTagPrefix":    "service.k8s.aws",
					"backendSGNamePrefix": "k8s-traffic",
					"myKey":               "myVal",
				},
			},
			wantErr: errors.New("invalid key: myKey. Valid keys are: [backendSGNamePrefix clusterSgRuleLabelPrefix clusterTagPrefix ingressTagPrefix serviceTagPrefix]"),
		},
		{
			name: "resource prefix is missing some valid keys",
			fields: fields{
				ResourcePrefix: map[string]string{
					"clusterTagPrefix":    "elbv2.k8s.aws",
					"ingressTagPrefix":    "ingress.k8s.aws",
					"serviceTagPrefix":    "service.k8s.aws",
					"backendSGNamePrefix": "k8s-traffic",
				},
			},
			wantErr: errors.New("invalid number of keys. Expected 5 keys, but got 4 keys"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ControllerConfig{
				ResourcePrefix: tt.fields.ResourcePrefix,
			}
			err := cfg.validateResourcePrefixKeys()
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
