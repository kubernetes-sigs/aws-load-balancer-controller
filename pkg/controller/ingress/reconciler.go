package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const FinalizerAWSALBIngressController = "ingress.k8s.aws/ingress-finalizer"

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, cloud cloud.Cloud, config ingress.Config,
	ingGroupBuilder ingress.GroupBuilder, modelBuilder build.Builder, modelDeployer deploy.Deployer) reconcile.Reconciler {

	return &ReconcileIngress{client: mgr.GetClient(), ingGroupBuilder: ingGroupBuilder, modelBuilder: modelBuilder, modelDeployer: modelDeployer}
}

var _ reconcile.Reconciler = &ReconcileIngress{}

// ReconcileIngress reconciles a Ingress object
type ReconcileIngress struct {
	client          client.Client
	ingGroupBuilder ingress.GroupBuilder
	modelBuilder    build.Builder
	modelDeployer   deploy.Deployer
}

func (r *ReconcileIngress) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	groupID := ingress.DecodeGroupIDFromReconcileRequest(request)
	ctx := logging.WithLogger(context.TODO(), log.Log.WithName(groupID.String()))

	group, err := r.ingGroupBuilder.Build(ctx, groupID)
	if err != nil {
		return reconcile.Result{}, err
	}

	activeMembers := make([]string, 0, len(group.ActiveMembers))
	for _, ing := range group.ActiveMembers {
		activeMembers = append(activeMembers, k8s.NamespacedName(ing).String())
	}
	leavingMembers := make([]string, 0, len(group.LeavingMembers))
	for _, ing := range group.LeavingMembers {
		leavingMembers = append(leavingMembers, k8s.NamespacedName(ing).String())
	}

	//addedFinalizer := false
	//for _, ing := range group.ActiveMembers {
	//	if !algorithm.ContainsString(ing.ObjectMeta.Finalizers, FinalizerAWSALBIngressController) {
	//		ing.ObjectMeta.Finalizers = append(ing.ObjectMeta.Finalizers, FinalizerAWSALBIngressController)
	//		if err := r.client.Update(ctx, ing); err != nil {
	//			return reconcile.Result{}, err
	//		}
	//		addedFinalizer = true
	//	}
	//}
	//if addedFinalizer {
	//	return reconcile.Result{}, nil
	//}

	logging.FromContext(ctx).Info("start reconcile", "groupID", group.ID.String(),
		"activeMembers", activeMembers, "leavingMembers", leavingMembers)
	model, err := r.modelBuilder.Build(ctx, group)
	if err != nil {
		return reconcile.Result{}, err
	}

	logging.FromContext(ctx).Info("successfully built model", "groupID", group.ID.String())

	payload, err := json.Marshal(model)
	fmt.Println(string(payload))

	lbDNS, err := r.modelDeployer.Deploy(ctx, model)
	if err != nil {
		return reconcile.Result{}, err
	}
	logging.FromContext(ctx).Info("successfully deployed model", "groupID", group.ID.String())

	if lbDNS != "" {
		for _, ing := range group.ActiveMembers {
			if err := r.updateIngressStatus(ctx, ing, lbDNS); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	//for _, ing := range group.LeavingMembers {
	//	if algorithm.ContainsString(ing.ObjectMeta.Finalizers, FinalizerAWSALBIngressController) {
	//		ing.ObjectMeta.Finalizers = algorithm.RemoveString(ing.ObjectMeta.Finalizers, FinalizerAWSALBIngressController)
	//		if err := r.client.Update(ctx, ing); err != nil {
	//			return reconcile.Result{}, err
	//		}
	//	}
	//}

	return reconcile.Result{}, nil
}

func (r *ReconcileIngress) updateIngressStatus(ctx context.Context, ingress *extensions.Ingress, lbDNS string) error {
	if len(ingress.Status.LoadBalancer.Ingress) != 1 ||
		ingress.Status.LoadBalancer.Ingress[0].IP != "" ||
		ingress.Status.LoadBalancer.Ingress[0].Hostname != lbDNS {
		ingress.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				Hostname: lbDNS,
			},
		}
		return r.client.Status().Update(ctx, ingress)
	}
	return nil
}
