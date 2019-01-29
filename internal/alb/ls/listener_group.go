package ls

import (
	"context"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/cert"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/auth"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/tls"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
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

func NewGroupController(store store.Storer, cloud aws.CloudAPI, authModule auth.Module, tlsModule tls.Module,
	certGroupController cert.GroupController) GroupController {
	lsController := NewController(cloud, authModule)
	return &defaultGroupController{
		cloud: cloud,
		store: store,

		tlsModule:           tlsModule,
		certGroupController: certGroupController,
		lsController:        lsController,
	}
}

type defaultGroupController struct {
	cloud aws.CloudAPI
	store store.Storer

	tlsModule           tls.Module
	certGroupController cert.GroupController
	lsController        Controller
}

func (c *defaultGroupController) Reconcile(ctx context.Context, lbArn string, ingress *extensions.Ingress, tgGroup tg.TargetGroupGroup) error {
	tlsCfg, err := c.tlsModule.NewConfig(ctx, ingress)
	if err != nil {
		return err
	}
	certGroup, err := c.certGroupController.Reconcile(ctx, types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}, tlsCfg.RawCertificates)
	if err != nil {
		return err
	}

	ingressAnnos, err := c.store.GetIngressAnnotations(k8s.MetaNamespaceKey(ingress))
	if err != nil {
		return err
	}
	instancesByPort, err := c.loadListenerInstances(ctx, lbArn)
	if err != nil {
		return err
	}
	portsInUse := sets.NewInt64()
	for _, port := range ingressAnnos.LoadBalancer.Ports {
		portsInUse.Insert(port.Port)
		instance := instancesByPort[port.Port]
		if err := c.lsController.Reconcile(ctx, ReconcileOptions{
			LBArn:        lbArn,
			Ingress:      ingress,
			IngressAnnos: ingressAnnos,
			Port:         port,
			TGGroup:      tgGroup,

			TLSConfig: tlsCfg,
			CertGroup: certGroup,

			Instance: instance,
		}); err != nil {
			return err
		}
	}
	portsUnsed := sets.Int64KeySet(instancesByPort).Difference(portsInUse)
	for port := range portsUnsed {
		instance := instancesByPort[port]
		albctx.GetLogger(ctx).Infof("deleting listener %v, arn: %v", aws.Int64Value(instance.Port), aws.StringValue(instance.ListenerArn))
		if err := c.cloud.DeleteListenersByArn(ctx, aws.StringValue(instance.ListenerArn)); err != nil {
			return err
		}
	}
	return nil
}

func (c *defaultGroupController) Delete(ctx context.Context, lbArn string) error {
	instancesByPort, err := c.loadListenerInstances(ctx, lbArn)
	if err != nil {
		return err
	}
	for _, instance := range instancesByPort {
		albctx.GetLogger(ctx).Infof("deleting listener %v, arn: %v", aws.Int64Value(instance.Port), aws.StringValue(instance.ListenerArn))
		if err := c.cloud.DeleteListenersByArn(ctx, aws.StringValue(instance.ListenerArn)); err != nil {
			return err
		}
	}
	return nil
}

func (c *defaultGroupController) loadListenerInstances(ctx context.Context, lbArn string) (map[int64]*elbv2.Listener, error) {
	instances, err := c.cloud.ListListenersByLoadBalancer(ctx, lbArn)
	if err != nil {
		return nil, err
	}
	instanceByPort := make(map[int64]*elbv2.Listener)
	for _, instance := range instances {
		instanceByPort[aws.Int64Value(instance.Port)] = instance
	}
	return instanceByPort, nil
}
