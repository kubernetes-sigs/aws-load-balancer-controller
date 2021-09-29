package endpoints

import (
	"testing"

	awsendpoints "github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestAWSEndpointResolver_String(t *testing.T) {
	type fields struct {
		configuration map[string]string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "non-empty configuration",
			fields: fields{
				configuration: map[string]string{
					awsendpoints.Ec2ServiceID: "https://ec2.domain.com",
					awsendpoints.ElasticloadbalancingServiceID: "https://elbv2.domain.com",
				},
			},
			want: "ec2=https://ec2.domain.com,elasticloadbalancing=https://elbv2.domain.com",
		},
		{
			name: "nil configuration",
			fields: fields{
				configuration: nil,
			},
			want: "",
		},
		{
			name: "empty configuration",
			fields: fields{
				configuration: nil,
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AWSEndpointResolver{
				configuration: tt.fields.configuration,
			}
			got := c.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAWSEndpointResolver_Set(t *testing.T) {
	type fields struct {
		configuration map[string]string
	}
	type args struct {
		val string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    AWSEndpointResolver
		wantErr error
	}{
		{
			name: "when default value is nil",
			fields: fields{
				configuration: nil,
			},
			args: args{
				val: "ec2=https://ec2.domain.com,elasticloadbalancing=https://elbv2.domain.com",
			},
			want: AWSEndpointResolver{
				configuration: map[string]string{
					awsendpoints.Ec2ServiceID: "https://ec2.domain.com",
					awsendpoints.ElasticloadbalancingServiceID: "https://elbv2.domain.com",
				},
			},
		},
		{
			name: "when val is empty",
			fields: fields{
				configuration: map[string]string{},
			},
			args: args{
				val: "",
			},
			want: AWSEndpointResolver{
				configuration: map[string]string{},
			},
		},
		{
			name: "when val is not valid format - case 1",
			fields: fields{
				configuration: map[string]string{},
			},
			args: args{
				val: "a=b=c",
			},
			wantErr: errors.Errorf("a=b=c must be formatted as serviceID=URL"),
		},
		{
			name: "when url is not absolute",
			fields: fields{
				configuration: map[string]string{},
			},
			args: args{
				val: "a=/relative/url",
			},
			wantErr: errors.Errorf("/relative/url must be an absolute url"),
		},
		{
			name: "when url is invalid",
			fields: fields{
				configuration: map[string]string{},
			},
			args: args{
				val: "a=invalid\turl",
			},
			wantErr: errors.Errorf("invalid\turl must be a valid url"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AWSEndpointResolver{
				configuration: tt.fields.configuration,
			}
			err := c.Set(tt.args.val)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, *c)
			}
		})
	}
}

func TestAWSEndpointResolver_Type(t *testing.T) {
	c := &AWSEndpointResolver{}
	got := c.Type()
	assert.Equal(t, "awsEndpointResolver", got)
}

func TestAWSEndpointResolver_EndpointFor(t *testing.T) {
	configuration := map[string]string{
		awsendpoints.Ec2ServiceID: "https://ec2.domain.com",
		awsendpoints.ElasticloadbalancingServiceID: "https://elbv2.domain.com",
	}
	c := &AWSEndpointResolver{
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
