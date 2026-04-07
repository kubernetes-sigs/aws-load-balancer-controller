package aws

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GenerateAWSConfig_SetsHTTPClientTimeout(t *testing.T) {
	gen := NewAWSConfigGenerator(CloudConfig{
		Region:     "us-west-2",
		MaxRetries: 3,
	}, imds.EndpointModeStateIPv4, nil)

	cfg, err := gen.GenerateAWSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg.HTTPClient)

	httpClient, ok := cfg.HTTPClient.(*http.Client)
	require.True(t, ok, "HTTPClient should be *http.Client")
	assert.Equal(t, defaultAWSSDKClientTimeout, httpClient.Timeout)
}
