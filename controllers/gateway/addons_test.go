package gateway

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/addon"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sort"
	"testing"
)

func Test_getStoredAddonConfig(t *testing.T) {
	testCases := []struct {
		name     string
		input    *gwv1.Gateway
		expected []addon.AddonMetadata
	}{
		{
			name:     "nil annotations",
			input:    &gwv1.Gateway{},
			expected: []addon.AddonMetadata{},
		},
		{
			name: "no addon annotations",
			input: &gwv1.Gateway{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: []addon.AddonMetadata{},
		},
		{
			name: "unknown addon",
			input: &gwv1.Gateway{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"gateway.k8s.aws.addon.foo": "true",
					},
				},
			},
			expected: []addon.AddonMetadata{},
		},
		{
			name: "all addons disabled",
			input: &gwv1.Gateway{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"gateway.k8s.aws.addon.wafv2":  "false",
						"gateway.k8s.aws.addon.shield": "false",
					},
				},
			},
			expected: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: false,
				},
				{
					Name:    addon.Shield,
					Enabled: false,
				},
			},
		},
		{
			name: "all addons enabled",
			input: &gwv1.Gateway{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"gateway.k8s.aws.addon.wafv2":  "true",
						"gateway.k8s.aws.addon.shield": "true",
					},
				},
			},
			expected: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: true,
				},
				{
					Name:    addon.Shield,
					Enabled: true,
				},
			},
		},
		{
			name: "malformed value",
			input: &gwv1.Gateway{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"gateway.k8s.aws.addon.wafv2": "fooooooo",
					},
				},
			},
			expected: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: false,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getStoredAddonConfig(tc.input, logr.Discard())
			sort.Slice(result, func(i, j int) bool {
				return result[i].Name > result[j].Name
			})

			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_generateAddOnKey(t *testing.T) {
	testCases := []struct {
		name     string
		input    addon.Addon
		expected string
	}{
		{
			name:     "wafv2",
			input:    addon.WAFv2,
			expected: "gateway.k8s.aws.addon.wafv2",
		},
		{
			name:     "shield",
			input:    addon.Shield,
			expected: "gateway.k8s.aws.addon.shield",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateAddOnKey(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_diffAddOns(t *testing.T) {
	testCases := []struct {
		name             string
		old              []addon.Addon
		new              []addon.AddonMetadata
		expectedAdditons sets.Set[addon.Addon]
		expectedRemovals sets.Set[addon.Addon]
	}{
		{
			name:             "no old, no new",
			old:              []addon.Addon{},
			new:              []addon.AddonMetadata{},
			expectedAdditons: sets.New[addon.Addon](),
			expectedRemovals: sets.New[addon.Addon](),
		},
		{
			name: "just old, no new",
			old: []addon.Addon{
				addon.WAFv2,
			},
			new:              []addon.AddonMetadata{},
			expectedAdditons: sets.New[addon.Addon](),
			expectedRemovals: sets.New[addon.Addon](addon.WAFv2),
		},
		{
			name: "just new, no old - enabled",
			old:  []addon.Addon{},
			new: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: true,
				},
			},
			expectedAdditons: sets.New[addon.Addon](addon.WAFv2),
			expectedRemovals: sets.New[addon.Addon](),
		},
		{
			name: "just new, no old - disabled",
			old:  []addon.Addon{},
			new: []addon.AddonMetadata{
				{
					Name:    addon.WAFv2,
					Enabled: false,
				},
			},
			expectedAdditons: sets.New[addon.Addon](),
			expectedRemovals: sets.New[addon.Addon](),
		},
		{
			name: "new and old, but with changes - new enabled",
			old: []addon.Addon{
				addon.WAFv2,
			},
			new: []addon.AddonMetadata{
				{
					Name:    addon.Shield,
					Enabled: true,
				},
			},
			expectedAdditons: sets.New[addon.Addon](addon.Shield),
			expectedRemovals: sets.New[addon.Addon](addon.WAFv2),
		},
		{
			name: "new and old, but with changes - new disabled",
			old: []addon.Addon{
				addon.WAFv2,
			},
			new: []addon.AddonMetadata{
				{
					Name:    addon.Shield,
					Enabled: false,
				},
			},
			expectedAdditons: sets.New[addon.Addon](),
			expectedRemovals: sets.New[addon.Addon](addon.WAFv2),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			additions, removals := diffAddOns(tc.old, tc.new)
			assert.Equal(t, tc.expectedAdditons, additions)
			assert.Equal(t, tc.expectedRemovals, removals)
		})
	}
}

func Test_persistAddOns(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		changes     []addon.Addon
		remove      bool
		expected    map[string]string
	}{
		{
			name: "no initial annotations, no changes",
		},
		{
			name: "no initial annotations, with changes - remove",
			changes: []addon.Addon{
				addon.WAFv2,
			},
			remove: true,
			expected: map[string]string{
				"gateway.k8s.aws.addon.wafv2": "false",
			},
		},
		{
			name: "no initial annotations, with changes - add",
			changes: []addon.Addon{
				addon.WAFv2,
			},
			remove: false,
			expected: map[string]string{
				"gateway.k8s.aws.addon.wafv2": "true",
			},
		},
		{
			name: "with initial annotations, with changes - remove",
			changes: []addon.Addon{
				addon.WAFv2,
			},
			annotations: map[string]string{
				"foo": "bar",
			},
			remove: true,
			expected: map[string]string{
				"gateway.k8s.aws.addon.wafv2": "false",
				"foo":                         "bar",
			},
		},
		{
			name: "with initial annotations, with changes - add",
			changes: []addon.Addon{
				addon.WAFv2,
			},
			annotations: map[string]string{
				"foo": "bar",
			},
			remove: false,
			expected: map[string]string{
				"gateway.k8s.aws.addon.wafv2": "true",
				"foo":                         "bar",
			},
		},
		{
			name: "with other add on already present annotations, with changes - remove",
			changes: []addon.Addon{
				addon.WAFv2,
			},
			annotations: map[string]string{
				"gateway.k8s.aws.addon.shield": "true",
			},
			remove: true,
			expected: map[string]string{
				"gateway.k8s.aws.addon.wafv2":  "false",
				"gateway.k8s.aws.addon.shield": "true",
			},
		},
		{
			name: "with other add on already present annotations, with changes - add",
			changes: []addon.Addon{
				addon.WAFv2,
			},
			annotations: map[string]string{
				"gateway.k8s.aws.addon.shield": "true",
			},
			remove: false,
			expected: map[string]string{
				"gateway.k8s.aws.addon.wafv2":  "true",
				"gateway.k8s.aws.addon.shield": "true",
			},
		},
		{
			name: "flip value of addon - remove",
			changes: []addon.Addon{
				addon.WAFv2,
			},
			annotations: map[string]string{
				"gateway.k8s.aws.addon.wafv2": "true",
			},
			remove: true,
			expected: map[string]string{
				"gateway.k8s.aws.addon.wafv2": "false",
			},
		},
		{
			name: "flip value of addon - add",
			changes: []addon.Addon{
				addon.WAFv2,
			},
			annotations: map[string]string{
				"gateway.k8s.aws.addon.wafv2": "false",
			},
			expected: map[string]string{
				"gateway.k8s.aws.addon.wafv2": "true",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := testutils.GenerateTestClient()

			gw := &gwv1.Gateway{
				ObjectMeta: v1.ObjectMeta{
					Name:        "test",
					Annotations: tc.annotations,
				},
			}

			err := client.Create(context.Background(), gw)
			assert.NoError(t, err)

			err = persistAddOns(context.Background(), client, gw, tc.changes, tc.remove)
			assert.NoError(t, err)

			newGw := &gwv1.Gateway{}
			err = client.Get(context.Background(), k8s.NamespacedName(gw), newGw)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, newGw.Annotations)
		})
	}
}
