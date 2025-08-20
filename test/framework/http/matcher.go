package http

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"strings"
)

// Matcher tests against specific HTTP behavior.
type Matcher interface {
	Matches(resp Response) error
}

// ResponseBodyMatches asserts HTTP response body matches.
func ResponseBodyMatches(expectedBody []byte) *responseBodyMatches {
	return &responseBodyMatches{
		expectedBody: expectedBody,
	}
}

var _ Matcher = &responseBodyMatches{}

type responseBodyMatches struct {
	expectedBody []byte
}

func (m *responseBodyMatches) Matches(resp Response) error {
	if cmp.Equal(resp.Body, m.expectedBody) {
		return nil
	}

	return errors.Errorf("Response Body mismatches, diff: %v", cmp.Diff(resp.Body, m.expectedBody))
}

// ResponseCodeMatches asserts HTTP response code matches
func ResponseCodeMatches(expectedResponseCode int) *responseCodeMatches {
	return &responseCodeMatches{
		expectedResponseCode: expectedResponseCode,
	}
}

var _ Matcher = &responseCodeMatches{}

type responseCodeMatches struct {
	expectedResponseCode int
}

func (m *responseCodeMatches) Matches(resp Response) error {
	if resp.ResponseCode == m.expectedResponseCode {
		return nil
	}
	return errors.Errorf("response code mismatch, want %v, got %v", m.expectedResponseCode, resp.ResponseCode)
}

// ResponseHeaderContains asserts HTTP response header contains expected value
func ResponseHeaderContains(headerName, expectedValue string) *responseHeaderContains {
	return &responseHeaderContains{
		headerName:    headerName,
		expectedValue: expectedValue,
	}
}

var _ Matcher = &responseHeaderContains{}

type responseHeaderContains struct {
	headerName    string
	expectedValue string
}

func (m *responseHeaderContains) Matches(resp Response) error {
	headerValues, exists := resp.Headers[m.headerName]
	if !exists {
		return errors.Errorf("Header %s not found in response", m.headerName)
	}

	// Check if any header value contains the expected substring
	for _, value := range headerValues {
		if strings.Contains(value, m.expectedValue) {
			return nil
		}
	}

	return errors.Errorf("Header %s values %v do not contain expected value %s",
		m.headerName, headerValues, m.expectedValue)
}
