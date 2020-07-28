package throttle

import (
	"github.com/aws/aws-sdk-go/aws/request"
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

func (t *throttler) InjectHandlers(handlers *request.Handlers) {
	handlers.Sign.PushFrontNamed(request.NamedHandler{
		Name: sdkHandlerRequestThrottle,
		Fn:   t.beforeSign,
	})
}

// beforeSign is added to the Sign chain; called before each request
func (t *throttler) beforeSign(r *request.Request) {
	for _, conditionLimiter := range t.conditionLimiters {
		if conditionLimiter.condition(r) {
			conditionLimiter.limiter.Wait(r.Context())
		}
	}
}
