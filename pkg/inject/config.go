package inject

import "github.com/spf13/pflag"

const (
	flagEnablePodReadinessGateInject = "enable-pod-readiness-gate-inject"
)

type Config struct {
	EnablePodReadinessGateInject bool
}

func (cfg *Config) BindFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&cfg.EnablePodReadinessGateInject, flagEnablePodReadinessGateInject, true,
		`If enabled, targetHealth readiness gate will get injected to the pod spec for the matching endpoint pods`)
}
