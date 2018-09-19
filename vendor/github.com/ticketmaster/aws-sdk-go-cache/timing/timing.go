package timing

import (
	"context"
	"time"
)

// Data contains the request timing information
type Data struct {
	connStart time.Time
}

// SetConnectionStart is used to set the start time of the measured request
func (t *Data) SetConnectionStart(s time.Time) {
	t.connStart = s
}

// RequestDuration returns the duration of the API call
func (t *Data) RequestDuration() time.Duration {
	return time.Now().Sub(t.connStart)
}

// GetData returns the timing data from the provided context
func GetData(ctx context.Context) *Data {
	d := ctx.Value(timingContextKey)
	if d == nil {
		return nil
	}
	return ctx.Value(timingContextKey).(*Data)
}
