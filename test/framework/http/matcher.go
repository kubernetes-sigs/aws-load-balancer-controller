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
