package gateway

import (
	"context"
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/gateway/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	networkingpkg "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	gatewayTagPrefix = "gateway.k8s.aws"
	controllerName   = "gateway"

	// the groupVersion of used Gateway & GatewayClass resource.
	gatewayResourcesGroupVersion = "gateway.networking.k8s.io/v1beta1"
	gatewayClassKind             = "GatewayClass"
)

// NewGatewayReconciler constructs new GatewayReconciler
func NewGatewayReconciler(cloud aws.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager, networkingSGManager networkingpkg.SecurityGroupManager,
	networkingSGReconciler networkingpkg.SecurityGroupReconciler, subnetsResolver networkingpkg.SubnetsResolver,
	controllerConfig config.ControllerConfig, backendSGProvider networkingpkg.BackendSGProvider, logger logr.Logger) *groupReconciler {

	annotationParser := annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixGateway)
	authConfigBuilder := gateway.NewDefaultAuthConfigBuilder(annotationParser)
	enhancedBackendBuilder := gateway.NewDefaultEnhancedBackendBuilder(k8sClient, annotationParser, authConfigBuilder)
	referenceIndexer := gateway.NewDefaultReferenceIndexer(enhancedBackendBuilder, authConfigBuilder, logger)
	trackingProvider := tracking.NewDefaultProvider(gatewayTagPrefix, controllerConfig.ClusterName)
	elbv2TaggingManager := elbv2deploy.NewDefaultTaggingManager(cloud.ELBV2(), cloud.VpcID(), controllerConfig.FeatureGates, logger)
	modelBuilder := gateway.NewDefaultModelBuilder(k8sClient, eventRecorder,
		cloud.EC2(), cloud.ACM(),
		annotationParser, subnetsResolver,
		authConfigBuilder, enhancedBackendBuilder, trackingProvider, elbv2TaggingManager, controllerConfig.FeatureGates,
		cloud.VpcID(), controllerConfig.ClusterName, controllerConfig.DefaultTags, controllerConfig.ExternalManagedTags,
		controllerConfig.DefaultSSLPolicy, controllerConfig.DefaultTargetType, backendSGProvider,
		controllerConfig.EnableBackendSecurityGroup, controllerConfig.DisableRestrictedSGRules, controllerConfig.FeatureGates.Enabled(config.EnableIPTargetType), logger)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler,
		controllerConfig, gatewayTagPrefix, logger)
	classLoader := gateway.NewDefaultClassLoader(k8sClient)
	classAnnotationMatcher := gateway.NewDefaultClassAnnotationMatcher(controllerConfig.GatewayConfig.GatewayClass)
	manageGatewayesWithoutGatewayClass := controllerConfig.GatewayConfig.GatewayClass == ""
	groupLoader := gateway.NewDefaultGroupLoader(k8sClient, eventRecorder, annotationParser, classLoader, classAnnotationMatcher, manageGatewayesWithoutGatewayClass)
	groupFinalizerManager := gateway.NewDefaultFinalizerManager(finalizerManager)

	return &gatewayReconciler{
		k8sClient:         k8sClient,
		eventRecorder:     eventRecorder,
		referenceIndexer:  referenceIndexer,
		modelBuilder:      modelBuilder,
		stackMarshaller:   stackMarshaller,
		stackDeployer:     stackDeployer,
		backendSGProvider: backendSGProvider,

		groupLoader:           groupLoader,
		groupFinalizerManager: groupFinalizerManager,
		logger:                logger,

		maxConcurrentReconciles: controllerConfig.GatewayConfig.MaxConcurrentReconciles,
	}
}

// gatewayReconciler reconciles a Gateway
type gatewayReconciler struct {
	k8sClient         client.Client
	eventRecorder     record.EventRecorder
	referenceIndexer  gateway.ReferenceIndexer
	modelBuilder      gateway.ModelBuilder
	stackMarshaller   deploy.StackMarshaller
	stackDeployer     deploy.StackDeployer
	backendSGProvider networkingpkg.BackendSGProvider
	secretsManager    k8s.SecretsManager

	groupLoader           gateway.GroupLoader
	groupFinalizerManager gateway.FinalizerManager
	logger                logr.Logger

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=elbv2.k8s.aws,resources=gatewayclassparams,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=gateways,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=gateways/status,verbs=update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=extensions,resources=gateways,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=extensions,resources=gateways/status,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *gatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(ctx, req), r.logger)
}

func (r *gatewayReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	ingGroupID := gateway.DecodeGroupIDFromReconcileRequest(req)
	ingGroup, err := r.groupLoader.Load(ctx, ingGroupID)
	if err != nil {
		return err
	}

	if err := r.groupFinalizerManager.AddGroupFinalizer(ctx, ingGroupID, ingGroup.Members); err != nil {
		r.recordGatewayGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return err
	}
	_, lb, err := r.buildAndDeployModel(ctx, ingGroup)
	if err != nil {
		return err
	}

	if len(ingGroup.Members) > 0 && lb != nil {
		lbDNS, err := lb.DNSName().Resolve(ctx)
		if err != nil {
			return err
		}
		if err := r.updateGatewayGroupStatus(ctx, ingGroup, lbDNS); err != nil {
			r.recordGatewayGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedUpdateStatus, fmt.Sprintf("Failed update status due to %v", err))
			return err
		}
	}

	if len(ingGroup.Members) == 0 {
		if err := r.backendSGProvider.Release(ctx); err != nil {
			return err
		}
	}

	if len(ingGroup.InactiveMembers) > 0 {
		if err := r.groupFinalizerManager.RemoveGroupFinalizer(ctx, ingGroupID, ingGroup.InactiveMembers); err != nil {
			r.recordGatewayGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed remove finalizer due to %v", err))
			return err
		}
	}

	r.recordGatewayGroupEvent(ctx, ingGroup, corev1.EventTypeNormal, k8s.GatewayEventReasonSuccessfullyReconciled, "Successfully reconciled")
	return nil
}

func (r *gatewayReconciler) buildAndDeployModel(ctx context.Context, ingGroup gateway.Group) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack, lb, secrets, err := r.modelBuilder.Build(ctx, ingGroup)
	if err != nil {
		r.recordGatewayGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.recordGatewayGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}
	r.logger.Info("successfully built model", "model", stackJSON)

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		r.recordGatewayGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		return nil, nil, err
	}
	r.logger.Info("successfully deployed model", "ingressGroup", ingGroup.ID)
	r.secretsManager.MonitorSecrets(ingGroup.ID.String(), secrets)
	return stack, lb, err
}

func (r *gatewayReconciler) recordGatewayGroupEvent(_ context.Context, ingGroup ingress.Group, eventType string, reason string, message string) {
	for _, member := range ingGroup.Members {
		r.eventRecorder.Event(member.Ing, eventType, reason, message)
	}
}

func (r *gatewayReconciler) updateGatewayGroupStatus(ctx context.Context, ingGroup ingress.Group, lbDNS string) error {
	for _, member := range ingGroup.Members {
		if err := r.updateGatewayStatus(ctx, lbDNS, member.Ing); err != nil {
			return err
		}
	}
	return nil
}

func (r *gatewayReconciler) updateGatewayStatus(ctx context.Context, lbDNS string, gateway *v1beta1.Gateway) error {
	if len(gateway.Status.Addresses) != 1 ||
		gateway.Status.Addresses[0].Value != lbDNS {
		gatewayOld := gateway.DeepCopy()
		addressType := v1beta1.HostnameAddressType
		gateway.Status.Addresses = []v1beta1.GatewayAddress{{
			Type:  &addressType,
			Value: lbDNS,
		},
		}
		if err := r.k8sClient.Status().Patch(ctx, gateway, client.MergeFrom(gatewayOld)); err != nil {
			return errors.Wrapf(err, "failed to update gateway status: %v", k8s.NamespacedName(gateway))
		}
	}
	return nil
}

func (r *gatewayReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, clientSet *kubernetes.Clientset) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		MaxConcurrentReconciles: r.maxConcurrentReconciles,
		Reconciler:              r,
	})
	if err != nil {
		return err
	}

	resList, err := clientSet.ServerResourcesForGroupVersion(gatewayResourcesGroupVersion)
	if err != nil {
		return err
	}
	gatewayClassResourceAvailable := isResourceKindAvailable(resList, gatewayClassKind)
	if err := r.setupIndexes(ctx, mgr.GetFieldIndexer(), gatewayClassResourceAvailable); err != nil {
		return err
	}
	if err := r.setupWatches(ctx, c, gatewayClassResourceAvailable, clientSet); err != nil {
		return err
	}
	return nil
}

func (r *gatewayReconciler) setupIndexes(ctx context.Context, fieldIndexer client.FieldIndexer, gatewayClassResourceAvailable bool) error {
	if err := fieldIndexer.IndexField(ctx, &v1beta1.Gateway{}, gateway.IndexKeyServiceRefName,
		func(obj client.Object) []string {
			return r.referenceIndexer.BuildServiceRefIndexes(context.Background(), obj.(*v1beta1.Gateway))
		},
	); err != nil {
		return err
	}
	if err := fieldIndexer.IndexField(ctx, &v1beta1.Gateway{}, gateway.IndexKeySecretRefName,
		func(obj client.Object) []string {
			return r.referenceIndexer.BuildSecretRefIndexes(context.Background(), obj.(*v1beta1.Gateway))
		},
	); err != nil {
		return err
	}
	if err := fieldIndexer.IndexField(ctx, &corev1.Service{}, gateway.IndexKeySecretRefName,
		func(obj client.Object) []string {
			return r.referenceIndexer.BuildSecretRefIndexes(context.Background(), obj.(*corev1.Service))
		},
	); err != nil {
		return err
	}
	if gatewayClassResourceAvailable {
		if err := fieldIndexer.IndexField(ctx, &v1beta1.GatewayClass{}, gateway.IndexKeyGatewayClassParamsRefName,
			func(obj client.Object) []string {
				return r.referenceIndexer.BuildGatewayClassParamsRefIndexes(ctx, obj.(*v1beta1.GatewayClass))
			},
		); err != nil {
			return err
		}
		if err := fieldIndexer.IndexField(ctx, &v1beta1.Gateway{}, gateway.IndexKeyGatewayClassRefName,
			func(obj client.Object) []string {
				return r.referenceIndexer.BuildGatewayClassRefIndexes(ctx, obj.(*v1beta1.Gateway))
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *gatewayReconciler) setupWatches(_ context.Context, c controller.Controller, gatewayClassResourceAvailable bool, clientSet *kubernetes.Clientset) error {
	gatewayEventChan := make(chan event.GenericEvent)
	svcEventChan := make(chan event.GenericEvent)
	secretEventChan := make(chan event.GenericEvent)
	tcpRouteEventChan := make(chan event.GenericEvent)
	gatewayEventHandler := eventhandlers.NewEnqueueRequestsForGatewayEvent(r.groupLoader, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("gateway"))
	tcpRouteEventHandler := eventhandlers.NewEnqueueRequestsForTCPRouteEvent(r.groupLoader, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("tcpRoute"))
	svcEventHandler := eventhandlers.NewEnqueueRequestsForServiceEvent(gatewayEventChan, r.k8sClient, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("service"))
	secretEventHandler := eventhandlers.NewEnqueueRequestsForSecretEvent(gatewayEventChan, svcEventChan, r.k8sClient, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("secret"))
	if err := c.Watch(&source.Channel{Source: gatewayEventChan}, gatewayEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Channel{Source: tcpRouteEventChan}, tcpRouteEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Channel{Source: svcEventChan}, svcEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &v1beta1.Gateway{}}, gatewayEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &v1alpha2.TCPRoute{}}, tcpRouteEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, svcEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Channel{Source: secretEventChan}, secretEventHandler); err != nil {
		return err
	}
	if gatewayClassResourceAvailable {
		gatewayClassEventChan := make(chan event.GenericEvent)
		gatewayClassParamsEventHandler := eventhandlers.NewEnqueueRequestsForGatewayClassParamsEvent(gatewayClassEventChan, r.k8sClient, r.eventRecorder,
			r.logger.WithName("eventHandlers").WithName("gatewayClassParams"))
		gatewayClassEventHandler := eventhandlers.NewEnqueueRequestsForGatewayClassEvent(gatewayEventChan, r.k8sClient, r.eventRecorder,
			r.logger.WithName("eventHandlers").WithName("gatewayClass"))
		if err := c.Watch(&source.Channel{Source: gatewayClassEventChan}, gatewayClassEventHandler); err != nil {
			return err
		}
		if err := c.Watch(&source.Kind{Type: &elbv2api.GatewayClassParams{}}, gatewayClassParamsEventHandler); err != nil {
			return err
		}
		if err := c.Watch(&source.Kind{Type: &v1beta1.GatewayClass{}}, gatewayClassEventHandler); err != nil {
			return err
		}
	}
	r.secretsManager = k8s.NewSecretsManager(clientSet, secretEventChan, ctrl.Log.WithName("secrets-manager"))
	return nil
}

// isResourceKindAvailable checks whether specific kind is available.
func isResourceKindAvailable(resList *metav1.APIResourceList, kind string) bool {
	for _, res := range resList.APIResources {
		if res.Kind == kind {
			return true
		}
	}
	return false
}
