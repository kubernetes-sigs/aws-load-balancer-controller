package elbv2

import (
	"github.com/aws/smithy-go"
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
				err: &smithy.GenericAPIError{Code: "ListenerNotFound", Message: "some message"},
			},
			want: true,
		},
		{
			name: "wraps ListenerNotFound error",
			args: args{
				err: errors.Wrap(&smithy.GenericAPIError{Code: "ListenerNotFound", Message: "some message"}, "wrapped message"),
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
