package runtime

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// RetryImmediateOnError tries to run fn every interval until it succeeds, a non-retryable error occurs or the timeout is reached.
// If the timeout is reached while retrying, the returned error wraps both the timeout and the last retryable error.
func RetryImmediateOnError(interval time.Duration, timeout time.Duration, retryable func(error) bool, fn func() error) error {
	var lastRetryableErr error
	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		err := fn()
		if err != nil {
			if retryable(err) {
				lastRetryableErr = err
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if wait.Interrupted(err) && lastRetryableErr != nil {
		return fmt.Errorf("%w: %v", err, lastRetryableErr)
	}
	return err
}
