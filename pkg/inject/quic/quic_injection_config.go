package quic

import "github.com/spf13/pflag"

const (
	flagQUICEnvironmentVariableName = "quic-environment-variable-name"
	defaultEnvironmentVariableName  = "AWS_LBC_QUIC_SERVER_ID"
)

// ServerIDInjectionConfig configuration for handling Server ID injection.
type ServerIDInjectionConfig struct {
	EnvironmentVariableName string
}

func (cfg *ServerIDInjectionConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.EnvironmentVariableName, flagQUICEnvironmentVariableName, defaultEnvironmentVariableName,
		`The environment variable to find the generated QUIC Server ID.`)
}
