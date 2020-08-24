package utils

import "time"

const (
	PollIntervalShort  = 2 * time.Second
	PollIntervalMedium = 10 * time.Second
	PollIntervalLong   = 30 * time.Second
	PollTimeoutShort   = 1 * time.Minute
	PollTimeoutMedium  = 5 * time.Minute
	PollTimeoutLong    = 10 * time.Minute
)
