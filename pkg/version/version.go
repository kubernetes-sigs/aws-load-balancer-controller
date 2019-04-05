package version

import (
	"fmt"
)

var (
	// RELEASE returns the release version
	RELEASE = "UNKNOWN"
	// REPO returns the git repository URL
	REPO = "UNKNOWN"
	// COMMIT returns the short sha from git
	COMMIT = "UNKNOWN"
)

// String returns information about the release.
func String() string {
	return fmt.Sprintf(`-------------------------------------------------------------------------------
AWS ALB Ingress controller
  Release:    %v
  Build:      %v
  Repository: %v
-------------------------------------------------------------------------------
`, RELEASE, COMMIT, REPO)
}
