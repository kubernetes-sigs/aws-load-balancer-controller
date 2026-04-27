package gateway

import (
	"context"

	"github.com/pkg/errors"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// isDryRunEnabled returns true when the Gateway requests dry-run mode via annotation.
func isDryRunEnabled(gw *gwv1.Gateway) bool {
	if gw == nil {
		return false
	}
	return gw.Annotations[gateway_constants.AnnotationDryRun] == gateway_constants.AnnotationDryRunEnabledValue
}

// hasDryRunPlanAnnotation returns true if the Gateway already carries a prior dry-run plan
// written by the controller.
func hasDryRunPlanAnnotation(gw *gwv1.Gateway) bool {
	if gw == nil {
		return false
	}
	_, exists := gw.Annotations[gateway_constants.AnnotationDryRunPlan]
	return exists
}

// reconcileDryRun handles the dry-run branch for the Gateway reconciler. It marshals the
// already-built stack and writes the JSON to the Gateway's dry-run-plan annotation.
// It intentionally skips all AWS deploy side-effects (finalizers, SG release, secrets
// monitoring, addon persistence, service reference counting).
func (r *gatewayReconciler) reconcileDryRun(ctx context.Context, gw *gwv1.Gateway, stack core.Stack) error {
	planJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		return err
	}

	if err := r.patchDryRunPlanAnnotation(ctx, gw, planJSON); err != nil {
		return err
	}

	r.logger.Info("dry-run plan generated", "gateway", k8s.NamespacedName(gw))
	return nil
}

// patchDryRunPlanAnnotation writes the serialized stack JSON to the Gateway's dry-run-plan
// annotation. Returns early if the value is unchanged to avoid reconcile loops.
func (r *gatewayReconciler) patchDryRunPlanAnnotation(ctx context.Context, gw *gwv1.Gateway, planJSON string) error {
	if gw.Annotations[gateway_constants.AnnotationDryRunPlan] == planJSON {
		return nil
	}
	gwOld := gw.DeepCopy()
	if gw.Annotations == nil {
		gw.Annotations = map[string]string{}
	}
	gw.Annotations[gateway_constants.AnnotationDryRunPlan] = planJSON
	if err := r.k8sClient.Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return errors.Wrapf(err, "failed to patch dry-run plan annotation on gateway %s", k8s.NamespacedName(gw))
	}
	return nil
}

// cleanupDryRunState removes the dry-run-plan annotation from a Gateway that no longer has
// dry-run enabled. It is a no-op if the annotation is not present.
func (r *gatewayReconciler) cleanupDryRunState(ctx context.Context, gw *gwv1.Gateway) error {
	if !hasDryRunPlanAnnotation(gw) {
		return nil
	}
	gwOld := gw.DeepCopy()
	delete(gw.Annotations, gateway_constants.AnnotationDryRunPlan)
	if err := r.k8sClient.Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return errors.Wrapf(err, "failed to remove dry-run plan annotation on gateway %s", k8s.NamespacedName(gw))
	}
	return nil
}
