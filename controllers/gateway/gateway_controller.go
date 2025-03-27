package gateway

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	gatewaymodel "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/model"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/util"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Reconciler interface {
	Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error)
	SetupWithManager(mgr ctrl.Manager) error
}

var _ Reconciler = &gatewayReconciler{}

// NewNLBGatewayReconciler constructs a gateway reconciler to handle specifically for NLB gateways
func NewNLBGatewayReconciler(routeLoader routeutils.Loader, cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, finalizerManager k8s.FinalizerManager, networkingSGReconciler networking.SecurityGroupReconciler, networkingSGManager networking.SecurityGroupManager, elbv2TaggingManager elbv2deploy.TaggingManager, subnetResolver networking.SubnetsResolver, vpcInfoProvider networking.VPCInfoProvider, backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters) Reconciler {
	return newGatewayReconciler(nlbGatewayController, controllerConfig.NLBGatewayMaxConcurrentReconciles, nlbGatewayTagPrefix, nlbGatewayFinalizer, routeLoader, routeutils.L4RouteFilter, cloud, k8sClient, eventRecorder, controllerConfig, finalizerManager, networkingSGReconciler, networkingSGManager, elbv2TaggingManager, subnetResolver, vpcInfoProvider, backendSGProvider, sgResolver, logger, metricsCollector, reconcileCounters)
}

// NewALBGatewayReconciler constructs a gateway reconciler to handle specifically for ALB gateways
func NewALBGatewayReconciler(routeLoader routeutils.Loader, cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, finalizerManager k8s.FinalizerManager, networkingSGReconciler networking.SecurityGroupReconciler, networkingSGManager networking.SecurityGroupManager, elbv2TaggingManager elbv2deploy.TaggingManager, subnetResolver networking.SubnetsResolver, vpcInfoProvider networking.VPCInfoProvider, backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters) Reconciler {
	return newGatewayReconciler(albGatewayController, controllerConfig.ALBGatewayMaxConcurrentReconciles, albGatewayTagPrefix, albGatewayFinalizer, routeLoader, routeutils.L7RouteFilter, cloud, k8sClient, eventRecorder, controllerConfig, finalizerManager, networkingSGReconciler, networkingSGManager, elbv2TaggingManager, subnetResolver, vpcInfoProvider, backendSGProvider, sgResolver, logger, metricsCollector, reconcileCounters)
}

// newGatewayReconciler constructs a reconciler that responds to gateway object changes
func newGatewayReconciler(controllerName string, maxConcurrentReconciles int, gatewayTagPrefix string, finalizer string, routeLoader routeutils.Loader, routeFilter routeutils.LoadRouteFilter, cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, finalizerManager k8s.FinalizerManager, networkingSGReconciler networking.SecurityGroupReconciler, networkingSGManager networking.SecurityGroupManager, elbv2TaggingManager elbv2deploy.TaggingManager, subnetResolver networking.SubnetsResolver, vpcInfoProvider networking.VPCInfoProvider, backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters) Reconciler {

	trackingProvider := tracking.NewDefaultProvider(gatewayTagPrefix, controllerConfig.ClusterName)
	modelBuilder := gatewaymodel.NewDefaultModelBuilder(subnetResolver, vpcInfoProvider, cloud.VpcID(), trackingProvider, elbv2TaggingManager, cloud.EC2(), controllerConfig.FeatureGates, controllerConfig.ClusterName, controllerConfig.DefaultTags, sets.New(controllerConfig.ExternalManagedTags...), controllerConfig.DefaultSSLPolicy, controllerConfig.DefaultTargetType, controllerConfig.DefaultLoadBalancerScheme, backendSGProvider, sgResolver, controllerConfig.EnableBackendSecurityGroup, controllerConfig.DisableRestrictedSGRules, logger)

	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler, elbv2TaggingManager, controllerConfig, gatewayTagPrefix, logger, metricsCollector, controllerName)

	return &gatewayReconciler{
		controllerName:          controllerName,
		maxConcurrentReconciles: maxConcurrentReconciles,
		finalizer:               finalizer,
		gatewayLoader:           routeLoader,
		routeFilter:             routeFilter,
		k8sClient:               k8sClient,
		modelBuilder:            modelBuilder,
		backendSGProvider:       backendSGProvider,
		stackMarshaller:         stackMarshaller,
		stackDeployer:           stackDeployer,
		finalizerManager:        finalizerManager,
		eventRecorder:           eventRecorder,
		logger:                  logger,
		metricsCollector:        metricsCollector,
		reconcileCounters:       reconcileCounters,
	}
}

// gatewayReconciler reconciles a Gateway.
type gatewayReconciler struct {
	controllerName          string
	finalizer               string
	maxConcurrentReconciles int
	gatewayLoader           routeutils.Loader
	routeFilter             routeutils.LoadRouteFilter
	k8sClient               client.Client
	modelBuilder            gatewaymodel.Builder
	backendSGProvider       networking.BackendSGProvider
	stackMarshaller         deploy.StackMarshaller
	stackDeployer           deploy.StackDeployer
	finalizerManager        k8s.FinalizerManager
	eventRecorder           record.EventRecorder
	logger                  logr.Logger

	metricsCollector  lbcmetrics.MetricCollector
	reconcileCounters *metricsutil.ReconcileCounters
}

// TODO - Add Gateway and TG configuration permissions

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes/finalizers,verbs=update

func (r *gatewayReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	err := r.reconcileHelper(ctx, req)
	if err != nil {
		r.logger.Error(err, "Got this error!")
	}
	return runtime.HandleReconcileError(err, r.logger)
}

func (r *gatewayReconciler) reconcileHelper(ctx context.Context, req reconcile.Request) error {

	gw := &gwv1.Gateway{}
	if err := r.k8sClient.Get(ctx, req.NamespacedName, gw); err != nil {
		return client.IgnoreNotFound(err)
	}

	r.logger.Info("Got request for reconcile", "gw", *gw)

	gwClass := &gwv1.GatewayClass{}

	// Gateway Class is a cluster scoped resource, but the k8s client only accepts namespaced names.
	gwClassNamespacedName := types.NamespacedName{
		Name: string(gw.Spec.GatewayClassName),
	}

	if err := r.k8sClient.Get(ctx, gwClassNamespacedName, gwClass); err != nil {
		r.logger.Info("Failed to get GatewayClass", "error", err, "gw-class", gwClassNamespacedName.Name)
		return client.IgnoreNotFound(err)
	}

	if string(gwClass.Spec.ControllerName) != r.controllerName {
		// ignore this gateway event as the gateway belongs to a different controller.
		return nil
	}

	allRoutes, err := r.gatewayLoader.LoadRoutesForGateway(ctx, *gw, r.routeFilter)

	if err != nil {
		return err
	}

	stack, lb, backendSGRequired, err := r.buildModel(ctx, gw, gwClass, allRoutes)

	if err != nil {
		return err
	}

	if lb == nil {
		err = r.reconcileDelete(ctx, gw, allRoutes)
		if err != nil {
			r.logger.Error(err, "Failed to process gateway delete")
		}
		return err
	}

	return r.reconcileUpdate(ctx, gw, stack, lb, backendSGRequired)
}

func (r *gatewayReconciler) reconcileDelete(ctx context.Context, gw *gwv1.Gateway, routes map[int][]routeutils.RouteDescriptor) error {
	for _, routeList := range routes {
		if len(routeList) != 0 {
			// TODO - Better error messaging (e.g. tell user the routes that are still attached)
			return errors.New("Gateway still has routes attached")
		}
	}

	return r.finalizerManager.RemoveFinalizers(ctx, gw, r.finalizer)
}

func (r *gatewayReconciler) reconcileUpdate(ctx context.Context, gw *gwv1.Gateway, stack core.Stack,
	lb *elbv2model.LoadBalancer, backendSGRequired bool) error {

	if err := r.finalizerManager.AddFinalizers(ctx, gw, r.finalizer); err != nil {
		r.eventRecorder.Event(gw, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return err
	}
	err := r.deployModel(ctx, gw, stack)
	if err != nil {
		return err
	}
	lbDNS, err := lb.DNSName().Resolve(ctx)
	if err != nil {
		return err
	}

	if !backendSGRequired {
		if err := r.backendSGProvider.Release(ctx, networking.ResourceTypeService, []types.NamespacedName{k8s.NamespacedName(gw)}); err != nil {
			return err
		}
	}

	if err = r.updateGatewayStatus(ctx, lbDNS, gw); err != nil {
		r.eventRecorder.Event(gw, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedUpdateStatus, fmt.Sprintf("Failed update status due to %v", err))
		return err
	}
	r.eventRecorder.Event(gw, corev1.EventTypeNormal, k8s.ServiceEventReasonSuccessfullyReconciled, "Successfully reconciled")
	return nil
}

func (r *gatewayReconciler) deployModel(ctx context.Context, gw *gwv1.Gateway, stack core.Stack) error {
	if err := r.stackDeployer.Deploy(ctx, stack, r.metricsCollector, r.controllerName); err != nil {
		var requeueNeededAfter *runtime.RequeueNeededAfter
		if errors.As(err, &requeueNeededAfter) {
			return err
		}
		r.eventRecorder.Event(gw, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		return err
	}
	r.logger.Info("successfully deployed model", "gateway", k8s.NamespacedName(gw))
	return nil
}

func (r *gatewayReconciler) buildModel(ctx context.Context, gw *gwv1.Gateway, gwClass *gwv1.GatewayClass, listenerToRoute map[int][]routeutils.RouteDescriptor) (core.Stack, *elbv2model.LoadBalancer, bool, error) {
	stack, lb, backendSGRequired, err := r.modelBuilder.Build(ctx, gw, gwClass, listenerToRoute)
	if err != nil {
		r.eventRecorder.Event(gw, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, false, err
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.eventRecorder.Event(gw, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, false, err
	}
	r.logger.Info("successfully built model", "model", stackJSON)
	return stack, lb, backendSGRequired, nil
}

func (r *gatewayReconciler) updateGatewayStatus(ctx context.Context, lbDNS string, gw *gwv1.Gateway) error {
	// TODO Consider LB ARN.

	// Gateway Address Status
	if len(gw.Status.Addresses) != 1 ||
		gw.Status.Addresses[0].Value != "" ||
		gw.Status.Addresses[0].Value != lbDNS {
		gwOld := gw.DeepCopy()
		ipAddressType := gwv1.HostnameAddressType
		gw.Status.Addresses = []gwv1.GatewayStatusAddress{
			{
				Type:  &ipAddressType,
				Value: lbDNS,
			},
		}
		if err := r.k8sClient.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
			return errors.Wrapf(err, "failed to update gw status: %v", k8s.NamespacedName(gw))
		}
	}

	// TODO: Listener status ListenerStatus
	// https://github.com/aws/aws-application-networking-k8s/blob/main/pkg/controllers/gateway_controller.go#L350

	return nil
}

func (r *gatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	/*
		gatewayClassHandler := eventhandlers.NewEnqueueRequestsForGatewayClassEvent(r.logger, r.k8sClient, r.config)
		tcpRouteHandler := eventhandlers.NewEnqueueRequestsForTCPRouteEvent(r.logger, r.k8sClient, r.config)
		udpRouteHandler := eventhandlers.NewEnqueueRequestsForUDPRouteEvent(r.logger, r.k8sClient, r.config)
		return ctrl.NewControllerManagedBy(mgr).
			Named("nlbgateway").
			// Anything that influences a gateway object must be added here.
			For(&gwv1.Gateway{}).
			Watches(&gwv1.GatewayClass{}, gatewayClassHandler).
			Watches(&gwalpha2.TCPRoute{}, tcpRouteHandler).
			Watches(&gwalpha2.UDPRoute{}, udpRouteHandler).
			WithOptions(controller.Options{
				MaxConcurrentReconciles: r.config.MaxConcurrentReconciles,
			}).
			Complete(r)
	*/

	return ctrl.NewControllerManagedBy(mgr).
		Named(r.controllerName).
		// Anything that influences a gateway object must be added here.
		For(&gwv1.Gateway{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}
