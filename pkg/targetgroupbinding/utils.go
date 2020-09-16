package targetgroupbinding

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
)

// Index Key for "ServiceReference" index.
const IndexKeyServiceRefName = "spec.serviceRef.name"
const IndexKeyTargetType = "spec.targetType"

// Index Func for "ServiceReference" index.
func IndexFuncServiceRefName(obj runtime.Object) []string {
	tgb := obj.(*elbv2api.TargetGroupBinding)
	return []string{tgb.Spec.ServiceRef.Name}
}

func IndexFuncTargetType(obj runtime.Object) []string {
	tgb := obj.(*elbv2api.TargetGroupBinding)
	targetType := ""
	if tgb.Spec.TargetType != nil {
		targetType = string(*tgb.Spec.TargetType)
	}
	return []string{targetType}
}

func IsTargetGroupNotFoundError(err error) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == "TargetGroupNotFound"
	}
	return false
}

func buildServiceReferenceKey(tgb *elbv2api.TargetGroupBinding, svcRef elbv2api.ServiceReference) types.NamespacedName {
	return types.NamespacedName{
		Namespace: tgb.Namespace,
		Name:      svcRef.Name,
	}
}
