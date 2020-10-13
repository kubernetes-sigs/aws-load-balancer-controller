package utils

import (
	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo"
	zapraw "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// NewGinkgoLogger returns new logger with ginkgo backend.
func NewGinkgoLogger() logr.Logger {
	encoder := zapcore.NewJSONEncoder(zapraw.NewProductionEncoderConfig())

	return zap.New(zap.UseDevMode(false),
		zap.Level(zapraw.InfoLevel),
		zap.WriteTo(ginkgo.GinkgoWriter),
		zap.Encoder(encoder))
}
