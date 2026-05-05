package ingress

import (
	"context"
	"fmt"

	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	dryRunPlanAnnotation = annotations.AnnotationPrefixIngress + "/" + annotations.IngressSuffixDryRunPlan
)

// patchDryRunPlanAnnotation writes the serialized stack JSON to the dry-run-plan
// annotation on the given ingress. It skips the patch if the value is unchanged
// to avoid unnecessary API calls and reconcile loops.
func patchDryRunPlanAnnotation(ctx context.Context, k8sClient client.Client, ing *networking.Ingress, planJSON string) error {
	if ing.Annotations[dryRunPlanAnnotation] == planJSON {
		return nil
	}
	ingOld := ing.DeepCopy()
	if ing.Annotations == nil {
		ing.Annotations = map[string]string{}
	}
	ing.Annotations[dryRunPlanAnnotation] = planJSON
	if err := k8sClient.Patch(ctx, ing, client.MergeFrom(ingOld)); err != nil {
		return fmt.Errorf("failed to patch dry-run plan annotation on ingress %s: %w", k8s.NamespacedName(ing), err)
	}
	return nil
}
