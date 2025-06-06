package routeutils

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Implements helper function to add finalizer for target group configuration
func AddTargetGroupConfigurationFinalizer(ctx context.Context, k8sClient client.Client, tgConfig *elbv2gw.TargetGroupConfiguration) error {
	finalizer := shared_constants.TargetGroupConfigurationFinalizer
	// check if finalizer already exist
	if k8s.HasFinalizer(tgConfig, finalizer) {
		return nil
	}
	finalizerManager := k8s.NewDefaultFinalizerManager(k8sClient, logr.Discard())

	return finalizerManager.AddFinalizers(ctx, tgConfig, finalizer)
}

// RemoveTargetGroupConfigurationFinalizer removes target group configuration finalizer when service is deleted
func RemoveTargetGroupConfigurationFinalizer(ctx context.Context, svc *corev1.Service, k8sClient client.Client, logger logr.Logger, recorder record.EventRecorder) {
	tgConfig, err := LookUpTargetGroupConfiguration(ctx, k8sClient, k8s.NamespacedName(svc))
	if err != nil {
		logger.Error(err, "failed to look up target group configuration", "service", svc.Name)
		return
	}
	if tgConfig == nil {
		logger.V(1).Info("TargetGroupConfigurationNotFound, ignoring remove finalizer.", "TargetGroupConfiguration", svc.Name)
		return
	}

	tgFinalizer := shared_constants.TargetGroupConfigurationFinalizer
	if k8s.HasFinalizer(tgConfig, tgFinalizer) {
		finalizerManager := k8s.NewDefaultFinalizerManager(k8sClient, logr.Discard())
		if err := finalizerManager.RemoveFinalizers(ctx, tgConfig, tgFinalizer); err != nil {
			recorder.Event(tgConfig, corev1.EventTypeWarning, k8s.TargetGroupBindingEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed to remove target group configuration finalizer due to %v", err))
		}
		logger.V(1).Info("Successfully removed target group configuration finalizer.", "TargetGroupConfiguration", tgConfig.Name)
	}
}
