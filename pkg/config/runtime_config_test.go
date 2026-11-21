package config

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestRuntimeConfig_BindFlags_LeaderElectionLeaseDuration(t *testing.T) {
	t.Run("default flag value", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cfg := RuntimeConfig{}
		cfg.BindFlags(fs)
		err := fs.Parse([]string{})
		assert.NoError(t, err)
		assert.Equal(t, time.Duration(0), cfg.LeaderElectionLeaseDuration)
	})

	t.Run("configured flag value", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cfg := RuntimeConfig{}
		cfg.BindFlags(fs)
		err := fs.Parse([]string{"--leader-election-lease-duration=30s"})
		assert.NoError(t, err)
		assert.Equal(t, 30*time.Second, cfg.LeaderElectionLeaseDuration)
	})
}

func TestBuildRuntimeOptions_LeaderElectionLeaseDuration(t *testing.T) {
	scheme := runtime.NewScheme()

	t.Run("default lease duration (not set)", func(t *testing.T) {
		cfg := RuntimeConfig{
			LeaderElectionLeaseDuration: 0,
		}
		opts, err := BuildRuntimeOptions(cfg, scheme)
		assert.NoError(t, err)
		assert.Nil(t, opts.LeaseDuration)
	})

	t.Run("configured lease duration", func(t *testing.T) {
		cfg := RuntimeConfig{
			LeaderElectionLeaseDuration: 30 * time.Second,
		}
		opts, err := BuildRuntimeOptions(cfg, scheme)
		assert.NoError(t, err)
		assert.NotNil(t, opts.LeaseDuration)
		assert.Equal(t, 30*time.Second, *opts.LeaseDuration)
	})
}
