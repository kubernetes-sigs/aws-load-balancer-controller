/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/elbv2/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1alpha1"
)

const (
	targetGroupBindingFinalizer = "elbv2.k8s.aws/resources"
)

// NewTargetGroupBindingReconciler constructs new targetGroupBindingReconciler
func NewTargetGroupBindingReconciler(k8sClient client.Client, finalizerManager k8s.FinalizerManager, tgbResourceManager targetgroupbinding.ResourceManager, logger logr.Logger) *targetGroupBindingReconciler {
	return &targetGroupBindingReconciler{
		k8sClient:          k8sClient,
		finalizerManager:   finalizerManager,
		tgbResourceManager: tgbResourceManager,
		logger:             logger,
	}
}

// targetGroupBindingReconciler reconciles a TargetGroupBinding object
type targetGroupBindingReconciler struct {
	k8sClient          client.Client
	finalizerManager   k8s.FinalizerManager
	tgbResourceManager targetgroupbinding.ResourceManager
	logger             logr.Logger
}

// +kubebuilder:rbac:groups=elbv2.k8s.aws,resources=targetgroupbindings,verbs=get;list;watch;update;patch;create;delete
// +kubebuilder:rbac:groups=elbv2.k8s.aws,resources=targetgroupbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;update;patch;create;delete

func (r *targetGroupBindingReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(req), r.logger)
}

func (r *targetGroupBindingReconciler) reconcile(req ctrl.Request) error {
	ctx := context.Background()
	tgb := &elbv2api.TargetGroupBinding{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, tgb); err != nil {
		return client.IgnoreNotFound(err)
	}

	if !tgb.DeletionTimestamp.IsZero() {
		return r.cleanupTargetGroupBinding(ctx, tgb)
	}
	return r.reconcileTargetGroupBinding(ctx, tgb)
}

func (r *targetGroupBindingReconciler) reconcileTargetGroupBinding(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	if err := r.finalizerManager.AddFinalizers(ctx, tgb, targetGroupBindingFinalizer); err != nil {
		return err
	}
	if err := r.tgbResourceManager.Reconcile(ctx, tgb); err != nil {
		return err
	}
	return nil
}

func (r *targetGroupBindingReconciler) cleanupTargetGroupBinding(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	if k8s.HasFinalizer(tgb, targetGroupBindingFinalizer) {
		if err := r.tgbResourceManager.Cleanup(ctx, tgb); err != nil {
			return err
		}
		if err := r.finalizerManager.RemoveFinalizers(ctx, tgb, targetGroupBindingFinalizer); err != nil {
			return err
		}
	}
	return nil
}

func (r *targetGroupBindingReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := r.setupIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return err
	}

	epEventsHandler := eventhandlers.NewEnqueueRequestsForEndpointsEvent(r.k8sClient,
		r.logger.WithName("eventHandlers").WithName("endpoints"))
	nodeEventsHandler := eventhandlers.NewEnqueueRequestsForNodeEvent(r.k8sClient,
		r.logger.WithName("eventHandlers").WithName("node"))
	return ctrl.NewControllerManagedBy(mgr).
		For(&elbv2api.TargetGroupBinding{}).
		Watches(&source.Kind{Type: &corev1.Endpoints{}}, epEventsHandler).
		Watches(&source.Kind{Type: &corev1.Node{}}, nodeEventsHandler).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		Complete(r)
}

func (r *targetGroupBindingReconciler) setupIndexes(ctx context.Context, fieldIndexer client.FieldIndexer) error {
	if err := fieldIndexer.IndexField(ctx, &elbv2api.TargetGroupBinding{},
		targetgroupbinding.IndexKeyServiceRefName, targetgroupbinding.IndexFuncServiceRefName); err != nil {
		return err
	}
	if err := fieldIndexer.IndexField(ctx, &elbv2api.TargetGroupBinding{},
		targetgroupbinding.IndexKeyTargetType, targetgroupbinding.IndexFuncTargetType); err != nil {
		return err
	}
	return nil
}
