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

package metric

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DummyCollector dummy implementation for mocks in tests
type DummyCollector struct{}

// IncReloadCount ...
func (dc DummyCollector) IncReconcileCount() {}

// IncReloadErrorCount ...
func (dc DummyCollector) IncReconcileErrorCount(string) {}

// SetManagedIngresses ...
func (dc DummyCollector) SetManagedIngresses(map[string]int) {}

// IncAPIRequestCount ...
func (dc DummyCollector) IncAPIRequestCount(prometheus.Labels) {}

//  ObserveAPIRequest ...
func (dc DummyCollector) ObserveAPIRequest(prometheus.Labels, time.Time) {}

// IncAPIErrorCount ...
func (dc DummyCollector) IncAPIErrorCount(prometheus.Labels) {}

// IncAPIRetryCount ...
func (dc DummyCollector) IncAPIRetryCount(prometheus.Labels) {}

// Start ...
func (dc DummyCollector) Start() {}

// Stop ...
func (dc DummyCollector) Stop() {}

// RemoveMetrics ...
func (dc DummyCollector) RemoveMetrics(string) {}
