package runtime

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
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
		name      string
		args      args
		wantCount int
		wantErr   error
	}{
		{
			name: "retry 4 times before failure",
			args: args{
				interval:  10 * time.Millisecond,
				timeout:   100 * time.Millisecond,
				retryable: retryable,
				fn:        failureAfterRetryCountFnGen(4),
			},
			wantCount: 5,
			wantErr:   errors.New("failure"),
		},
		{
			name: "retry 4 times before failure - but timeout after 2nd retry",
			args: args{
				interval:  10 * time.Millisecond,
				timeout:   19 * time.Millisecond,
				retryable: retryable,
				fn:        failureAfterRetryCountFnGen(4),
			},
			wantCount: 3,
			wantErr:   errors.New("timed out waiting for the condition"),
		},
		{
			name: "retry 4 times before success",
			args: args{
				interval:  10 * time.Millisecond,
				timeout:   100 * time.Millisecond,
				retryable: retryable,
				fn:        successAfterRetryCountFnGen(4),
			},
			wantCount: 5,
			wantErr:   nil,
		},
		{
			name: "retry 4 times before success - but timeout after 2nd retry",
			args: args{
				interval:  10 * time.Millisecond,
				timeout:   19 * time.Millisecond,
				retryable: retryable,
				fn:        successAfterRetryCountFnGen(4),
			},
			wantCount: 3,
			wantErr:   errors.New("timed out waiting for the condition"),
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
			assert.Equal(t, tt.wantCount, count)
		})
	}
}
