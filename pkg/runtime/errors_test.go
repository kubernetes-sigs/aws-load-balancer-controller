package runtime

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestNewRequeueError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name       string
		args       args
		wantErr    string
		wantUnwrap error
	}{
		{
			name: "wraps non-nil error",
			args: args{
				err: errors.New("some error"),
			},
			wantErr:    "some error",
			wantUnwrap: errors.New("some error"),
		},
		{
			name: "wraps nil error",
			args: args{
				err: nil,
			},
			wantErr:    "",
			wantUnwrap: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewRequeueError(tt.args.err)
			assert.Equal(t, tt.wantErr, got.Error())
			if tt.wantUnwrap != nil {
				assert.EqualError(t, got.Unwrap(), tt.wantUnwrap.Error())
			} else {
				assert.NoError(t, got.Unwrap())
			}
		})
	}
}

func TestNewRequeueAfterError(t *testing.T) {
	type args struct {
		err      error
		duration time.Duration
	}
	tests := []struct {
		name         string
		args         args
		wantErr      string
		wantUnwrap   error
		wantDuration time.Duration
	}{
		{
			name: "wraps non-nil error",
			args: args{
				err:      errors.New("some error"),
				duration: 3 * time.Second,
			},
			wantErr:      "some error",
			wantUnwrap:   errors.New("some error"),
			wantDuration: 3 * time.Second,
		},
		{
			name: "wraps nil error",
			args: args{
				err:      nil,
				duration: 3 * time.Second,
			},
			wantErr:      "",
			wantUnwrap:   nil,
			wantDuration: 3 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewRequeueAfterError(tt.args.err, tt.args.duration)
			assert.Equal(t, tt.wantErr, got.Error())
			if tt.wantUnwrap != nil {
				assert.EqualError(t, got.Unwrap(), tt.wantUnwrap.Error())
			} else {
				assert.NoError(t, got.Unwrap())
			}
			assert.Equal(t, 3*time.Second, got.Duration())
		})
	}
}
