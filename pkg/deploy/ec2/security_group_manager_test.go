package ec2

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_isSecurityGroupDependencyViolationError(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "is DependencyViolation error",
			args: args{
				err: awserr.New("DependencyViolation", "some message", nil),
			},
			want: true,
		},
		{
			name: "wraps DependencyViolation error",
			args: args{
				err: errors.Wrap(awserr.New("DependencyViolation", "some message", nil), "wrapped message"),
			},
			want: true,
		},
		{
			name: "isn't DependencyViolation error",
			args: args{
				err: errors.New("some other error"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSecurityGroupDependencyViolationError(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
