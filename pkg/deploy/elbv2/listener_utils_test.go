package elbv2

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_isListenerNotFoundError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "is ListenerNotFound error",
			args: args{
				err: awserr.New("ListenerNotFound", "some message", nil),
			},
			want: true,
		},
		{
			name: "wraps ListenerNotFound error",
			args: args{
				err: errors.Wrap(awserr.New("ListenerNotFound", "some message", nil), "wrapped message"),
			},
			want: true,
		},
		{
			name: "isn't ListenerNotFound error",
			args: args{
				err: errors.New("some other error"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isListenerNotFoundError(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
