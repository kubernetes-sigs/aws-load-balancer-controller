/*
Copyright 2015 The Kubernetes Authors.

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

package config

import (
	"fmt"
	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/utils"
	"github.com/spf13/pflag"
)

type Feature string

const (
	WAF            Feature = "waf"
	ShieldAdvanced Feature = "shield"
)

type FeatureGate interface {
	// Enabled returns whether a feature is enabled
	Enabled(feature Feature) bool

	// Enable will enable a feature
	Enable(feature Feature)

	// Disable will disable a feature
	Disable(feature Feature)

	// BindFlags bind featureGate flags
	BindFlags(fs *pflag.FlagSet)
}

var _ FeatureGate = (*defaultFeatureGate)(nil)
var _ pflag.Value = (*defaultFeatureGate)(nil)

type defaultFeatureGate struct {
	featureState map[Feature]bool
}

// NewFeatureGate constructs new featureGate
func NewFeatureGate() FeatureGate {
	return &defaultFeatureGate{
		featureState: map[Feature]bool{
			WAF:            true,
			ShieldAdvanced: true,
		},
	}
}

func (f *defaultFeatureGate) BindFlags(fs *pflag.FlagSet) {
	fs.Var(f, "feature-gates", "A set of key=bool pairs enable/disable features")
}

func (f *defaultFeatureGate) Enabled(feature Feature) bool {
	return f.featureState[feature]
}

func (f *defaultFeatureGate) Enable(feature Feature) {
	f.featureState[feature] = true
}

func (f *defaultFeatureGate) Disable(feature Feature) {
	f.featureState[feature] = false
}

func (f *defaultFeatureGate) String() string {
	var featureSettings []string
	for feature, enabled := range f.featureState {
		featureSettings = append(featureSettings, fmt.Sprintf("%v=%v", feature, enabled))
	}
	return strings.Join(featureSettings, ",")
}

func (f *defaultFeatureGate) Set(value string) error {
	settings, err := utils.SplitMapStringBool(value)
	if err != nil {
		return fmt.Errorf("failed to parse feature-gate settings due to %v", err)
	}
	for k, v := range settings {
		_, ok := f.featureState[Feature(k)]
		if !ok {
			return fmt.Errorf("unknown feature: %v", k)
		}
		f.featureState[Feature(k)] = v
	}
	return nil
}

func (f *defaultFeatureGate) Type() string {
	return "mapStringBool"
}
