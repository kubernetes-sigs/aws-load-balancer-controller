package throttle

import (
	"context"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_matchService(t *testing.T) {
	type args struct {
		serviceID string
	}
	tests := []struct {
		name string
		args args
		ctx  context.Context
		want bool
	}{
		{
			name: "service matches",
			args: args{
				serviceID: "App Mesh",
			},
			ctx:  awsmiddleware.SetServiceID(context.TODO(), "App Mesh"),
			want: true,
		},
		{
			name: "service mismatches",
			args: args{
				serviceID: "App Mesh",
			},
			ctx:  awsmiddleware.SetServiceID(context.TODO(), "Some Service"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			predict := matchService(tt.args.serviceID)
			got := predict(tt.ctx)
			assert.Equal(t, tt.want, got)
		})
	}
}
