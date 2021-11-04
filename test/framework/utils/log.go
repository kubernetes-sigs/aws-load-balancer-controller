package utils

import (
	"fmt"
	"github.com/gavv/httpexpect/v2"
	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo"
	zapraw "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type GinkgoLogger interface {
	logr.Logger
	httpexpect.LoggerReporter
}

var _ GinkgoLogger = &defaultGinkgoLogger{}

type defaultGinkgoLogger struct {
	logr.Logger
}

func (l *defaultGinkgoLogger) Logf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Logger.Info(message)
}

func (l *defaultGinkgoLogger) Errorf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	ginkgo.Fail(message)
}

// NewGinkgoLogger returns new logger with ginkgo backend.
func NewGinkgoLogger() GinkgoLogger {
	encoder := zapcore.NewJSONEncoder(zapraw.NewProductionEncoderConfig())

	logger := zap.New(zap.UseDevMode(false),
		zap.Level(zapraw.InfoLevel),
		zap.WriteTo(ginkgo.GinkgoWriter),
		zap.Encoder(encoder))
	return &defaultGinkgoLogger{Logger: logger}
}
