package utils

import "time"

const (
	PollIntervalShort  = 2 * time.Second
	PollIntervalMedium = 10 * time.Second
	PollIntervalLong   = 30 * time.Second
	PollTimeoutShort   = 1 * time.Minute
	PollTimeoutMedium  = 5 * time.Minute
	PollTimeoutLong    = 15 * time.Minute

	// IngressReconcileTimeout is the timeout we expected the controller finishes reconcile for Ingresses.
	IngressReconcileTimeout = 1 * time.Minute
	// IngressDNSAvailableWaitTimeout is the timeout we expect the DNS records of ALB to be propagated.
	IngressDNSAvailableWaitTimeout = 3 * time.Minute
)
