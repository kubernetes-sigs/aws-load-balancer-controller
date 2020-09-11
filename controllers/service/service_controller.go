package service

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-alb-ingress-controller/controllers/service/eventhandlers"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/services"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/runtime"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/service/nlb"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	LoadBalancerFinalizer   = "service.k8s.aws/load-balancer-finalizer"
	DefaultTagPrefix        = "service.k8s.aws"
	ServiceAnnotationPrefix = "service.beta.kubernetes.io"
	controllerName          = "service"
)

func NewServiceReconciler(k8sClient client.Client, elbv2Client services.ELBV2, vpcId string, clusterName string, resolver networking.SubnetsResolver, log logr.Logger) *ServiceReconciler {
	return &ServiceReconciler{
		k8sClient:        k8sClient,
		elbv2Client:      elbv2Client,
		vpcID:            vpcId,
		clusterName:      clusterName,
		log:              log,
		annotationParser: annotations.NewSuffixAnnotationParser(ServiceAnnotationPrefix),
		finalizerManager: k8s.NewDefaultFinalizerManager(k8sClient, log),
		subnetsResolver:  resolver,
	}
}

type ServiceReconciler struct {
	k8sClient        client.Client
	elbv2Client      services.ELBV2
	vpcID            string
	clusterName      string
	log              logr.Logger
	annotationParser annotations.Parser
	finalizerManager k8s.FinalizerManager
	subnetsResolver  networking.SubnetsResolver
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
func (r *ServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(req), r.log)
}

func (r *ServiceReconciler) reconcile(req ctrl.Request) error {
	ctx := context.Background()
	svc := &corev1.Service{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, svc); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !svc.DeletionTimestamp.IsZero() {
		return r.cleanupLoadBalancerResources(ctx, svc)
	}
	return r.reconcileLoadBalancerResources(ctx, svc)
}

func (r *ServiceReconciler) buildAndDeployModel(ctx context.Context, svc *corev1.Service) (core.Stack, error) {
	nlbBuilder := nlb.NewServiceBuilder(svc, r.subnetsResolver, k8s.NamespacedName(svc), r.annotationParser)
	stack, err := nlbBuilder.Build(ctx)
	if err != nil {
		return nil, err
	}

	d := deploy.NewDefaultStackMarshaller()
	jsonString, err := d.Marshal(stack)
	r.log.Info("Built service model", "stack", jsonString)

	deployer := deploy.NewDefaultStackDeployer(r.k8sClient, r.elbv2Client, r.vpcID, r.clusterName, DefaultTagPrefix, r.log)
	err = deployer.Deploy(ctx, stack)
	if err != nil {
		return nil, err
	}
	r.log.Info("Successfully deployed service resources")
	return stack, nil
}

func (r *ServiceReconciler) reconcileLoadBalancerResources(ctx context.Context, svc *corev1.Service) error {
	if err := r.finalizerManager.AddFinalizers(ctx, svc, LoadBalancerFinalizer); err != nil {
		return err
	}
	stack, err := r.buildAndDeployModel(ctx, svc)
	if err != nil {
		return err
	}
	var resLBs []*elbv2model.LoadBalancer
	stack.ListResources(&resLBs)
	if len(resLBs) == 0 {
		r.log.Info("Unable to list LoadBalancer resource")
	} else {
		var resTGs []*elbv2model.TargetGroup
		resLB := resLBs[0]
		dnsName, _ := resLB.DNSName().Resolve(ctx)
		stack.ListResources(&resTGs)
		r.log.Info("Deployed LoadBalancer", "dnsname", dnsName, "#target groups", len(resTGs))
		err = r.updateServiceStatus(ctx, svc, dnsName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ServiceReconciler) cleanupLoadBalancerResources(ctx context.Context, svc *corev1.Service) error {
	if k8s.HasFinalizer(svc, LoadBalancerFinalizer) {
		_, err := r.buildAndDeployModel(ctx, svc)
		if err != nil {
			return err
		}
		if err := r.finalizerManager.RemoveFinalizers(ctx, svc, LoadBalancerFinalizer); err != nil {
			return err
		}
	}
	return nil
}

func (r *ServiceReconciler) updateServiceStatus(ctx context.Context, svc *corev1.Service, lbDNS string) error {
	if len(svc.Status.LoadBalancer.Ingress) != 1 || svc.Status.LoadBalancer.Ingress[0].IP != "" || svc.Status.LoadBalancer.Ingress[0].Hostname != lbDNS {
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				Hostname: lbDNS,
			},
		}
		if err := r.k8sClient.Status().Update(ctx, svc); err != nil {
			return errors.Wrapf(err, "failed to update service:%v", svc)
		}
		return r.k8sClient.Status().Update(ctx, svc)
	}
	return nil
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		MaxConcurrentReconciles: 1,
		Reconciler:              r,
	})
	if err != nil {
		return err
	}
	return r.setupWatches(mgr, c)
}

func (r *ServiceReconciler) setupWatches(mgr ctrl.Manager, c controller.Controller) error {
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(mgr.GetEventRecorderFor(controllerName), r.annotationParser)
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, svcEventHandler); err != nil {
		return err
	}
	return nil
}
