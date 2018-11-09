package aws

import (
	"net/http"

	"github.com/golang/glog"
	"k8s.io/apiserver/pkg/server/healthz"
)

type HealthChecker struct {
	healthCheckFuncs []func() error
}

// Constructs a new healthChecker
func NewHealthChecker(cloud CloudAPI) *HealthChecker {
	healthCheckFuncs := []func() error{cloud.StatusEC2(), cloud.StatusIAM()}
	if cloud.ACMAvailable() {
		healthCheckFuncs = append(healthCheckFuncs, cloud.StatusACM())
	}

	return &HealthChecker{
		healthCheckFuncs: healthCheckFuncs,
	}
}

var _ healthz.HealthzChecker = (*HealthChecker)(nil)

func (c *HealthChecker) Name() string {
	// TODO, can we rename it to, e.g. AWS? is there any dependencies on the name?
	return "aws-alb-ingress-controller"
}

// TODO, validate the call health check frequency
func (c *HealthChecker) Check(_ *http.Request) error {
	for _, fn := range c.healthCheckFuncs {
		err := fn()
		if err != nil {
			glog.Errorf("Controller health check failed: %v", err.Error())
			return err
		}
	}
	return nil
}
