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
)

// Name returns the healthcheck name
func (c ALBController) Name() string {
	return "aws-alb-ingress-controller"
}

// Check returns if the nginx healthz endpoint is returning ok (status code 200)
func (c *ALBController) Check(_ *http.Request) error {
	res, err := http.Get(fmt.Sprintf("http://0.0.0.0:%v%v", c.cfg.HealthzPort, albHealthPath))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("ingress controller is not healthy")
	}

	return err
}
