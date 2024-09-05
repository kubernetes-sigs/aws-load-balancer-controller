package throttle

// TODO : WIP
//import (
//	"github.com/aws/aws-sdk-go/aws/request"
//	smithyhttp "github.com/aws/smithy-go/transport/http"
//	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware/"
//	"regexp"
//)
//
//type Condition func(r *awshttp.) bool
//
//func matchService(serviceID string) Condition {
//	return func(r *awshttp.) bool {
//		return awsmiddleware.GetServiceID() == serviceID
//	}
//}
//
//func matchServiceOperation(serviceID string, operation string) Condition {
//	return func(r *smithyhttp.Request) bool {
//		if r.Operation == nil {
//			return false
//		}
//		return r.ClientInfo.ServiceID == serviceID && r.Operation.Name == operation
//	}
//}
//
//func matchServiceOperationPattern(serviceID string, operationPtn *regexp.Regexp) Condition {
//	return func(r *smithyhttp.Request) bool {
//		if r.Operation == nil {
//			return false
//		}
//		return r.ClientInfo.ServiceID == serviceID && operationPtn.Match([]byte(r.Operation.Name))
//	}
//}
