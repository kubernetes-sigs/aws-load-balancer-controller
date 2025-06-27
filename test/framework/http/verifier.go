package http

import (
	"crypto/tls"
	gohttp "net/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

// Verifier is responsible for verify the behavior of an HTTP endpoint.
type Verifier interface {
	VerifyURL(url string, matchers ...Matcher) error
}

func NewDefaultVerifier() *defaultVerifier {
	httpClient := &gohttp.Client{}
	httpClient.Transport = &gohttp.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	return &defaultVerifier{
		httpClient: httpClient,
	}
}

var _ Verifier = &defaultVerifier{}

// default implementation for Verifier.
type defaultVerifier struct {
	httpClient *gohttp.Client
}

func (v *defaultVerifier) VerifyURL(url string, matchers ...Matcher) error {
	goResp, err := v.httpClient.Get(url)
	if err != nil {
		return err
	}
	resp, err := buildResponse(goResp)
	if err != nil {
		return err
	}

	var matchErrs []error
	for _, matcher := range matchers {
		if err := matcher.Matches(resp); err != nil {
			matchErrs = append(matchErrs, err)
		}
	}
	if len(matchErrs) != 0 {
		return utils.NewMultiError(matchErrs...)
	}
	return nil
}
