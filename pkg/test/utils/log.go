package utils

import (
	"github.com/onsi/ginkgo"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewGinkgoLogger returns new zap logger with ginkgo backend.
func NewGinkgoLogger() *zap.Logger {
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(ginkgo.GinkgoWriter),
		zap.InfoLevel,
	)
	return zap.New(core)
}
