package endpoints

import (
	"testing"

	awsendpoints "github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/stretchr/testify/assert"
)

func TestAWSEndpointResolver_EndpointFor(t *testing.T) {
	configuration := map[string]string{
		awsendpoints.Ec2ServiceID:                  "https://ec2.domain.com",
		awsendpoints.ElasticloadbalancingServiceID: "https://elbv2.domain.com",
	}
	c := &resolver{
		configuration: configuration,
	}

	testRegion := "region"

	type args struct {
		val string
	}

	tests := []struct {
		name    string
		args    args
		want    *awsendpoints.ResolvedEndpoint
		wantErr error
	}{
		{
			name: "when custom endpoint is configured",
			args: args{
				val: awsendpoints.Ec2ServiceID,
			},
			want: &awsendpoints.ResolvedEndpoint{
				URL: configuration[awsendpoints.Ec2ServiceID],
			},
		},
		{
			name: "when custom endpoint is unconfigured",
			args: args{
				val: awsendpoints.WafServiceID,
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := c.EndpointFor(tt.args.val, testRegion)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				if tt.want != nil {
					assert.Equal(t, *tt.want, res)
				} else {
					defaultEndpoint, err := awsendpoints.DefaultResolver().EndpointFor(tt.args.val, testRegion)
					assert.NoError(t, err)
					assert.Equal(t, defaultEndpoint, res)
				}
			}
		})
	}
}
