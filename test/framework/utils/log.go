package utils

import (
	"fmt"

	httpexpectv2 "github.com/gavv/httpexpect/v2"
	"github.com/go-logr/logr"
	ginkgov2 "github.com/onsi/ginkgo/v2"
	zapraw "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type GinkgoLogger interface {
	logr.Logger
	httpexpectv2.LoggerReporter
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
	ginkgov2.Fail(message)
}

// NewGinkgoLogger returns new logger with ginkgo backend.
func NewGinkgoLogger() GinkgoLogger {
	encoder := zapcore.NewJSONEncoder(zapraw.NewProductionEncoderConfig())

	logger := zap.New(zap.UseDevMode(false),
		zap.Level(zapraw.InfoLevel),
		zap.WriteTo(ginkgov2.GinkgoWriter),
		zap.Encoder(encoder))
	return &defaultGinkgoLogger{Logger: logger}
}
