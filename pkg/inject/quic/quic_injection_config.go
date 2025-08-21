package quic

import "github.com/spf13/pflag"

// elbv2.k8s.aws/quic-server-id-inject

const (
	quicEnabledContainers           = "service.beta.kubernetes.io/aws-load-balancer-quic-enabled-containers"
	flagQUICEnvironmentVariableName = "quic-environment-variable-name"
	defaultEnvironmentVariableName  = "AWS_LBC_QUIC_SERVER_ID"
)

type QUICServerIDInjectionConfig struct {
	EnvironmentVariableName string
}

func (cfg *QUICServerIDInjectionConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.EnvironmentVariableName, flagQUICEnvironmentVariableName, defaultEnvironmentVariableName,
		`The environment variable to find the generated QUIC Server ID.`)
}
