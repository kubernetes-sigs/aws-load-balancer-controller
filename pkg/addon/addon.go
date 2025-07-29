package addon

type Addon string

const (
	WAFv2  Addon = "WAFv2"
	Shield Addon = "Shield"
)

var (
	AllAddons = []Addon{WAFv2, Shield}
)

type AddonMetadata struct {
	Name    Addon
	Enabled bool
}
