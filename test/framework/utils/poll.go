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
	// CertReconcileTimeout is the timeout we expect the controller finishes reconcile ACM certificates for it's Ingress objects
	CertReconcileTimeout = 30 * time.Minute
)
