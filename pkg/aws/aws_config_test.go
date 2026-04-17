package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/version"
)

// staticEC2EndpointResolver redirects all ec2 calls to a fixed URL.
type staticEC2EndpointResolver struct{ url string }

func (r staticEC2EndpointResolver) ResolveEndpoint(_ context.Context, _ ec2.EndpointParameters) (smithyendpoints.Endpoint, error) {
	u, err := url.Parse(r.url)
	if err != nil {
		return smithyendpoints.Endpoint{}, err
	}
	return smithyendpoints.Endpoint{URI: *u}, nil
}

func TestGenerateAWSConfig_UserAgentHeader(t *testing.T) {
	tests := []struct {
		name       string
		gitVersion string
		wantUA     string
	}{
		{
			name:       "user agent is exactly elbv2.k8s.aws/<version>",
			gitVersion: "v2.14.1",
			wantUA:     "elbv2.k8s.aws/v2.14.1",
		},
		{
			name:       "user agent reflects updated git version",
			gitVersion: "v3.0.0",
			wantUA:     "elbv2.k8s.aws/v3.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version.GitVersion = tt.gitVersion

			var capturedUA string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedUA = r.Header.Get("User-Agent")
				w.WriteHeader(http.StatusBadRequest)
			}))
			defer srv.Close()

			gen := NewAWSConfigGenerator(CloudConfig{Region: "us-east-1", MaxRetries: 1}, imds.EndpointModeStateIPv4, nil)
			awsCfg, err := gen.GenerateAWSConfig(
				config.WithCredentialsProvider(aws.AnonymousCredentials{}),
			)
			require.NoError(t, err)

			ec2Client := ec2.NewFromConfig(awsCfg, func(o *ec2.Options) {
				o.EndpointResolverV2 = staticEC2EndpointResolver{url: srv.URL}
			})
			_, _ = ec2Client.DescribeInstances(t.Context(), &ec2.DescribeInstancesInput{})

			require.NotEmpty(t, capturedUA, "no request was captured")
			assert.Equal(t, tt.wantUA, capturedUA)
		})
	}
}
