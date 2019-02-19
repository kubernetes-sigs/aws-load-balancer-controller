package utils

import "time"

const (
	// How often to Poll pods, nodes and claims.
	PollIntervalShort  = 2 * time.Second
	PollIntervalMedium = 10 * time.Second
)
