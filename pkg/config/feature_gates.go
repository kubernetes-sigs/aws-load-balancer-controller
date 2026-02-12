package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

type Feature string

type FeatureStatus struct {
	// Enabled - Is this feature enabled?
	Enabled bool
	// IsDefaulted - Did the user specify this flag, or have we fallen back to the default feature state?
	IsDefaulted bool
}

const (
	ListenerRulesTagging          Feature = "ListenerRulesTagging"
	WeightedTargetGroups          Feature = "WeightedTargetGroups"
	ServiceTypeLoadBalancerOnly   Feature = "ServiceTypeLoadBalancerOnly"
	EndpointsFailOpen             Feature = "EndpointsFailOpen"
	EnableServiceController       Feature = "EnableServiceController"
	EnableIPTargetType            Feature = "EnableIPTargetType"
	EnableTCPUDPListenerType      Feature = "EnableTCPUDPListener"
	EnableRGTAPI                  Feature = "EnableRGTAPI"
	SubnetsClusterTagCheck        Feature = "SubnetsClusterTagCheck"
	NLBHealthCheckAdvancedConfig  Feature = "NLBHealthCheckAdvancedConfig"
	NLBSecurityGroup              Feature = "NLBSecurityGroup"
	ALBSingleSubnet               Feature = "ALBSingleSubnet"
	LBCapacityReservation         Feature = "LBCapacityReservation"
	SubnetDiscoveryByReachability Feature = "SubnetDiscoveryByReachability"
	NLBGatewayAPI                 Feature = "NLBGatewayAPI"
	ALBGatewayAPI                 Feature = "ALBGatewayAPI"
	GlobalAcceleratorController   Feature = "GlobalAcceleratorController"
	EnhancedDefaultBehavior       Feature = "EnhancedDefaultBehavior"
	EnableDefaultTagsLowPriority  Feature = "EnableDefaultTagsLowPriority"
	ALBTargetControlAgent         Feature = "ALBTargetControlAgent"
	EnableCertificateManagement   Feature = "EnableCertificateManagement"
)

type FeatureGates interface {
	// Enabled returns whether a feature is enabled
	Enabled(feature Feature) bool

	// GetFeatureStatus returns Enabled(feature) but with more metadata
	GetFeatureStatus(feature Feature) FeatureStatus

	// Enable will enable a feature
	Enable(feature Feature)

	// Disable will disable a feature
	Disable(feature Feature)

	// BindFlags bind featureGates flags
	BindFlags(fs *pflag.FlagSet)
}

var (
	_ FeatureGates = (*defaultFeatureGates)(nil)
	_ pflag.Value  = (*defaultFeatureGates)(nil)
)

type defaultFeatureGates struct {
	featureState map[Feature]FeatureStatus
}

// NewFeatureGates constructs new featureGates
func NewFeatureGates() FeatureGates {
	return &defaultFeatureGates{
		featureState: map[Feature]FeatureStatus{
			ListenerRulesTagging:          generateDefaultFeatureStatus(true),
			WeightedTargetGroups:          generateDefaultFeatureStatus(true),
			ServiceTypeLoadBalancerOnly:   generateDefaultFeatureStatus(false),
			EndpointsFailOpen:             generateDefaultFeatureStatus(true),
			EnableServiceController:       generateDefaultFeatureStatus(true),
			EnableIPTargetType:            generateDefaultFeatureStatus(true),
			EnableRGTAPI:                  generateDefaultFeatureStatus(false),
			SubnetsClusterTagCheck:        generateDefaultFeatureStatus(true),
			NLBHealthCheckAdvancedConfig:  generateDefaultFeatureStatus(true),
			NLBSecurityGroup:              generateDefaultFeatureStatus(true),
			ALBSingleSubnet:               generateDefaultFeatureStatus(false),
			SubnetDiscoveryByReachability: generateDefaultFeatureStatus(true),
			LBCapacityReservation:         generateDefaultFeatureStatus(true),
			NLBGatewayAPI:                 generateDefaultFeatureStatus(true),
			ALBGatewayAPI:                 generateDefaultFeatureStatus(true),
			GlobalAcceleratorController:   generateDefaultFeatureStatus(false),
			EnableTCPUDPListenerType:      generateDefaultFeatureStatus(false),
			EnhancedDefaultBehavior:       generateDefaultFeatureStatus(false),
			EnableDefaultTagsLowPriority:  generateDefaultFeatureStatus(false),
			ALBTargetControlAgent:         generateDefaultFeatureStatus(false),
			EnableCertificateManagement:   false,
		},
	}
}

func (f *defaultFeatureGates) BindFlags(fs *pflag.FlagSet) {
	fs.Var(f, "feature-gates", "A set of key=bool pairs enable/disable features")
}

func (f *defaultFeatureGates) GetFeatureStatus(feature Feature) FeatureStatus {
	// Return a copy to not corrupt internal state.
	return FeatureStatus{
		Enabled:     f.featureState[feature].Enabled,
		IsDefaulted: f.featureState[feature].IsDefaulted,
	}
}

func (f *defaultFeatureGates) Enabled(feature Feature) bool {
	return f.featureState[feature].Enabled
}

func (f *defaultFeatureGates) Enable(feature Feature) {
	f.featureState[feature] = generateSetFeatureStatus(true)
}

func (f *defaultFeatureGates) Disable(feature Feature) {
	f.featureState[feature] = generateSetFeatureStatus(false)
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
		f.featureState[Feature(k)] = generateSetFeatureStatus(v)
	}
	return nil
}

func (f *defaultFeatureGates) Type() string {
	return "mapStringBool"
}

func generateDefaultFeatureStatus(enabled bool) FeatureStatus {
	return FeatureStatus{
		Enabled:     enabled,
		IsDefaulted: true,
	}
}

func generateSetFeatureStatus(enabled bool) FeatureStatus {
	return FeatureStatus{
		Enabled:     enabled,
		IsDefaulted: false,
	}
}
