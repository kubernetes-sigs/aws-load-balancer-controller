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

// clearDryRunPlanAnnotation removes the dry-run-plan annotation from an ingress
// if it's currently set. This is a no-op when the annotation is absent, so the
// patch is skipped and no API call is made. Used by the group controller to
// clean up stale plan annotations on non-primary members when the group's
// membership shifts (so the migration console sees exactly one holder).
func clearDryRunPlanAnnotation(ctx context.Context, k8sClient client.Client, ing *networking.Ingress) error {
	if _, ok := ing.Annotations[dryRunPlanAnnotation]; !ok {
		return nil
	}
	ingOld := ing.DeepCopy()
	delete(ing.Annotations, dryRunPlanAnnotation)
	if err := k8sClient.Patch(ctx, ing, client.MergeFrom(ingOld)); err != nil {
		return fmt.Errorf("failed to clear dry-run plan annotation on ingress %s: %w", k8s.NamespacedName(ing), err)
	}
	return nil
}
