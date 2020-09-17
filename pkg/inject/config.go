package inject

import "github.com/spf13/pflag"

const (
	flagEnableReadinessInject = "enable-readiness-gate-inject"
)

type Config struct {
	EnablePodReadinessGateInject bool
}

func (cfg *Config) BindFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&cfg.EnablePodReadinessGateInject, flagEnableReadinessInject, false,
		`If enabled, readiness gate config will get injected to the pod spec for the matching endpoint pods`)
}
