package runtime

import "github.com/go-logr/logr"

// NewConciseLogger constructs new conciseLogger
func NewConciseLogger(logger logr.Logger) *conciseLogger {
	return &conciseLogger{
		Logger: logger,
	}
}

var _ logr.Logger = &conciseLogger{}

// conciseLogger will log concise Error messages.
// We have used github.com/pkg/errors extensively, when logged with zap logger, a full stacktrace is logged but it's usually unhelpful due to go routine usage.
// this conciseLogger will wrap the error inside a conciseError, so that only necessary error message is logged.
type conciseLogger struct {
	logr.Logger
}

func (r *conciseLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	r.Logger.Error(&conciseError{err: err}, msg, keysAndValues...)
}

func (r *conciseLogger) WithValues(keysAndValues ...interface{}) logr.Logger {
	return NewConciseLogger(r.Logger.WithValues(keysAndValues...))
}

func (r *conciseLogger) WithName(name string) logr.Logger {
	return NewConciseLogger(r.Logger.WithName(name))
}

var _ error = &conciseError{}

// conciseError will only contain concise error message.
type conciseError struct {
	err error
}

func (e *conciseError) Error() string {
	return e.err.Error()
}
