package addon

type Addon string

const (
	WAFv2               Addon = "WAFv2"
	Shield              Addon = "Shield"
	ProvisionedCapacity Addon = "ProvisionedCapacity"
)

var (
	AllAddons = []Addon{WAFv2, Shield, ProvisionedCapacity}
)

type AddonMetadata struct {
	Name    Addon
	Enabled bool
}
