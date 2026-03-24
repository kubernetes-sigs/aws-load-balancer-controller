package throttle

import (
	"regexp"

	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/wafregional"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	"golang.org/x/time/rate"
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
			elasticloadbalancingv2.ServiceID: {
				{
					operationPtn: regexp.MustCompile("^RegisterTargets|^DeregisterTargets"),
					r:            rate.Limit(4),
					burst:        20,
				},
				{
					operationPtn: regexp.MustCompile("^Describe.*"),
					r:            rate.Limit(10),
					burst:        40,
				},
				{
					operationPtn: regexp.MustCompile("^Modify.*"),
					r:            rate.Limit(3),
					burst:        20,
				},
			},
		},
	}
}
