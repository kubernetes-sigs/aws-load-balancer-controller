package throttle

import (
	"github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/aws/aws-sdk-go/service/wafv2"
	"golang.org/x/time/rate"
	"regexp"
)

// NewDefaultServiceOperationsThrottleConfig returns a ServiceOperationsThrottleConfig with default settings.
func NewDefaultServiceOperationsThrottleConfig() *ServiceOperationsThrottleConfig {
	return &ServiceOperationsThrottleConfig{
		value: map[string][]throttleConfig{
			wafregional.ServiceID: {
				{
					operationPtn: regexp.MustCompile("^AssociateWebACL|DisassociateWebACL"),
					r:            rate.Limit(0.5),
					burst:        1,
				},
				{
					operationPtn: regexp.MustCompile("^GetWebACLForResource|ListResourcesForWebACL"),
					r:            rate.Limit(1),
					burst:        1,
				},
			},
			wafv2.ServiceID: {
				{
					operationPtn: regexp.MustCompile("^AssociateWebACL|DisassociateWebACL"),
					r:            rate.Limit(0.5),
					burst:        1,
				},
				{
					operationPtn: regexp.MustCompile("^GetWebACLForResource|ListResourcesForWebACL"),
					r:            rate.Limit(1),
					burst:        1,
				},
			},
		},
	}
}
