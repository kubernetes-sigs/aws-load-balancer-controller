package shared_utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestTagHelperImpl_validateTagCollisionWithExternalManagedTags(t *testing.T) {
	tests := []struct {
		name                string
		externalManagedTags sets.Set[string]
		inputTags           map[string]string
		expectedError       bool
		expectedErrorMsg    string
	}{
		{
			name:                "no external managed tags - no collision",
			externalManagedTags: sets.New[string](),
			inputTags:           map[string]string{"key1": "value1", "key2": "value2"},
			expectedError:       false,
		},
		{
			name:                "empty input tags - no collision",
			externalManagedTags: sets.New[string]("managed-tag1", "managed-tag2"),
			inputTags:           map[string]string{},
			expectedError:       false,
		},
		{
			name:                "nil input tags - no collision",
			externalManagedTags: sets.New[string]("managed-tag1", "managed-tag2"),
			inputTags:           nil,
			expectedError:       false,
		},
		{
			name:                "no collision with external managed tags",
			externalManagedTags: sets.New[string]("managed-tag1", "managed-tag2"),
			inputTags:           map[string]string{"user-tag1": "value1", "user-tag2": "value2"},
			expectedError:       false,
		},
		{
			name:                "single tag collision",
			externalManagedTags: sets.New[string]("managed-tag1", "managed-tag2"),
			inputTags:           map[string]string{"user-tag1": "value1", "managed-tag1": "value2"},
			expectedError:       true,
			expectedErrorMsg:    "external managed tag key managed-tag1 cannot be specified",
		},
		{
			name:                "multiple tag collisions - returns error for first collision found",
			externalManagedTags: sets.New[string]("managed-tag1", "managed-tag2"),
			inputTags:           map[string]string{"managed-tag1": "value1", "managed-tag2": "value2"},
			expectedError:       true,
		},
		{
			name:                "case sensitive collision check",
			externalManagedTags: sets.New[string]("Managed-Tag"),
			inputTags:           map[string]string{"managed-tag": "value1"},
			expectedError:       false,
		},
		{
			name:                "exact case match collision",
			externalManagedTags: sets.New[string]("Managed-Tag"),
			inputTags:           map[string]string{"Managed-Tag": "value1"},
			expectedError:       true,
			expectedErrorMsg:    "external managed tag key Managed-Tag cannot be specified",
		},
		{
			name:                "collision with AWS standard tags",
			externalManagedTags: sets.New[string]("kubernetes.io/cluster/test", "kubernetes.io/service-name"),
			inputTags:           map[string]string{"custom-tag": "value1", "kubernetes.io/cluster/test": "owned"},
			expectedError:       true,
			expectedErrorMsg:    "external managed tag key kubernetes.io/cluster/test cannot be specified",
		},
		{
			name:                "no collision with similar but different tag names",
			externalManagedTags: sets.New[string]("managed-tag"),
			inputTags:           map[string]string{"managed-tag-suffix": "value1", "prefix-managed-tag": "value2"},
			expectedError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := TagHelperConfig{
				ExternalManagedTags: tt.externalManagedTags,
			}
			helper := &tagHelperImpl{
				config: config,
			}

			err := helper.validateTagCollisionWithExternalManagedTags(tt.inputTags)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
				// For multiple collisions, just verify it contains the expected error pattern
				if len(tt.inputTags) > 1 && tt.expectedErrorMsg == "" {
					assert.Contains(t, err.Error(), "external managed tag key")
					assert.Contains(t, err.Error(), "cannot be specified")
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
