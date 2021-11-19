package config

import (
	"fmt"
	"github.com/spf13/pflag"
	"strconv"
	"strings"
)

type Feature string

const (
	EnableListenerRulesTagging Feature = "enable-listener-rules-tagging"
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
			EnableListenerRulesTagging: true,
		},
	}
}

func (f *defaultFeatureGate) BindFlags(fs *pflag.FlagSet) {
	fs.Var(f, "feature-gate", "A set of key=bool pairs enable/disable features")
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

// SplitMapStringBool parse comma-separated string of key1=value1,key2=value2. value is either true or false
func (f *defaultFeatureGate) SplitMapStringBool(str string) (map[string]bool, error) {
	result := make(map[string]bool)
	for _, s := range strings.Split(str, ",") {
		if len(s) == 0 {
			continue
		}
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid mapStringBool: %v", s)
		}
		k := strings.TrimSpace(parts[0])
		v, err := strconv.ParseBool(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid mapStringBool: %v", s)
		}
		result[k] = v
	}
	return result, nil
}

func (f *defaultFeatureGate) Set(value string) error {
	settings, err := f.SplitMapStringBool(value)
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
