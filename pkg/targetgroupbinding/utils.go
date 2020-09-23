package targetgroupbinding

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1alpha1"
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

func buildServiceReferenceKey(tgb *elbv2api.TargetGroupBinding, svcRef elbv2api.ServiceReference) types.NamespacedName {
	return types.NamespacedName{
		Namespace: tgb.Namespace,
		Name:      svcRef.Name,
	}
}
