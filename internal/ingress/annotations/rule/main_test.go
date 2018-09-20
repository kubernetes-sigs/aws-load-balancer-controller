package rule

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
				IgnoreHostHeader: aws.Bool(true),
			},
			Target: &Config{
				IgnoreHostHeader: aws.Bool(false),
			},
			ExpectedResult: &Config{
				IgnoreHostHeader: aws.Bool(true),
			},
		},
		{
			Source: &Config{
				IgnoreHostHeader: aws.Bool(false),
			},
			Target: &Config{
				IgnoreHostHeader: aws.Bool(true),
			},
			ExpectedResult: &Config{
				IgnoreHostHeader: aws.Bool(true),
			},
		},
	} {
		actualResult := tc.Source.Merge(tc.Target)
		assert.Equal(t, tc.ExpectedResult, actualResult)
	}
}
