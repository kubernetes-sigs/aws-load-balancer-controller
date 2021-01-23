package gracefuldrain

import (
	"github.com/spf13/pflag"
	"time"
)

const (
	flagPodGracefulDrainDelay = "pod-graceful-drain-delay"
)

type Config struct {
	PodGracefulDrainDelay time.Duration
}

func (cfg *Config) BindFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&cfg.PodGracefulDrainDelay, flagPodGracefulDrainDelay, time.Duration(0),
		`Deregistering pod's deletions are delayed while draining`)
}
