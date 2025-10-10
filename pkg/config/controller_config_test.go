package config

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
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
					shared_constants.TagKeyK8sCluster: "value-a",
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
			err := cfg.validateDefaultTagsCollisionWithTrackingTags()
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
				ExternalManagedTags: []string{shared_constants.TagKeyK8sCluster},
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
			err := cfg.validateExternalManagedTagsCollisionWithTrackingTags()
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

func TestControllerConfig_validateManageBackendSecurityGroupRulesConfiguration(t *testing.T) {
	tests := []struct {
		name                                  string
		enableManageBackendSecurityGroupRules bool
		enableBackendSecurityGroup            bool
		wantErr                               bool
		errMsg                                string
	}{
		{
			name:                                  "with enableManageBackendSecurityGroupRules=false and enableBackendSecurityGroup=false - should succeed",
			enableManageBackendSecurityGroupRules: false,
			enableBackendSecurityGroup:            false,
			wantErr:                               false,
		},
		{
			name:                                  "with enableManageBackendSecurityGroupRules=true and enableBackendSecurityGroup=true - should succeed",
			enableManageBackendSecurityGroupRules: true,
			enableBackendSecurityGroup:            true,
			wantErr:                               false,
		},
		{
			name:                                  "with enableBackendSecurityGroup=true - should succeed",
			enableManageBackendSecurityGroupRules: false,
			enableBackendSecurityGroup:            true,
			wantErr:                               false,
		},
		{
			name:                                  "with enableManageBackendSecurityGroupRules=true and enableBackendSecurityGroup=false - expect error",
			enableManageBackendSecurityGroupRules: true,
			enableBackendSecurityGroup:            false,
			wantErr:                               true,
			errMsg:                                "backend security group must be enabled when manage backend security group rule is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ControllerConfig{
				EnableManageBackendSecurityGroupRules: tt.enableManageBackendSecurityGroupRules,
				EnableBackendSecurityGroup:            tt.enableBackendSecurityGroup,
			}

			err := cfg.validateManageBackendSecurityGroupRulesConfiguration()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestControllerConfig_validateDefaultSubnets(t *testing.T) {
	type fields struct {
		DefaultSubnets []string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name: "default subnets is empty",
			fields: fields{
				DefaultSubnets: nil,
			},
			wantErr: nil,
		},
		{
			name: "default subnets is not empty",
			fields: fields{
				DefaultSubnets: []string{"subnet-1", "subnet-2"},
			},
			wantErr: nil,
		},
		{
			name: "default subnets is not empty and duplicate subnets are specified",
			fields: fields{
				DefaultSubnets: []string{"subnet-1", "subnet-2", "subnet-1"},
			},
			wantErr: errors.New("duplicate subnet id subnet-1 is specified in the --default-subnets flag"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ControllerConfig{
				DefaultSubnets: tt.fields.DefaultSubnets,
			}
			err := cfg.validateDefaultSubnets()
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
