package runtime

import (
	"time"
)

// NewRequeueError constructs new RequeueError to
// instruct controller-runtime to requeue the processing item without been logged as error.
func NewRequeueError(err error) *RequeueError {
	return &RequeueError{
		err: err,
	}
}

// NewRequeueAfterError constructs new RequeueAfterError to
// instruct controller-runtime to requeue the processing item after specified duration without been logged as error.
func NewRequeueAfterError(err error, duration time.Duration) *RequeueAfterError {
	return &RequeueAfterError{
		err:      err,
		duration: duration,
	}
}

var _ error = &RequeueError{}

// An error to instruct controller-runtime to requeue the processing item without been logged as error.
// This should be used when a "error condition" occurrence is sort of expected and can be resolved by retry.
// e.g. a dependency haven't been fulfilled yet.
type RequeueError struct {
	err error
}

func (e *RequeueError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *RequeueError) Unwrap() error {
	return e.err
}

var _ error = &RequeueAfterError{}

// An error to instruct controller-runtime to requeue the processing item after specified duration without been logged as error.
// This should be used when a "error condition" occurrence is sort of expected and can be resolved by retry.
// e.g. a dependency haven't been fulfilled yet, and expected it to be fulfilled after duration.
// Note: use this with care,a simple wait might suits your use case better.
type RequeueAfterError struct {
	err      error
	duration time.Duration
}

func (e *RequeueAfterError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *RequeueAfterError) Duration() time.Duration {
	return e.duration
}

func (e *RequeueAfterError) Unwrap() error {
	return e.err
}
