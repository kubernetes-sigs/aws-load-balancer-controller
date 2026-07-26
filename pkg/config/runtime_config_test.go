package config

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestRuntimeConfig_BindFlags_CacheSyncTimeout(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected time.Duration
	}{
		{
			name:     "default value is 2 minutes",
			args:     []string{},
			expected: 2 * time.Minute,
		},
		{
			name:     "custom value is respected",
			args:     []string{"--cache-sync-timeout=5m"},
			expected: 5 * time.Minute,
		},
		{
			name:     "short duration value",
			args:     []string{"--cache-sync-timeout=30s"},
			expected: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := RuntimeConfig{}
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			cfg.BindFlags(fs)
			err := fs.Parse(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.CacheSyncTimeout)
		})
	}
}

func TestBuildRuntimeOptions_CacheSyncTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{
			name:     "default timeout is passed through",
			timeout:  2 * time.Minute,
			expected: 2 * time.Minute,
		},
		{
			name:     "custom timeout is passed through",
			timeout:  10 * time.Minute,
			expected: 10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			rtCfg := RuntimeConfig{
				CacheSyncTimeout: tt.timeout,
			}
			opts, err := BuildRuntimeOptions(rtCfg, scheme)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, opts.Controller.CacheSyncTimeout)
		})
	}
}
