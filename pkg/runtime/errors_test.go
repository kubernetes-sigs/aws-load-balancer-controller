package runtime

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestNewRequeueNeeded(t *testing.T) {
	type args struct {
		msg string
	}
	tests := []struct {
		name       string
		args       args
		wantReason string
	}{
		{
			name: "standard case",
			args: args{
				msg: "some message",
			},
			wantReason: "some message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewRequeueNeeded(tt.args.msg)
			assert.Equal(t, tt.wantReason, got.Reason())
		})
	}
}

func TestNewRequeueNeededAfter(t *testing.T) {
	type args struct {
		msg      string
		duration time.Duration
	}
	tests := []struct {
		name         string
		args         args
		wantReason   string
		wantDuration time.Duration
	}{
		{
			name: "standard case",
			args: args{
				msg:      "some message",
				duration: 3 * time.Second,
			},
			wantReason:   "some message",
			wantDuration: 3 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewRequeueNeededAfter(tt.args.msg, tt.args.duration)
			assert.Equal(t, tt.wantReason, got.Reason())
			assert.Equal(t, 3*time.Second, got.Duration())
		})
	}
}
