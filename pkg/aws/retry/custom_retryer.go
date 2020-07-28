package retry

import (
	"github.com/aws/aws-sdk-go/aws/request"
)

// request option that configures a custom maxRetries.
func WithMaxRetries(maxRetries int) request.Option {
	return func(r *request.Request) {
		r.Retryer = &CustomRetryer{
			Retryer:       r.Retryer,
			numMaxRetries: maxRetries,
		}
	}
}

type CustomRetryer struct {
	request.Retryer
	numMaxRetries int
}

func (c *CustomRetryer) MaxRetries() int {
	return c.numMaxRetries
}
