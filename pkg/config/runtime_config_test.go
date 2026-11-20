package config

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildRuntimeOptions_LeaderElectionLeaseDuration(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want *time.Duration
	}{
		{
			name: "flag not set keeps the controller-runtime default",
			args: nil,
			want: nil,
		},
		{
			name: "zero keeps the controller-runtime default",
			args: []string{"--leader-election-lease-duration=0s"},
			want: nil,
		},
		{
			name: "explicit value is passed through",
			args: []string{"--leader-election-lease-duration=30s"},
			want: func() *time.Duration { d := 30 * time.Second; return &d }(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			cfg := RuntimeConfig{}
			cfg.BindFlags(fs)
			require.NoError(t, fs.Parse(tt.args))

			opts, err := BuildRuntimeOptions(cfg, runtime.NewScheme())
			require.NoError(t, err)
			assert.Equal(t, tt.want, opts.LeaseDuration)
		})
	}
}
