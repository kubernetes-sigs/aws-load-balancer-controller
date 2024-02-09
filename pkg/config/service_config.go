package config

import "github.com/spf13/pflag"

const (
	flagLoadBalancerClass    = "load-balancer-class"
	defaultLoadBalancerClass = "service.k8s.aws/nlb"
)

// ServiceConfig contains the configurations for the Service controller
type ServiceConfig struct {
	// LoadBalancerClass is the name of the load balancer class reconciled by this controller
	LoadBalancerClass string
}

// BindFlags binds the command line flags to the fields in the config object
func (cfg *ServiceConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cfg.LoadBalancerClass, flagLoadBalancerClass, defaultLoadBalancerClass,
		"Name of the load balancer class reconciled by this controller")
}
