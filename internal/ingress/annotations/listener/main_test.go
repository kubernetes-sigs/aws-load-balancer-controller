package listener

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

func TestMerge(t *testing.T) {
	for _, tc := range []struct {
		Source         *Config
		Target         *Config
		ExpectedResult *Config
	}{
		{
			Source: &Config{
				SslPolicy:      aws.String("SslPolicyA"),
				CertificateArn: aws.String("CertificateArnA"),
			},
			Target: &Config{
				SslPolicy:      aws.String("SslPolicyB"),
				CertificateArn: aws.String("CertificateArnB"),
			},
			ExpectedResult: &Config{
				SslPolicy:      aws.String("SslPolicyA"),
				CertificateArn: aws.String("CertificateArnA"),
			},
		},
		{
			Source: &Config{
				SslPolicy:      aws.String(""),
				CertificateArn: aws.String(""),
			},
			Target: &Config{
				SslPolicy:      aws.String("SslPolicyB"),
				CertificateArn: aws.String("CertificateArnB"),
			},
			ExpectedResult: &Config{
				SslPolicy:      aws.String("SslPolicyB"),
				CertificateArn: aws.String("CertificateArnB"),
			},
		},
	} {
		actualResult := tc.Source.Merge(tc.Target)
		assert.Equal(t, tc.ExpectedResult, actualResult)
	}
}
