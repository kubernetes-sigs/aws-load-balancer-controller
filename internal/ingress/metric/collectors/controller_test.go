/*
Copyright 2018 The Kubernetes Authors.

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

package collectors

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestControllerCounters(t *testing.T) {
	const metadata = `
		# HELP aws_alb_ingress_controller_success Cumulative number of Ingress controller reconcile operations
		# TYPE aws_alb_ingress_controller_success counter
	`
	cases := []struct {
		name    string
		test    func(*Controller)
		metrics []string
		want    string
	}{
		{
			name: "should return not increment in metrics if no operations are invoked",
			test: func(cm *Controller) {
			},
			want: metadata + `
			`,
			metrics: []string{"aws_alb_ingress_controller_success"},
		},
		{
			name: "single increase in reload count should return 1",
			test: func(cm *Controller) {
				cm.IncReconcileCount()
				// cm.ConfigSuccess(0, true)
			},
			want: metadata + `
				aws_alb_ingress_controller_success{class="alb",namespace="default"} 1
			`,
			metrics: []string{"aws_alb_ingress_controller_success"},
		},
		{
			name: "single increase in error reload count should return 1",
			test: func(cm *Controller) {
				cm.IncReconcileErrorCount()
			},
			want: `
				# HELP aws_alb_ingress_controller_errors Cumulative number of Ingress controller errors during reconcile operations
				# TYPE aws_alb_ingress_controller_errors counter
				aws_alb_ingress_controller_errors{class="alb",namespace="default"} 1
			`,
			metrics: []string{"aws_alb_ingress_controller_errors"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cm := NewController("pod", "default", "alb")
			reg := prometheus.NewPedanticRegistry()
			if err := reg.Register(cm); err != nil {
				t.Errorf("registering collector failed: %s", err)
			}

			c.test(cm)

			if err := GatherAndCompare(cm, c.want, c.metrics, reg); err != nil {
				t.Errorf("unexpected error collecting result:\n%s", err)
			}

			reg.Unregister(cm)
		})
	}
}
