package http

import (
	"crypto/tls"
	gohttp "net/http"

	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

// URLOptions contains options for HTTP/HTTPS requests
type URLOptions struct {
	// TLS options
	InsecureSkipVerify bool

	// Request options
	Method          string
	HostHeader      string
	Headers         map[string]string
	FollowRedirects bool
}

// DefaultURLOptions provides reasonable defaults for URLOptions
func DefaultURLOptions() URLOptions {
	return URLOptions{
		InsecureSkipVerify: true,
		FollowRedirects:    true,
	}
}

// Verifier is responsible for verify the behavior of an HTTP endpoint.
type Verifier interface {
	VerifyURL(url string, matchers ...Matcher) error
	VerifyURLWithOptions(url string, options URLOptions, matchers ...Matcher) error
}

func NewDefaultVerifier() *defaultVerifier {
	httpClient := &gohttp.Client{}
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

	return v.VerifyURLWithOptions(url, DefaultURLOptions(), matchers...)
}

func (v *defaultVerifier) VerifyURLWithOptions(url string, options URLOptions, matchers ...Matcher) error {
	// Create a custom client for this request
	client := v.httpClient
	client.Transport = &gohttp.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: options.InsecureSkipVerify,
		},
	}

	// Create request with specified method (default to GET if not provided)
	method := options.Method
	if method == "" {
		method = "GET"
	}

	req, err := gohttp.NewRequest(method, url, nil)
	if err != nil {
		return err
	}

	// Set custom headers
	for k, v := range options.Headers {
		req.Header.Set(k, v)
	}

	// Set Host header if provided (overrides any value in Headers)
	if options.HostHeader != "" {
		req.Host = options.HostHeader
	}

	// Configure redirect behavior
	if !options.FollowRedirects {
		client.CheckRedirect = func(req *gohttp.Request, via []*gohttp.Request) error {
			return gohttp.ErrUseLastResponse // Stop following redirects
		}
	}

	// Execute request
	goResp, err := client.Do(req)
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
