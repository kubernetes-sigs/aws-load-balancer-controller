package utils

import "fmt"

// constructs new MultiError
func NewMultiError(errs ...error) *MultiError {
	return &MultiError{errs: errs}
}

var _ error = &MultiError{}

type MultiError struct {
	errs []error
}

func (e *MultiError) Error() string {
	if len(e.errs) == 0 {
		return ""
	}
	var errMSGs []string
	for _, err := range e.errs {
		errMSGs = append(errMSGs, err.Error())
	}
	return fmt.Sprintf("multiple error: %v", errMSGs)
}
