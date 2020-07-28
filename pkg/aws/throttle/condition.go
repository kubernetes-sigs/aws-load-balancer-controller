package throttle

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"regexp"
)

type Condition func(r *request.Request) bool

func matchService(serviceID string) Condition {
	return func(r *request.Request) bool {
		return r.ClientInfo.ServiceID == serviceID
	}
}

func matchServiceOperation(serviceID string, operation string) Condition {
	return func(r *request.Request) bool {
		if r.Operation == nil {
			return false
		}
		return r.ClientInfo.ServiceID == serviceID && r.Operation.Name == operation
	}
}

func matchServiceOperationPattern(serviceID string, operationPtn *regexp.Regexp) Condition {
	return func(r *request.Request) bool {
		if r.Operation == nil {
			return false
		}
		return r.ClientInfo.ServiceID == serviceID && operationPtn.Match([]byte(r.Operation.Name))
	}
}
