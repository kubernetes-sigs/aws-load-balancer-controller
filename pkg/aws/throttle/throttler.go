package throttle

import (
	"context"
	smithymiddleware "github.com/aws/smithy-go/middleware"
	"golang.org/x/time/rate"
	"regexp"
)

const sdkHandlerRequestThrottle = "requestThrottle"

type conditionLimiter struct {
	condition Condition
	limiter   *rate.Limiter
}

type throttler struct {
	conditionLimiters []conditionLimiter
}

// NewThrottler constructs new request throttler instance.
func NewThrottler(config *ServiceOperationsThrottleConfig) *throttler {
	throttler := &throttler{}
	for serviceID, operationsThrottleConfigs := range config.value {
		for _, operationsThrottleConfig := range operationsThrottleConfigs {
			throttler = throttler.WithOperationPatternThrottle(
				serviceID,
				operationsThrottleConfig.operationPtn,
				operationsThrottleConfig.r,
				operationsThrottleConfig.burst)
		}
	}
	return throttler
}

func (t *throttler) WithConditionThrottle(condition Condition, r rate.Limit, burst int) *throttler {
	limiter := rate.NewLimiter(r, burst)
	t.conditionLimiters = append(t.conditionLimiters, conditionLimiter{
		condition: condition,
		limiter:   limiter,
	})
	return t
}

func (t *throttler) WithServiceThrottle(serviceID string, r rate.Limit, burst int) *throttler {
	return t.WithConditionThrottle(matchService(serviceID), r, burst)
}

func (t *throttler) WithOperationThrottle(serviceID string, operation string, r rate.Limit, burst int) *throttler {
	return t.WithConditionThrottle(matchServiceOperation(serviceID, operation), r, burst)
}

func (t *throttler) WithOperationPatternThrottle(serviceID string, operationPtn *regexp.Regexp, r rate.Limit, burst int) *throttler {
	return t.WithConditionThrottle(matchServiceOperationPattern(serviceID, operationPtn), r, burst)
}

/*
WithSDKRequestThrottleMiddleware is a middleware that applies client side rate limiting to the clients. This is added in finalize step of middleware stack
and is called before each request in middleware chain
*/
func WithSDKRequestThrottleMiddleware(throttler *throttler) func(stack *smithymiddleware.Stack) error {
	return func(stack *smithymiddleware.Stack) error {
		return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc(sdkHandlerRequestThrottle, func(
			ctx context.Context, input smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler,
		) (
			output smithymiddleware.FinalizeOutput, metadata smithymiddleware.Metadata, err error,
		) {
			throttler.beforeSign(ctx)
			return next.HandleFinalize(ctx, input)
		}), smithymiddleware.Before)
	}
}

// beforeSign is added to the Finalize step of middleware stack; called before each request
func (t *throttler) beforeSign(ctx context.Context) {
	for _, conditionLimiter := range t.conditionLimiters {
		if conditionLimiter.condition(ctx) {
			conditionLimiter.limiter.Wait(ctx)
		}
	}
}
