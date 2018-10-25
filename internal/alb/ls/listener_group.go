package ls

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/rs"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
)

type GroupController interface {
	// Reconcile ensures listeners exists in LB to satisfy ingress requirements.
	Reconcile(ctx context.Context, lbArn string, ingress *extensions.Ingress, tgGroup tg.TargetGroupGroup) error

	// Delete ensures all listeners are deleted
	Delete(ctx context.Context, lbArn string) error
}

func NewGroupController(store store.Storer, elbv2 albelbv2.ELBV2API, rulesController rs.Controller) GroupController {
	lsController := NewController(elbv2, store, rulesController)
	return &defaultGroupController{
		elbv2:        elbv2,
		store:        store,
		lsController: lsController,
	}
}

type defaultGroupController struct {
	elbv2 albelbv2.ELBV2API
	store store.Storer

	lsController Controller
}

func (controller *defaultGroupController) Reconcile(ctx context.Context, lbArn string, ingress *extensions.Ingress, tgGroup tg.TargetGroupGroup) error {
	ingressAnnos, err := controller.store.GetIngressAnnotations(k8s.MetaNamespaceKey(ingress))
	if err != nil {
		return err
	}
	instancesByPort, err := controller.loadListenerInstances(lbArn)
	if err != nil {
		return err
	}

	portsInUse := sets.NewInt64()
	for _, port := range ingressAnnos.LoadBalancer.Ports {
		portsInUse.Insert(port.Port)
		instance := instancesByPort[port.Port]
		if err := controller.lsController.Reconcile(ctx, ReconcileOptions{
			LBArn:        lbArn,
			Ingress:      ingress,
			IngressAnnos: ingressAnnos,
			Port:         port,
			TGGroup:      tgGroup,
			Instance:     instance,
		}); err != nil {
			return err
		}
	}
	portsUnsed := sets.Int64KeySet(instancesByPort).Difference(portsInUse)
	for port := range portsUnsed {
		instance := instancesByPort[port]
		if err := controller.elbv2.DeleteListenersByArn(aws.StringValue(instance.ListenerArn)); err != nil {
			return err
		}
	}
	return nil
}

func (controller *defaultGroupController) Delete(ctx context.Context, lbArn string) error {
	instancesByPort, err := controller.loadListenerInstances(lbArn)
	if err != nil {
		return err
	}
	for _, instance := range instancesByPort {
		if err := controller.elbv2.DeleteListenersByArn(aws.StringValue(instance.ListenerArn)); err != nil {
			return err
		}
	}
	return nil
}

func (controller *defaultGroupController) loadListenerInstances(lbArn string) (map[int64]*elbv2.Listener, error) {
	instances, err := controller.elbv2.ListListenersByLoadBalancer(lbArn)
	if err != nil {
		return nil, err
	}
	instanceByPort := make(map[int64]*elbv2.Listener)
	for _, instance := range instances {
		instanceByPort[aws.Int64Value(instance.Port)] = instance
	}
	return instanceByPort, nil
}
