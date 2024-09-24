package endpoints

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAWSEndpointResolver_EndpointFor(t *testing.T) {
	configuration := map[string]string{
		ec2.ServiceID:                    "https://ec2.domain.com",
		elasticloadbalancingv2.ServiceID: "https://elbv2.domain.com",
	}
	c := &Resolver{
		configuration: configuration,
	}

	type args struct {
		val string
	}

	tests := []struct {
		name    string
		args    args
		want    *string
		wantErr error
	}{
		{
			name: "when custom endpoint is configured",
			args: args{
				val: ec2.ServiceID,
			},
			want: aws.String(configuration[ec2.ServiceID]),
		},
		{
			name: "when custom endpoint is unconfigured",
			args: args{
				val: wafv2.ServiceID,
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := c.EndpointFor(tt.args.val)
			assert.Equal(t, tt.want, res)
		})
	}
}
