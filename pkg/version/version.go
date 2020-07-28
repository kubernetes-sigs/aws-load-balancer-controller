package version

import (
	"encoding/json"
)

var (
	GitVersion string
	GitCommit  string
	BuildDate  string
)

// PrintableVersion returns formatted version string
func PrintableVersion() string {
	versionData := map[string]string{
		"GitVersion": GitVersion,
		"GitCommit":  GitCommit,
		"BuildDate":  BuildDate,
	}
	payload, _ := json.Marshal(versionData)
	return string(payload)
}
