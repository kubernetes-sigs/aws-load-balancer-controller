package quic

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestServerIDInjectionConfig_BindFlags_Integration(t *testing.T) {
	config := &ServerIDInjectionConfig{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	config.BindFlags(fs)

	// Test default value
	assert.Equal(t, defaultEnvironmentVariableName, config.EnvironmentVariableName)

	// Test setting custom value
	err := fs.Parse([]string{"--quic-environment-variable-name", "CUSTOM_QUIC_ID"})
	assert.NoError(t, err)
	assert.Equal(t, "CUSTOM_QUIC_ID", config.EnvironmentVariableName)
}

func TestServerIDInjectionConfig_DefaultValues(t *testing.T) {
	assert.Equal(t, "AWS_LBC_QUIC_SERVER_ID", defaultEnvironmentVariableName)
	assert.Equal(t, "quic-environment-variable-name", flagQUICEnvironmentVariableName)
}
