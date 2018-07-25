/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albacm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albiam"
)

// Name returns the healthcheck name
func (c ALBController) Name() string {
	return "aws-alb-ingress-controller"
}

// Check returns if the controller is healthy
func (c *ALBController) Check(_ *http.Request) error {
	if c.isHealthy {
		return nil
	}

	return fmt.Errorf("ingress controller is not healthy")
}

func (c *ALBController) runHealthChecks(interface{}) error {
	glog.V(2).Infof("Executing AWS health checks")
	for _, fn := range []func() error{
		albacm.ACMsvc.Status(),
		albec2.EC2svc.Status(),
		albelbv2.ELBV2svc.Status(),
		albiam.IAMsvc.Status(),
	} {
		err := fn()
		if err != nil {
			glog.Errorf("Controller health check failed: %v", err.Error())
			c.isHealthy = false
			return nil
		}
	}

	c.isHealthy = true
	return nil
}
