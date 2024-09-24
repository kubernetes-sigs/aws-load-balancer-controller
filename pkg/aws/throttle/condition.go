package throttle

import (
	"context"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"regexp"
)

type Condition func(ctx context.Context) bool

func matchService(serviceID string) Condition {
	return func(ctx context.Context) bool {
		return awsmiddleware.GetServiceID(ctx) == serviceID
	}
}

func matchServiceOperation(serviceID string, operation string) Condition {
	return func(ctx context.Context) bool {
		return awsmiddleware.GetServiceID(ctx) == serviceID && awsmiddleware.GetOperationName(ctx) == operation
	}
}

func matchServiceOperationPattern(serviceID string, operationPtn *regexp.Regexp) Condition {
	return func(ctx context.Context) bool {
		return awsmiddleware.GetServiceID(ctx) == serviceID && operationPtn.Match([]byte(awsmiddleware.GetOperationName(ctx)))
	}
}
