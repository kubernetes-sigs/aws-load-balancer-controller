package albctx

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

type contextKey string

var (
	contextKeyEventf = contextKey("Eventf")
	contextKeyLogger = contextKey("Logger")
)

type eventf func(string, string, string, ...interface{})

func missingEventf(eventType, reason, format string, vals ...interface{}) {
	f := fmt.Sprintf("Event function missing. Type(%v) Reason(%v): %v", eventType, reason, format)
	glog.Errorf(f, vals...)
}

func SetEventf(ctx context.Context, f eventf) context.Context {
	return context.WithValue(ctx, contextKeyEventf, f)
}

func GetEventf(ctx context.Context) eventf {
	if f, ok := ctx.Value(contextKeyEventf).(eventf); ok {
		return f
	}
	return missingEventf
}

func SetLogger(ctx context.Context, logger *log.Logger) context.Context {
	return context.WithValue(ctx, contextKeyLogger, logger)
}

func GetLogger(ctx context.Context) *log.Logger {
	logger, ok := ctx.Value(contextKeyLogger).(*log.Logger)
	if !ok {
		return log.New("UNKNOWN")
	}
	return logger
}
