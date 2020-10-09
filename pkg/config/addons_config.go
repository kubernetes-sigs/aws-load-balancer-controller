package config

import "github.com/spf13/pflag"

const (
	flagWAFEnabled    = "enable-waf"
	flagWAFV2Enabled  = "enable-wafv2"
	flagShieldEnabled = "enable-shield"
	defaultEnabled    = true
)

type AddonsConfig struct {
	// WAF addon for ALB
	WAFEnabled bool
	// WAFV2 addon for ALB
	WAFV2Enabled bool
	// Shield addon for ALB
	ShieldEnabled bool
}

func (f *AddonsConfig) BindFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&f.WAFEnabled, flagWAFEnabled, defaultEnabled, "Enable WAF addon for ALB")
	fs.BoolVar(&f.WAFEnabled, flagWAFV2Enabled, defaultEnabled, "Enable WAF V2 addon for ALB")
	fs.BoolVar(&f.WAFEnabled, flagShieldEnabled, defaultEnabled, "Enable Shield addon for ALB")
}
