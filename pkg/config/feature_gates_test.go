package config

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultFeatureGatesString(t *testing.T) {
	featureGates := NewFeatureGates().(*defaultFeatureGates)
	require.NoError(t, featureGates.Set("GlobalAcceleratorController=true"))

	raw := featureGates.String()
	settings, err := featureGates.SplitMapStringBool(raw)

	require.NoError(t, err)
	assert.Equal(t, featureGates.Enabled(ListenerRulesTagging), settings[string(ListenerRulesTagging)])
	assert.Equal(t, featureGates.Enabled(GlobalAcceleratorController), settings[string(GlobalAcceleratorController)])
	assert.True(t, settings[string(GlobalAcceleratorController)])
	assert.NotContains(t, raw, "{")
}

func TestDefaultFeatureGatesStringIsSorted(t *testing.T) {
	featureGates := NewFeatureGates().(*defaultFeatureGates)

	raw := featureGates.String()
	parts := strings.Split(raw, ",")

	assert.True(t, sort.StringsAreSorted(parts))
}
