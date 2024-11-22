package aws

import (
	"errors"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"

	"testing"
)

func Test_errorCodeForRequest(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "requests without error",
			args: args{
				err: nil,
			},
			want: "",
		},
		{
			name: "requests with internal error",
			args: args{
				err: errors.New("oops, some internal error"),
			},
			want: "internal",
		},
		{
			name: "requests with aws error",
			args: args{
				err: &smithy.GenericAPIError{Code: "NotFoundException", Message: ""},
			},
			want: "NotFoundException",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errorCodeForRequest(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
