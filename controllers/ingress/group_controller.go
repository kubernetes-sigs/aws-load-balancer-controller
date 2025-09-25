package ingress

import (
	"context"
	"fmt"
	awsmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/aws"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/ingress/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/util"
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
	ingressTagPrefix = "ingress.k8s.aws"
	controllerName   = "ingress"

	// the groupVersion of used Ingress & IngressClass resource.
	ingressResourcesGroupVersion = "networking.k8s.io/v1"
	ingressClassKind             = "IngressClass"
)

// NewGroupReconciler constructs new GroupReconciler
func NewGroupReconciler(cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager, networkingSGManager networkingpkg.SecurityGroupManager,
	networkingManager networkingpkg.NetworkingManager, networkingSGReconciler networkingpkg.SecurityGroupReconciler, subnetsResolver networkingpkg.SubnetsResolver,
	elbv2TaggingManager elbv2deploy.TaggingManager, controllerConfig config.ControllerConfig, backendSGProvider networkingpkg.BackendSGProvider,
	sgResolver networkingpkg.SecurityGroupResolver, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector, reconcileCounters *metricsutil.ReconcileCounters,
	targetGroupCollector awsmetrics.TargetGroupCollector) *groupReconciler {

	annotationParser := annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress)
	authConfigBuilder := ingress.NewDefaultAuthConfigBuilder(annotationParser)
	enhancedBackendBuilder := ingress.NewDefaultEnhancedBackendBuilder(k8sClient, annotationParser, authConfigBuilder, controllerConfig.IngressConfig.TolerateNonExistentBackendService, controllerConfig.IngressConfig.TolerateNonExistentBackendAction)
	referenceIndexer := ingress.NewDefaultReferenceIndexer(enhancedBackendBuilder, authConfigBuilder, logger)
	trackingProvider := tracking.NewDefaultProvider(ingressTagPrefix, controllerConfig.ClusterName)
	modelBuilder := ingress.NewDefaultModelBuilder(k8sClient, eventRecorder,
		cloud.EC2(), cloud.ELBV2(), cloud.WAFv2(), cloud.ACM(),
		annotationParser, subnetsResolver,
		authConfigBuilder, enhancedBackendBuilder, trackingProvider, elbv2TaggingManager, controllerConfig.FeatureGates,
		cloud.VpcID(), controllerConfig.ClusterName, controllerConfig.DefaultTags, controllerConfig.ExternalManagedTags,
		controllerConfig.DefaultSSLPolicy, controllerConfig.DefaultTargetType, controllerConfig.DefaultLoadBalancerScheme, backendSGProvider, sgResolver,
		controllerConfig.EnableBackendSecurityGroup, controllerConfig.EnableManageBackendSecurityGroupRules, controllerConfig.DisableRestrictedSGRules, controllerConfig.IngressConfig.AllowedCertificateAuthorityARNs, controllerConfig.FeatureGates.Enabled(config.EnableIPTargetType), logger, metricsCollector)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingManager, networkingSGManager, networkingSGReconciler, elbv2TaggingManager,
		controllerConfig, ingressTagPrefix, logger, metricsCollector, controllerName, targetGroupCollector)
	classLoader := ingress.NewDefaultClassLoader(k8sClient, true)
	classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher(controllerConfig.IngressConfig.IngressClass)
	manageIngressesWithoutIngressClass := controllerConfig.IngressConfig.IngressClass == ""
	groupLoader := ingress.NewDefaultGroupLoader(k8sClient, eventRecorder, annotationParser, classLoader, classAnnotationMatcher, manageIngressesWithoutIngressClass)
	groupFinalizerManager := ingress.NewDefaultFinalizerManager(finalizerManager)

	return &groupReconciler{
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
		metricsCollector:      metricsCollector,
		controllerName:        controllerName,
		reconcileCounters:     reconcileCounters,

		maxConcurrentReconciles: controllerConfig.IngressConfig.MaxConcurrentReconciles,
	}
}

// GroupReconciler reconciles a IngressGroup
type groupReconciler struct {
	k8sClient         client.Client
	eventRecorder     record.EventRecorder
	referenceIndexer  ingress.ReferenceIndexer
	modelBuilder      ingress.ModelBuilder
	stackMarshaller   deploy.StackMarshaller
	stackDeployer     deploy.StackDeployer
	backendSGProvider networkingpkg.BackendSGProvider
	secretsManager    k8s.SecretsManager

	groupLoader           ingress.GroupLoader
	groupFinalizerManager ingress.FinalizerManager
	logger                logr.Logger
	metricsCollector      lbcmetrics.MetricCollector
	controllerName        string
	reconcileCounters     *metricsutil.ReconcileCounters

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=elbv2.k8s.aws,resources=ingressclassparams,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=extensions,resources=ingresses/status,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *groupReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	r.reconcileCounters.IncrementIngress(req.NamespacedName)
	return runtime.HandleReconcileError(r.reconcile(ctx, req), r.logger)
}

func (r *groupReconciler) reconcile(ctx context.Context, req reconcile.Request) error {
	ingGroupID := ingress.DecodeGroupIDFromReconcileRequest(req)
	var err error
	var ingGroup ingress.Group
	loadIngressFn := func() {
		ingGroup, err = r.groupLoader.Load(ctx, ingGroupID)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "fetch_ingress", loadIngressFn)
	if err != nil {
		return ctrlerrors.NewErrorWithMetrics(controllerName, "fetch_ingress_error", err, r.metricsCollector)
	}

	addFinalizerFn := func() {
		err = r.groupFinalizerManager.AddGroupFinalizer(ctx, ingGroupID, ingGroup.Members)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "add_group_finalizer", addFinalizerFn)
	if err != nil {
		r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return ctrlerrors.NewErrorWithMetrics(controllerName, "add_group_finalizer_error", err, r.metricsCollector)
	}

	_, lb, frontendNlb, err := r.buildAndDeployModel(ctx, ingGroup)
	if err != nil {
		return err
	}

	if len(ingGroup.Members) > 0 && lb != nil {
		var statusErr error
		dnsResolveAndUpdateStatus := func() {
			var lbDNS string
			lbDNS, statusErr = lb.DNSName().Resolve(ctx)
			if statusErr != nil {
				return
			}
			var frontendNlbDNS string
			if frontendNlb != nil {
				frontendNlbDNS, statusErr = frontendNlb.DNSName().Resolve(ctx)
				if statusErr != nil {
					return
				}
			}
			statusErr = r.updateIngressGroupStatus(ctx, ingGroup, lbDNS, frontendNlbDNS)
			if statusErr != nil {
				r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedUpdateStatus,
					fmt.Sprintf("Failed update status due to %v", statusErr))
			}
		}
		r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "dns_resolve_and_update_status", dnsResolveAndUpdateStatus)
		if statusErr != nil {
			return ctrlerrors.NewErrorWithMetrics(controllerName, "dns_resolve_and_update_status_error", statusErr, r.metricsCollector)
		}
	}

	if len(ingGroup.InactiveMembers) > 0 {
		removeGroupFinalizerFn := func() {
			err = r.groupFinalizerManager.RemoveGroupFinalizer(ctx, ingGroupID, ingGroup.InactiveMembers)
		}
		r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "remove_group_finalizer", removeGroupFinalizerFn)
		if err != nil {
			r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed remove finalizer due to %v", err))
			return ctrlerrors.NewErrorWithMetrics(controllerName, "remove_group_finalizer_error", err, r.metricsCollector)
		}
	}

	r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeNormal, k8s.IngressEventReasonSuccessfullyReconciled, "Successfully reconciled")
	return nil
}

func (r *groupReconciler) buildAndDeployModel(ctx context.Context, ingGroup ingress.Group) (core.Stack, *elbv2model.LoadBalancer, *elbv2model.LoadBalancer, error) {
	var stack core.Stack
	var lb *elbv2model.LoadBalancer
	var secrets []types.NamespacedName
	var backendSGRequired bool
	var err error
	var frontendNlbTargetGroupDesiredState *core.FrontendNlbTargetGroupDesiredState
	var frontendNlb *elbv2model.LoadBalancer
	buildModelFn := func() {
		stack, lb, secrets, backendSGRequired, frontendNlbTargetGroupDesiredState, frontendNlb, err = r.modelBuilder.Build(ctx, ingGroup, r.metricsCollector)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "build_model", buildModelFn)
	if err != nil {
		r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, nil, ctrlerrors.NewErrorWithMetrics(controllerName, "build_model_error", err, r.metricsCollector)
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, nil, err
	}
	r.logger.Info("successfully built model", "model", stackJSON)

	deployModelFn := func() {
		err = r.stackDeployer.Deploy(ctx, stack, r.metricsCollector, "ingress", frontendNlbTargetGroupDesiredState)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "deploy_model", deployModelFn)
	if err != nil {
		var requeueNeededAfter *ctrlerrors.RequeueNeededAfter
		if errors.As(err, &requeueNeededAfter) {
			return nil, nil, nil, err
		}
		r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		return nil, nil, nil, ctrlerrors.NewErrorWithMetrics(controllerName, "deploy_model_error", err, r.metricsCollector)
	}
	r.logger.Info("successfully deployed model", "ingressGroup", ingGroup.ID)
	r.secretsManager.MonitorSecrets(ingGroup.ID.String(), secrets)
	var inactiveResources []types.NamespacedName
	inactiveResources = append(inactiveResources, k8s.ToSliceOfNamespacedNames(ingGroup.InactiveMembers)...)
	if !backendSGRequired {
		inactiveResources = append(inactiveResources, k8s.ToSliceOfNamespacedNames(ingGroup.Members)...)
	}
	if err := r.backendSGProvider.Release(ctx, networkingpkg.ResourceTypeIngress, inactiveResources); err != nil {
		return nil, nil, nil, ctrlerrors.NewErrorWithMetrics(controllerName, "release_auto_generated_backend_sg_error", err, r.metricsCollector)
	}
	return stack, lb, frontendNlb, nil
}

func (r *groupReconciler) recordIngressGroupEvent(_ context.Context, ingGroup ingress.Group, eventType string, reason string, message string) {
	for _, member := range ingGroup.Members {
		r.eventRecorder.Event(member.Ing, eventType, reason, message)
	}
}

func (r *groupReconciler) updateIngressGroupStatus(ctx context.Context, ingGroup ingress.Group, lbDNS string, frontendNLBDNS string) error {
	for _, member := range ingGroup.Members {
		if err := r.updateIngressStatus(ctx, lbDNS, frontendNLBDNS, member.Ing); err != nil {
			return err
		}
	}
	return nil
}

func (r *groupReconciler) updateIngressStatus(ctx context.Context, lbDNS string, frontendNlbDNS string, ing *networking.Ingress) error {
	ingOld := ing.DeepCopy()
	if len(ing.Status.LoadBalancer.Ingress) != 1 ||
		ing.Status.LoadBalancer.Ingress[0].IP != "" ||
		ing.Status.LoadBalancer.Ingress[0].Hostname != lbDNS {
		ing.Status.LoadBalancer.Ingress = []networking.IngressLoadBalancerIngress{
			{
				Hostname: lbDNS,
			},
		}
	}

	// Ensure frontendNLBDNS is appended if it is not already added
	if frontendNlbDNS != "" && !hasFrontendNlbHostName(ing.Status.LoadBalancer.Ingress, frontendNlbDNS) {
		ing.Status.LoadBalancer.Ingress = append(ing.Status.LoadBalancer.Ingress, networking.IngressLoadBalancerIngress{
			Hostname: frontendNlbDNS,
		})
	}

	if !isIngressStatusEqual(ingOld.Status.LoadBalancer.Ingress, ing.Status.LoadBalancer.Ingress) {
		if err := r.k8sClient.Status().Patch(ctx, ing, client.MergeFrom(ingOld)); err != nil {
			return errors.Wrapf(err, "failed to update ingress status: %v", k8s.NamespacedName(ing))
		}

	}

	return nil
}

func (r *groupReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, clientSet *kubernetes.Clientset) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		MaxConcurrentReconciles: r.maxConcurrentReconciles,
		Reconciler:              r,
	})
	if err != nil {
		return err
	}

	resList, err := clientSet.ServerResourcesForGroupVersion(ingressResourcesGroupVersion)
	if err != nil {
		return err
	}
	ingressClassResourceAvailable := isResourceKindAvailable(resList, ingressClassKind)
	if err := r.setupIndexes(ctx, mgr.GetFieldIndexer(), ingressClassResourceAvailable); err != nil {
		return err
	}
	if err := r.setupWatches(ctx, c, mgr, ingressClassResourceAvailable, clientSet); err != nil {
		return err
	}
	return nil
}

func (r *groupReconciler) setupIndexes(ctx context.Context, fieldIndexer client.FieldIndexer, ingressClassResourceAvailable bool) error {
	if err := fieldIndexer.IndexField(ctx, &networking.Ingress{}, ingress.IndexKeyServiceRefName,
		func(obj client.Object) []string {
			return r.referenceIndexer.BuildServiceRefIndexes(context.Background(), obj.(*networking.Ingress))
		},
	); err != nil {
		return err
	}
	if err := fieldIndexer.IndexField(ctx, &networking.Ingress{}, ingress.IndexKeySecretRefName,
		func(obj client.Object) []string {
			return r.referenceIndexer.BuildSecretRefIndexes(context.Background(), obj.(*networking.Ingress))
		},
	); err != nil {
		return err
	}
	if err := fieldIndexer.IndexField(ctx, &corev1.Service{}, ingress.IndexKeySecretRefName,
		func(obj client.Object) []string {
			return r.referenceIndexer.BuildSecretRefIndexes(context.Background(), obj.(*corev1.Service))
		},
	); err != nil {
		return err
	}
	if ingressClassResourceAvailable {
		if err := fieldIndexer.IndexField(ctx, &networking.IngressClass{}, ingress.IndexKeyIngressClassParamsRefName,
			func(obj client.Object) []string {
				return r.referenceIndexer.BuildIngressClassParamsRefIndexes(ctx, obj.(*networking.IngressClass))
			},
		); err != nil {
			return err
		}
		if err := fieldIndexer.IndexField(ctx, &networking.Ingress{}, ingress.IndexKeyIngressClassRefName,
			func(obj client.Object) []string {
				return r.referenceIndexer.BuildIngressClassRefIndexes(ctx, obj.(*networking.Ingress))
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *groupReconciler) setupWatches(_ context.Context, c controller.Controller, mgr ctrl.Manager, ingressClassResourceAvailable bool, clientSet *kubernetes.Clientset) error {
	ingEventChan := make(chan event.TypedGenericEvent[*networking.Ingress])
	svcEventChan := make(chan event.TypedGenericEvent[*corev1.Service])
	secretEventsChan := make(chan event.TypedGenericEvent[*corev1.Secret])
	ingEventHandler := eventhandlers.NewEnqueueRequestsForIngressEvent(r.groupLoader, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("ingress"))
	svcEventHandler := eventhandlers.NewEnqueueRequestsForServiceEvent(ingEventChan, r.k8sClient, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("service"))
	secretEventHandler := eventhandlers.NewEnqueueRequestsForSecretEvent(ingEventChan, svcEventChan, r.k8sClient, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("secret"))
	if err := c.Watch(source.Channel(ingEventChan, ingEventHandler)); err != nil {
		return err
	}
	if err := c.Watch(source.Channel(svcEventChan, svcEventHandler)); err != nil {
		return err
	}
	if err := c.Watch(source.Kind(mgr.GetCache(), &networking.Ingress{}, ingEventHandler)); err != nil {
		return err
	}
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, svcEventHandler)); err != nil {
		return err
	}
	if err := c.Watch(source.Channel(secretEventsChan, secretEventHandler)); err != nil {
		return err
	}
	if ingressClassResourceAvailable {
		ingClassEventChan := make(chan event.TypedGenericEvent[*networking.IngressClass])
		ingClassParamsEventHandler := eventhandlers.NewEnqueueRequestsForIngressClassParamsEvent(ingClassEventChan, r.k8sClient, r.eventRecorder,
			r.logger.WithName("eventHandlers").WithName("ingressClassParams"))
		ingClassEventHandler := eventhandlers.NewEnqueueRequestsForIngressClassEvent(ingEventChan, r.k8sClient, r.eventRecorder,
			r.logger.WithName("eventHandlers").WithName("ingressClass"))
		if err := c.Watch(source.Channel(ingClassEventChan, ingClassEventHandler)); err != nil {
			return err
		}
		if err := c.Watch(source.Kind(mgr.GetCache(), &elbv2api.IngressClassParams{}, ingClassParamsEventHandler)); err != nil {
			return err
		}
		if err := c.Watch(source.Kind(mgr.GetCache(), &networking.IngressClass{}, ingClassEventHandler)); err != nil {
			return err
		}
	}
	r.secretsManager = k8s.NewSecretsManager(clientSet, secretEventsChan, ctrl.Log.WithName("secrets-manager"))
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

func isIngressStatusEqual(a, b []networking.IngressLoadBalancerIngress) bool {
	if len(a) != len(b) {
		return false
	}

	setA := make(map[string]struct{}, len(a))
	setB := make(map[string]struct{}, len(b))

	for _, ingress := range a {
		setA[ingress.Hostname] = struct{}{}
	}

	for _, ingress := range b {
		setB[ingress.Hostname] = struct{}{}
	}

	for key := range setA {
		if _, exists := setB[key]; !exists {
			return false
		}
	}
	return true
}

func hasFrontendNlbHostName(ingressList []networking.IngressLoadBalancerIngress, frontendNlbDNS string) bool {
	for _, ingress := range ingressList {
		if ingress.Hostname == frontendNlbDNS {
			return true
		}

	}
	return false
}
