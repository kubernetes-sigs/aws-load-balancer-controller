package gateway

import (
	"context"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler interface {
	Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error)
	SetupWithManager(ctx context.Context, mgr ctrl.Manager) (controller.Controller, error)
	SetupWatches(ctx context.Context, controller controller.Controller, mgr ctrl.Manager, clientSet *kubernetes.Clientset) error
}
