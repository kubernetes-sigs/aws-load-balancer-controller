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
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/gateway/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	gatewaymodel "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/model"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/util"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Reconciler interface {
	Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error)
	SetupWithManager(ctx context.Context, mgr ctrl.Manager) (controller.Controller, error)
	SetupWatches(ctx context.Context, controller controller.Controller, mgr ctrl.Manager) error
}

var _ Reconciler = &gatewayReconciler{}

// NewNLBGatewayReconciler constructs a gateway reconciler to handle specifically for NLB gateways
func NewNLBGatewayReconciler(routeLoader routeutils.Loader, cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, finalizerManager k8s.FinalizerManager, networkingSGReconciler networking.SecurityGroupReconciler, networkingSGManager networking.SecurityGroupManager, elbv2TaggingManager elbv2deploy.TaggingManager, subnetResolver networking.SubnetsResolver, vpcInfoProvider networking.VPCInfoProvider, backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters) Reconciler {
	return newGatewayReconciler(constants.NLBGatewayController, elbv2model.LoadBalancerTypeNetwork, controllerConfig.NLBGatewayMaxConcurrentReconciles, constants.NLBGatewayTagPrefix, shared_constants.NLBGatewayFinalizer, routeLoader, routeutils.L4RouteFilter, cloud, k8sClient, eventRecorder, controllerConfig, finalizerManager, networkingSGReconciler, networkingSGManager, elbv2TaggingManager, subnetResolver, vpcInfoProvider, backendSGProvider, sgResolver, logger, metricsCollector, reconcileCounters.IncrementNLBGateway)
}

// NewALBGatewayReconciler constructs a gateway reconciler to handle specifically for ALB gateways
func NewALBGatewayReconciler(routeLoader routeutils.Loader, cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, finalizerManager k8s.FinalizerManager, networkingSGReconciler networking.SecurityGroupReconciler, networkingSGManager networking.SecurityGroupManager, elbv2TaggingManager elbv2deploy.TaggingManager, subnetResolver networking.SubnetsResolver, vpcInfoProvider networking.VPCInfoProvider, backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters) Reconciler {
	return newGatewayReconciler(constants.ALBGatewayController, elbv2model.LoadBalancerTypeApplication, controllerConfig.ALBGatewayMaxConcurrentReconciles, constants.ALBGatewayTagPrefix, shared_constants.ALBGatewayFinalizer, routeLoader, routeutils.L7RouteFilter, cloud, k8sClient, eventRecorder, controllerConfig, finalizerManager, networkingSGReconciler, networkingSGManager, elbv2TaggingManager, subnetResolver, vpcInfoProvider, backendSGProvider, sgResolver, logger, metricsCollector, reconcileCounters.IncrementALBGateway)
}

// newGatewayReconciler constructs a reconciler that responds to gateway object changes
func newGatewayReconciler(controllerName string, lbType elbv2model.LoadBalancerType, maxConcurrentReconciles int, gatewayTagPrefix string, finalizer string, routeLoader routeutils.Loader, routeFilter routeutils.LoadRouteFilter, cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder, controllerConfig config.ControllerConfig, finalizerManager k8s.FinalizerManager, networkingSGReconciler networking.SecurityGroupReconciler, networkingSGManager networking.SecurityGroupManager, elbv2TaggingManager elbv2deploy.TaggingManager, subnetResolver networking.SubnetsResolver, vpcInfoProvider networking.VPCInfoProvider, backendSGProvider networking.BackendSGProvider, sgResolver networking.SecurityGroupResolver, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileTracker func(namespaceName types.NamespacedName)) Reconciler {

	trackingProvider := tracking.NewDefaultProvider(gatewayTagPrefix, controllerConfig.ClusterName)
	modelBuilder := gatewaymodel.NewModelBuilder(subnetResolver, vpcInfoProvider, cloud.VpcID(), lbType, trackingProvider, elbv2TaggingManager, controllerConfig, cloud.EC2(), controllerConfig.FeatureGates, controllerConfig.ClusterName, controllerConfig.DefaultTags, sets.New(controllerConfig.ExternalManagedTags...), controllerConfig.DefaultSSLPolicy, controllerConfig.DefaultTargetType, controllerConfig.DefaultLoadBalancerScheme, backendSGProvider, sgResolver, controllerConfig.EnableBackendSecurityGroup, controllerConfig.DisableRestrictedSGRules, logger)

	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler, elbv2TaggingManager, controllerConfig, gatewayTagPrefix, logger, metricsCollector, controllerName)

	return &gatewayReconciler{
		controllerName:          controllerName,
		lbType:                  lbType,
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
		reconcileTracker:        reconcileTracker,
	}
}

// gatewayReconciler reconciles a Gateway.
type gatewayReconciler struct {
	controllerName          string
	lbType                  elbv2model.LoadBalancerType
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

	metricsCollector lbcmetrics.MetricCollector
	reconcileTracker func(namespaceName types.NamespacedName)
}

// TODO - Add Gateway and TG configuration permissions

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.k8s.aws,resources=loadbalancerconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.k8s.aws,resources=loadbalancerconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.k8s.aws,resources=loadbalancerconfigurations/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.k8s.aws,resources=targetgroupconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.k8s.aws,resources=targetgroupconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.k8s.aws,resources=targetgroupconfigurations/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/finalizers,verbs=update

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.k8s.aws,resources=loadbalancerconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.k8s.aws,resources=targetgroupconfigurations,verbs=get;list;watch

func (r *gatewayReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	r.reconcileTracker(req.NamespacedName)
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

	r.logger.Info("In Gateway Controller - Got these routes", "routes", allRoutes)

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
	if err := r.stackDeployer.Deploy(ctx, stack, r.metricsCollector, r.controllerName, nil); err != nil {
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
	stack, lb, backendSGRequired, err := r.modelBuilder.Build(ctx, gw, &elbv2gw.LoadBalancerConfiguration{}, listenerToRoute)
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

func (r *gatewayReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) (controller.Controller, error) {
	c, err := controller.New(r.controllerName, mgr, controller.Options{
		MaxConcurrentReconciles: r.maxConcurrentReconciles,
		Reconciler:              r,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *gatewayReconciler) SetupWatches(ctx context.Context, c controller.Controller, mgr ctrl.Manager) error {
	if err := r.setupCommonGatewayControllerWatches(c, mgr); err != nil {
		return err
	}
	switch r.controllerName {
	case constants.ALBGatewayController:
		if err := r.setupALBGatewayControllerWatches(c, mgr); err != nil {
			return err
		}
		break
	case constants.NLBGatewayController:
		if err := r.setupNLBGatewayControllerWatches(c, mgr); err != nil {
			return err
		}
		break
	default:
		return fmt.Errorf("unknown controller %v", r.controllerName)
	}
	return nil
}

func (r *gatewayReconciler) setupCommonGatewayControllerWatches(ctrl controller.Controller, mgr ctrl.Manager) error {
	loggerPrefix := r.logger.WithName("eventHandlers")

	gwEventHandler := eventhandlers.NewEnqueueRequestsForGatewayEventHandler(r.k8sClient, r.eventRecorder, r.controllerName,
		loggerPrefix.WithName("Gateway"))
	ctrl.Watch(source.Kind(mgr.GetCache(), &gwv1.Gateway{}, gwEventHandler))

	gwClassEventChan := make(chan event.TypedGenericEvent[*gwv1.GatewayClass])
	lbConfigEventChan := make(chan event.TypedGenericEvent[*elbv2gw.LoadBalancerConfiguration])

	gwClassEventHandler := eventhandlers.NewEnqueueRequestsForGatewayClassEvent(r.k8sClient, r.eventRecorder, r.controllerName,
		loggerPrefix.WithName("GatewayClass"))
	lbConfigEventHandler := eventhandlers.NewEnqueueRequestsForLoadBalancerConfigurationEvent(gwClassEventChan, r.k8sClient, r.eventRecorder, r.controllerName,
		loggerPrefix.WithName("LoadBalancerConfiguration"))

	if err := ctrl.Watch(source.Channel(gwClassEventChan, gwClassEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Channel(lbConfigEventChan, lbConfigEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &gwv1.GatewayClass{}, gwClassEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &elbv2gw.LoadBalancerConfiguration{}, lbConfigEventHandler)); err != nil {
		return err
	}
	return nil

}

func (r *gatewayReconciler) setupALBGatewayControllerWatches(ctrl controller.Controller, mgr ctrl.Manager) error {
	loggerPrefix := r.logger.WithName("eventHandlers")
	tbConfigEventChan := make(chan event.TypedGenericEvent[*elbv2gw.TargetGroupConfiguration])
	httpRouteEventChan := make(chan event.TypedGenericEvent[*gwv1.HTTPRoute])
	grpcRouteEventChan := make(chan event.TypedGenericEvent[*gwv1.GRPCRoute])
	svcEventChan := make(chan event.TypedGenericEvent[*corev1.Service])
	tgConfigEventHandler := eventhandlers.NewEnqueueRequestsForTargetGroupConfigurationEvent(svcEventChan, r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("TargetGroupConfiguration"))
	grpcRouteEventHandler := eventhandlers.NewEnqueueRequestsForGRPCRouteEvent(r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("GRPCRoute"))
	httpRouteEventHandler := eventhandlers.NewEnqueueRequestsForHTTPRouteEvent(r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("HTTPRoute"))
	svcEventHandler := eventhandlers.NewEnqueueRequestsForServiceEvent(httpRouteEventChan, grpcRouteEventChan, nil, nil, nil, r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("Service"), constants.ALBGatewayController)
	if err := ctrl.Watch(source.Channel(tbConfigEventChan, tgConfigEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Channel(httpRouteEventChan, httpRouteEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Channel(grpcRouteEventChan, grpcRouteEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Channel(svcEventChan, svcEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &elbv2gw.TargetGroupConfiguration{}, tgConfigEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, svcEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &gwv1.HTTPRoute{}, httpRouteEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &gwv1.GRPCRoute{}, grpcRouteEventHandler)); err != nil {
		return err
	}
	return nil
}

func (r *gatewayReconciler) setupNLBGatewayControllerWatches(ctrl controller.Controller, mgr ctrl.Manager) error {
	loggerPrefix := r.logger.WithName("eventHandlers")
	tbConfigEventChan := make(chan event.TypedGenericEvent[*elbv2gw.TargetGroupConfiguration])
	tcpRouteEventChan := make(chan event.TypedGenericEvent[*gwalpha2.TCPRoute])
	udpRouteEventChan := make(chan event.TypedGenericEvent[*gwalpha2.UDPRoute])
	tlsRouteEventChan := make(chan event.TypedGenericEvent[*gwalpha2.TLSRoute])
	svcEventChan := make(chan event.TypedGenericEvent[*corev1.Service])
	tgConfigEventHandler := eventhandlers.NewEnqueueRequestsForTargetGroupConfigurationEvent(svcEventChan, r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("TargetGroupConfiguration"))
	tcpRouteEventHandler := eventhandlers.NewEnqueueRequestsForTCPRouteEvent(r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("TCPRoute"))
	udpRouteEventHandler := eventhandlers.NewEnqueueRequestsForUDPRouteEvent(r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("UDPRoute"))
	tlsRouteEventHandler := eventhandlers.NewEnqueueRequestsForTLSRouteEvent(r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("TLSRoute"))
	svcEventHandler := eventhandlers.NewEnqueueRequestsForServiceEvent(nil, nil, tcpRouteEventChan, udpRouteEventChan, tlsRouteEventChan, r.k8sClient, r.eventRecorder,
		loggerPrefix.WithName("Service"), constants.NLBGatewayController)
	if err := ctrl.Watch(source.Channel(tbConfigEventChan, tgConfigEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Channel(tcpRouteEventChan, tcpRouteEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Channel(udpRouteEventChan, udpRouteEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Channel(tlsRouteEventChan, tlsRouteEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Channel(svcEventChan, svcEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &elbv2gw.TargetGroupConfiguration{}, tgConfigEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, svcEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &gwalpha2.TCPRoute{}, tcpRouteEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &gwalpha2.UDPRoute{}, udpRouteEventHandler)); err != nil {
		return err
	}
	if err := ctrl.Watch(source.Kind(mgr.GetCache(), &gwalpha2.TLSRoute{}, tlsRouteEventHandler)); err != nil {
		return err
	}
	return nil

}
