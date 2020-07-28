package retry

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWithMaxRetries(t *testing.T) {
	type args struct {
		maxRetries int
	}
	tests := []struct {
		name           string
		args           args
		wantMaxRetries int
	}{
		{
			name: "normal case",
			args: args{
				maxRetries: 10,
			},
			wantMaxRetries: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			option := WithMaxRetries(tt.args.maxRetries)
			r := &request.Request{}
			option(r)
			gotMaxRetries := r.MaxRetries()
			assert.Equal(t, tt.wantMaxRetries, gotMaxRetries)
		})
	}
}
