package utils

import (
	"fmt"

	httpexpectv2 "github.com/gavv/httpexpect/v2"
	"github.com/go-logr/logr"
	ginkgov2 "github.com/onsi/ginkgo/v2"
)

type GinkgoLogger interface {
	httpexpectv2.LoggerReporter
}

var _ GinkgoLogger = &DefaultGinkgoLogger{}

type DefaultGinkgoLogger struct {
	Logger logr.Logger
}

func (l *DefaultGinkgoLogger) Logf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Logger.Info(message)
}

func (l *DefaultGinkgoLogger) Errorf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	ginkgov2.Fail(message)
}
