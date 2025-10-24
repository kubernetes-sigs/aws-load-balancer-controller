package aga

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
)

func Test_tagHelperImpl_getAcceleratorTags(t *testing.T) {
	tests := []struct {
		name                string
		ga                  *agaapi.GlobalAccelerator
		externalManagedTags []string
		defaultTags         map[string]string
		want                map[string]string
		wantErr             bool
	}{
		{
			name: "no tags specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{},
			},
			externalManagedTags: []string{},
			defaultTags: map[string]string{
				"Environment": "test",
				"Team":        "platform",
			},
			want: map[string]string{
				"Environment": "test",
				"Team":        "platform",
			},
			wantErr: false,
		},
		{
			name: "user tags specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Application": "my-app",
						"Owner":       "team-a",
					},
				},
			},
			externalManagedTags: []string{},
			defaultTags: map[string]string{
				"Environment": "test",
				"Team":        "platform",
			},
			want: map[string]string{
				"Application": "my-app",
				"Owner":       "team-a",
				"Environment": "test",
				"Team":        "platform",
			},
			wantErr: false,
		},
		{
			name: "user tags override default tags",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Environment": "production",
						"Application": "my-app",
					},
				},
			},
			externalManagedTags: []string{},
			defaultTags: map[string]string{
				"Environment": "test",
				"Team":        "platform",
			},
			want: map[string]string{
				"Environment": "production", // User tag overrides default
				"Application": "my-app",
				"Team":        "platform",
			},
			wantErr: false,
		},
		{
			name: "external managed tags configured but not specified by user",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Application": "my-app",
						"Owner":       "team-a",
					},
				},
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			want: map[string]string{
				"Application": "my-app",
				"Owner":       "team-a",
				"Environment": "test",
			},
			wantErr: false,
		},
		{
			name: "external managed tags specified by user should cause error",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Application":   "my-app",
						"ExternalTag":   "external-value",
						"ManagedByTeam": "platform-team",
					},
				},
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "single external managed tag specified by user should cause error",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Application": "my-app",
						"ExternalTag": "external-value",
					},
				},
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty default tags",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Application": "my-app",
					},
				},
			},
			externalManagedTags: []string{},
			defaultTags:         map[string]string{},
			want: map[string]string{
				"Application": "my-app",
			},
			wantErr: false,
		},
		{
			name: "nil user tags",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: nil,
				},
			},
			externalManagedTags: []string{},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			want: map[string]string{
				"Environment": "test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			externalManagedTagsSet := sets.New(tt.externalManagedTags...)
			helper := newTagHelper(externalManagedTagsSet, tt.defaultTags)

			got, err := helper.getAcceleratorTags(tt.ga)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_tagHelperImpl_validateTagCollisionWithExternalManagedTags(t *testing.T) {
	tests := []struct {
		name                string
		tags                map[string]string
		externalManagedTags []string
		wantErr             bool
		expectedErrorMsg    string
	}{
		{
			name: "no collision - empty external managed tags",
			tags: map[string]string{
				"Application": "my-app",
				"Owner":       "team-a",
			},
			externalManagedTags: []string{},
			wantErr:             false,
		},
		{
			name: "no collision - different tag keys",
			tags: map[string]string{
				"Application": "my-app",
				"Owner":       "team-a",
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			wantErr:             false,
		},
		{
			name: "collision - single external managed tag",
			tags: map[string]string{
				"Application": "my-app",
				"ExternalTag": "external-value",
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			wantErr:             true,
			expectedErrorMsg:    "external managed tag key ExternalTag cannot be specified",
		},
		{
			name: "collision - multiple external managed tags",
			tags: map[string]string{
				"Application":   "my-app",
				"ExternalTag":   "external-value",
				"ManagedByTeam": "platform-team",
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			wantErr:             true,
			// Error message will contain one of the colliding tags
		},
		{
			name:                "no collision - empty tags",
			tags:                map[string]string{},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			wantErr:             false,
		},
		{
			name:                "no collision - nil tags",
			tags:                nil,
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			wantErr:             false,
		},
		{
			name: "collision - case sensitive",
			tags: map[string]string{
				"externaltag": "external-value", // lowercase
			},
			externalManagedTags: []string{"ExternalTag"}, // uppercase
			wantErr:             false,                   // Should not collide due to case sensitivity
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			externalManagedTagsSet := sets.New(tt.externalManagedTags...)
			helper := &tagHelperImpl{
				externalManagedTags: externalManagedTagsSet,
				defaultTags:         map[string]string{},
			}

			err := helper.validateTagCollisionWithExternalManagedTags(tt.tags)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_newTagHelper(t *testing.T) {
	tests := []struct {
		name                string
		externalManagedTags []string
		defaultTags         map[string]string
	}{
		{
			name:                "create with empty external managed tags",
			externalManagedTags: []string{},
			defaultTags: map[string]string{
				"Environment": "test",
			},
		},
		{
			name:                "create with external managed tags",
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			defaultTags: map[string]string{
				"Environment": "test",
				"Team":        "platform",
			},
		},
		{
			name:                "create with nil external managed tags",
			externalManagedTags: nil,
			defaultTags: map[string]string{
				"Environment": "test",
			},
		},
		{
			name:                "create with empty default tags",
			externalManagedTags: []string{"ExternalTag"},
			defaultTags:         map[string]string{},
		},
		{
			name:                "create with nil default tags",
			externalManagedTags: []string{"ExternalTag"},
			defaultTags:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			externalManagedTagsSet := sets.New(tt.externalManagedTags...)
			helper := newTagHelper(externalManagedTagsSet, tt.defaultTags)

			assert.NotNil(t, helper)

			// Verify the helper is of the correct type
			helperImpl, ok := helper.(*tagHelperImpl)
			assert.True(t, ok)

			// Verify the fields are set correctly
			assert.Equal(t, externalManagedTagsSet, helperImpl.externalManagedTags)
			assert.Equal(t, tt.defaultTags, helperImpl.defaultTags)
		})
	}
}
