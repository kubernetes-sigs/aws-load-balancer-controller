package albendpoints

import (
	"github.com/aws/aws-sdk-go/aws/endpoints"
)

// IsWAFRegionalAvailable return true only if WAFRegional service is available for the regionID
func IsWAFRegionalAvailable(regionID string) bool {
	resolver := endpoints.DefaultResolver()
	_, err := resolver.EndpointFor(endpoints.WafRegionalServiceID, regionID,
		func(opt *endpoints.Options) {
			opt.StrictMatching = true
		})

	return err == nil
}
