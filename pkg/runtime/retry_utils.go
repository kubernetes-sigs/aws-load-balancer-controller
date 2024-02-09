package runtime

import (
	"k8s.io/apimachinery/pkg/util/wait"
	"time"
)

// RetryImmediateOnError tries to run fn every interval until it succeeds, a non-retryable error occurs or the timeout is reached.
func RetryImmediateOnError(interval time.Duration, timeout time.Duration, retryable func(error) bool, fn func() error) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		err := fn()
		if err != nil {
			if retryable(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}
