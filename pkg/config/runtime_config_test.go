package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
)

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
