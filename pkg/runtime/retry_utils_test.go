package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_RetryImmediateOnError(t *testing.T) {
	retryable := func(err error) bool {
		return err.Error() == "retryable"
	}

	failureAfterRetryCountFnGen := func(x int) func() error {
		return func() error {
			x = x - 1
			if x < 0 {
				return errors.New("failure")
			}
			return errors.New("retryable")
		}
	}
	successAfterRetryCountFnGen := func(x int) func() error {
		return func() error {
			x = x - 1
			if x < 0 {
				return nil
			}
			return errors.New("retryable")
		}
	}

	type args struct {
		interval  time.Duration
		timeout   time.Duration
		retryable func(error) bool
		fn        func() error
	}
	tests := []struct {
		name         string
		args         args
		wantMinCount int
		wantMaxCount int
		wantErr      error
	}{
		{
			name: "retry 4 times before failure",
			args: args{
				interval:  50 * time.Millisecond,
				timeout:   500 * time.Millisecond,
				retryable: retryable,
				fn:        failureAfterRetryCountFnGen(4),
			},
			wantMinCount: 5,
			wantMaxCount: 5,
			wantErr:      errors.New("failure"),
		},
		{
			name: "retry 4 times before failure - but timeout after 2nd retry",
			args: args{
				interval:  50 * time.Millisecond,
				timeout:   120 * time.Millisecond,
				retryable: retryable,
				fn:        failureAfterRetryCountFnGen(4),
			},
			// PollImmediate calls fn at t=0, t=50ms, t=100ms. Timeout at 120ms
			// may race with the next tick, so count can be 3 or 4.
			wantMinCount: 3,
			wantMaxCount: 4,
			wantErr:      errors.New("timed out waiting for the condition"),
		},
		{
			name: "retry 4 times before success",
			args: args{
				interval:  50 * time.Millisecond,
				timeout:   500 * time.Millisecond,
				retryable: retryable,
				fn:        successAfterRetryCountFnGen(4),
			},
			wantMinCount: 5,
			wantMaxCount: 5,
			wantErr:      nil,
		},
		{
			name: "retry 4 times before success - but timeout after 2nd retry",
			args: args{
				interval:  50 * time.Millisecond,
				timeout:   120 * time.Millisecond,
				retryable: retryable,
				fn:        successAfterRetryCountFnGen(4),
			},
			// PollImmediate calls fn at t=0, t=50ms, t=100ms. Timeout at 120ms
			// may race with the next tick, so count can be 3 or 4.
			wantMinCount: 3,
			wantMaxCount: 4,
			wantErr:      errors.New("timed out waiting for the condition"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := 0
			err := RetryImmediateOnError(tt.args.interval, tt.args.timeout, tt.args.retryable, func() error {
				count += 1
				return tt.args.fn()
			})
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.GreaterOrEqual(t, count, tt.wantMinCount, "retry count below minimum")
			assert.LessOrEqual(t, count, tt.wantMaxCount, "retry count above maximum")
		})
	}
}
