package ec2

import (
	"github.com/aws/smithy-go"
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
				err: &smithy.GenericAPIError{Code: "DependencyViolation", Message: "some message"},
			},
			want: true,
		},
		{
			name: "wraps DependencyViolation error",
			args: args{
				err: errors.Wrap(&smithy.GenericAPIError{Code: "DependencyViolation", Message: "some message"}, "wrapped message"),
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
