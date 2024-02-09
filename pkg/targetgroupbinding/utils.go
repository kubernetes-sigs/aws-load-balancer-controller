package targetgroupbinding

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Prefix for TargetHealth pod condition type.
	TargetHealthPodConditionTypePrefix = "target-health.elbv2.k8s.aws"
	// Legacy Prefix for TargetHealth pod condition type(used by AWS ALB Ingress Controller)
	TargetHealthPodConditionTypePrefixLegacy = "target-health.alb.ingress.k8s.aws"

	// Index Key for "ServiceReference" index.
	IndexKeyServiceRefName = "spec.serviceRef.name"
)

// BuildTargetHealthPodConditionType constructs the condition type for TargetHealth pod condition.
func BuildTargetHealthPodConditionType(tgb *elbv2api.TargetGroupBinding) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s/%s", TargetHealthPodConditionTypePrefix, tgb.Name))
}

// IndexFuncServiceRefName is IndexFunc for "ServiceReference" index.
func IndexFuncServiceRefName(obj client.Object) []string {
	tgb := obj.(*elbv2api.TargetGroupBinding)
	return []string{tgb.Spec.ServiceRef.Name}
}

func buildServiceReferenceKey(tgb *elbv2api.TargetGroupBinding, svcRef elbv2api.ServiceReference) types.NamespacedName {
	return types.NamespacedName{
		Namespace: tgb.Namespace,
		Name:      svcRef.Name,
	}
}
