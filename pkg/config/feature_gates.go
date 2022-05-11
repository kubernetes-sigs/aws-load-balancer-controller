package config

import (
	"fmt"
	"github.com/spf13/pflag"
	"strconv"
	"strings"
)

type Feature string

const (
	ListenerRulesTagging        Feature = "ListenerRulesTagging"
	WeightedTargetGroups        Feature = "WeightedTargetGroups"
	ServiceTypeLoadBalancerOnly Feature = "ServiceTypeLoadBalancerOnly"
	EndpointsFailOpen           Feature = "EndpointsFailOpen"
	EnableServiceController     Feature = "EnableServiceController"
	EnableIPTargetType          Feature = "EnableIPTargetType"
	SubnetsClusterTagCheck      Feature = "SubnetsClusterTagCheck"
)

type FeatureGates interface {
	// Enabled returns whether a feature is enabled
	Enabled(feature Feature) bool

	// Enable will enable a feature
	Enable(feature Feature)

	// Disable will disable a feature
	Disable(feature Feature)

	// BindFlags bind featureGates flags
	BindFlags(fs *pflag.FlagSet)
}

var _ FeatureGates = (*defaultFeatureGates)(nil)
var _ pflag.Value = (*defaultFeatureGates)(nil)

type defaultFeatureGates struct {
	featureState map[Feature]bool
}

// NewFeatureGates constructs new featureGates
func NewFeatureGates() FeatureGates {
	return &defaultFeatureGates{
		featureState: map[Feature]bool{
			ListenerRulesTagging:        true,
			WeightedTargetGroups:        true,
			ServiceTypeLoadBalancerOnly: false,
			EndpointsFailOpen:           false,
			EnableServiceController:     true,
			EnableIPTargetType:          true,
			SubnetsClusterTagCheck:      true,
		},
	}
}

func (f *defaultFeatureGates) BindFlags(fs *pflag.FlagSet) {
	fs.Var(f, "feature-gates", "A set of key=bool pairs enable/disable features")
}

func (f *defaultFeatureGates) Enabled(feature Feature) bool {
	return f.featureState[feature]
}

func (f *defaultFeatureGates) Enable(feature Feature) {
	f.featureState[feature] = true
}

func (f *defaultFeatureGates) Disable(feature Feature) {
	f.featureState[feature] = false
}

func (f *defaultFeatureGates) String() string {
	var featureSettings []string
	for feature, enabled := range f.featureState {
		featureSettings = append(featureSettings, fmt.Sprintf("%v=%v", feature, enabled))
	}
	return strings.Join(featureSettings, ",")
}

// SplitMapStringBool parse comma-separated string of key1=value1,key2=value2. value is either true or false
func (f *defaultFeatureGates) SplitMapStringBool(str string) (map[string]bool, error) {
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

func (f *defaultFeatureGates) Set(value string) error {
	settings, err := f.SplitMapStringBool(value)
	if err != nil {
		return fmt.Errorf("failed to parse feature-gates settings due to %v", err)
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

func (f *defaultFeatureGates) Type() string {
	return "mapStringBool"
}
