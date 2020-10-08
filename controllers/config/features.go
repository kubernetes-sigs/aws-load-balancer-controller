package config

import "github.com/spf13/pflag"

const (
	flagWAFEnabled    = "enable-waf"
	flagWAFV2Enabled  = "enable-wafv2"
	flagShieldEnabled = "enable-shield"
)

// FeatureGate interface for enabling or disabling specific features
type FeatureGate interface {
	// Check if WAF feature is enabled
	WAFEnabled() bool
	// Check if WAFV2 feature is enabled
	WAFV2Enabled() bool
	// Check if Shield feature is enabled
	ShieldEnabled() bool
	BindFlags(fs *pflag.FlagSet)
}

// NewFeatureGate constructs new FeatureGate object
func NewFeatureGate() FeatureGate {
	return &defaultFeatureGate{}
}

type defaultFeatureGate struct {
	wafEnabled    bool
	wafv2Enabled  bool
	shieldEnabled bool
}

func (f *defaultFeatureGate) BindFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&f.wafEnabled, flagWAFEnabled, true, "Enable WAF")
	fs.BoolVar(&f.wafEnabled, flagWAFV2Enabled, true, "Enable WAF V2")
	fs.BoolVar(&f.wafEnabled, flagShieldEnabled, true, "Enable Shield")
}

func (f *defaultFeatureGate) WAFEnabled() bool {
	return f.wafEnabled
}

func (f *defaultFeatureGate) WAFV2Enabled() bool {
	return f.wafv2Enabled
}

func (f *defaultFeatureGate) ShieldEnabled() bool {
	return f.shieldEnabled
}
