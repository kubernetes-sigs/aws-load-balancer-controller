package networking

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_defaultSecurityGroupReconciler_shouldRetryWithoutCache(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should retry without cache when got duplicated permission error",
			args: args{
				err: awserr.New("InvalidPermission.Duplicate", "", nil),
			},
			want: true,
		},
		{
			name: "should retry without cache when got not found permission error",
			args: args{
				err: awserr.New("InvalidPermission.NotFound", "", nil),
			},
			want: true,
		},
		{
			name: "shouldn't retry when got some other error",
			args: args{
				err: awserr.New("SomeOtherError", "", nil),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &defaultSecurityGroupReconciler{}
			got := r.shouldRetryWithoutCache(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
