package ec2

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_isAddressInUseError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "is InvalidIPAddress.InUse error",
			args: args{
				err: awserr.New("InvalidIPAddress.InUse", "some message", nil),
			},
			want: true,
		},
		{
			name: "wraps InvalidIPAddress.InUse error",
			args: args{
				err: errors.Wrap(awserr.New("InvalidIPAddress.InUse", "some message", nil), "wrapped message"),
			},
			want: true,
		},
		{
			name: "isn't InvalidIPAddress.InUse error",
			args: args{
				err: errors.New("some other error"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAddressInUseError(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
