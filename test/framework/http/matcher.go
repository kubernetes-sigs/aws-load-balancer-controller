package http

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
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
