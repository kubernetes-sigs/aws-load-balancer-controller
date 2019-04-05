package backend

import (
	"k8s.io/apimachinery/pkg/types"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
)

const (
	EndpointBindingRepoIndexStack   = "stack"
	EndpointBindingRepoIndexService = "service"
)

const (
	EndpointBindingLabelKeyStack = "stack"
)

func RepoIndexFuncStack(eb *api.EndpointBinding) []string {
	stackID, ok := eb.Labels[EndpointBindingLabelKeyStack]
	if ok {
		return []string{stackID}
	}
	return nil
}

func RepoIndexFuncService(eb *api.EndpointBinding) []string {
	svcKey := types.NamespacedName{
		Namespace: eb.Spec.ServiceRef.Namespace,
		Name:      eb.Spec.ServiceRef.Name,
	}.String()
	return []string{svcKey}
}
