package runtime

import (
	"fmt"
	"time"
)

// NewRequeueNeeded constructs new RequeueError to
// instruct controller-runtime to requeue the processing item without been logged as error.
func NewRequeueNeeded(reason string) *RequeueNeeded {
	return &RequeueNeeded{
		reason: reason,
	}
}

// NewRequeueNeededAfter constructs new RequeueNeededAfter to
// instruct controller-runtime to requeue the processing item after specified duration without been logged as error.
func NewRequeueNeededAfter(reason string, duration time.Duration) *RequeueNeededAfter {
	return &RequeueNeededAfter{
		reason:   reason,
		duration: duration,
	}
}

var _ error = &RequeueNeeded{}

// An error to instruct controller-runtime to requeue the processing item without been logged as error.
// This should be used when a "error condition" occurrence is sort of expected and can be resolved by retry.
// e.g. a dependency haven't been fulfilled yet.
type RequeueNeeded struct {
	reason string
}

func (e *RequeueNeeded) Reason() string {
	return e.reason
}

func (e *RequeueNeeded) Error() string {
	return fmt.Sprintf("requeue needed: %v", e.reason)
}

var _ error = &RequeueNeededAfter{}

// An error to instruct controller-runtime to requeue the processing item after specified duration without been logged as error.
// This should be used when a "error condition" occurrence is sort of expected and can be resolved by retry.
// e.g. a dependency haven't been fulfilled yet, and expected it to be fulfilled after duration.
// Note: use this with care,a simple wait might suits your use case better.
type RequeueNeededAfter struct {
	reason   string
	duration time.Duration
}

func (e *RequeueNeededAfter) Reason() string {
	return e.reason
}

func (e *RequeueNeededAfter) Duration() time.Duration {
	return e.duration
}

func (e *RequeueNeededAfter) Error() string {
	return fmt.Sprintf("requeue needed after %v: %v", e.duration, e.reason)
}
