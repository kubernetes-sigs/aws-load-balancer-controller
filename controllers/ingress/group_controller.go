package ingress

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/ingress/eventhandlers"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
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
	ingressTagPrefix = "ingress.k8s.aws"
	controllerName   = "ingress"

	// the groupVersion of used Ingress & IngressClass resource.
	ingressResourcesGroupVersion = "networking.k8s.io/v1"
	ingressClassKind             = "IngressClass"
)

// NewGroupReconciler constructs new GroupReconciler
func NewGroupReconciler(cloud aws.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager, networkingSGManager networkingpkg.SecurityGroupManager,
	networkingSGReconciler networkingpkg.SecurityGroupReconciler, subnetsResolver networkingpkg.SubnetsResolver,
	controllerConfig config.ControllerConfig, backendSGProvider networkingpkg.BackendSGProvider, logger logr.Logger) *groupReconciler {

	annotationParser := annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress)
	authConfigBuilder := ingress.NewDefaultAuthConfigBuilder(annotationParser)
	enhancedBackendBuilder := ingress.NewDefaultEnhancedBackendBuilder(k8sClient, annotationParser, authConfigBuilder)
	referenceIndexer := ingress.NewDefaultReferenceIndexer(enhancedBackendBuilder, authConfigBuilder, logger)
	trackingProvider := tracking.NewDefaultProvider(ingressTagPrefix, controllerConfig.ClusterName)
	elbv2TaggingManager := elbv2deploy.NewDefaultTaggingManager(cloud.ELBV2(), cloud.VpcID(), controllerConfig.FeatureGates, logger)
	modelBuilder := ingress.NewDefaultModelBuilder(k8sClient, eventRecorder,
		cloud.EC2(), cloud.ACM(),
		annotationParser, subnetsResolver,
		authConfigBuilder, enhancedBackendBuilder, trackingProvider, elbv2TaggingManager, controllerConfig.FeatureGates,
		cloud.VpcID(), controllerConfig.ClusterName, controllerConfig.DefaultTags, controllerConfig.ExternalManagedTags,
		controllerConfig.DefaultSSLPolicy, backendSGProvider, controllerConfig.EnableBackendSecurityGroup, controllerConfig.DisableRestrictedSGRules, controllerConfig.FeatureGates.Enabled(config.EnableIPTargetType), logger)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	stackDeployer := deploy.NewDefaultStackDeployer(cloud, k8sClient, networkingSGManager, networkingSGReconciler,
		controllerConfig, ingressTagPrefix, logger)
	classLoader := ingress.NewDefaultClassLoader(k8sClient)
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

func (r *groupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return runtime.HandleReconcileError(r.reconcile(ctx, req), r.logger)
}

func (r *groupReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	ingGroupID := ingress.DecodeGroupIDFromReconcileRequest(req)
	ingGroup, err := r.groupLoader.Load(ctx, ingGroupID)
	if err != nil {
		return err
	}

	if err := r.groupFinalizerManager.AddGroupFinalizer(ctx, ingGroupID, ingGroup.Members); err != nil {
		r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
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
		if err := r.updateIngressGroupStatus(ctx, ingGroup, lbDNS); err != nil {
			r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedUpdateStatus, fmt.Sprintf("Failed update status due to %v", err))
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
			r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed remove finalizer due to %v", err))
			return err
		}
	}

	r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeNormal, k8s.IngressEventReasonSuccessfullyReconciled, "Successfully reconciled")
	return nil
}

func (r *groupReconciler) buildAndDeployModel(ctx context.Context, ingGroup ingress.Group) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack, lb, secrets, err := r.modelBuilder.Build(ctx, ingGroup)
	if err != nil {
		r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}
	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}
	r.logger.Info("successfully built model", "model", stackJSON)

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		r.recordIngressGroupEvent(ctx, ingGroup, corev1.EventTypeWarning, k8s.IngressEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		return nil, nil, err
	}
	r.logger.Info("successfully deployed model", "ingressGroup", ingGroup.ID)
	r.secretsManager.MonitorSecrets(ingGroup.ID.String(), secrets)
	return stack, lb, err
}

func (r *groupReconciler) recordIngressGroupEvent(_ context.Context, ingGroup ingress.Group, eventType string, reason string, message string) {
	for _, member := range ingGroup.Members {
		r.eventRecorder.Event(member.Ing, eventType, reason, message)
	}
}

func (r *groupReconciler) updateIngressGroupStatus(ctx context.Context, ingGroup ingress.Group, lbDNS string) error {
	for _, member := range ingGroup.Members {
		if err := r.updateIngressStatus(ctx, lbDNS, member.Ing); err != nil {
			return err
		}
	}
	return nil
}

func (r *groupReconciler) updateIngressStatus(ctx context.Context, lbDNS string, ing *networking.Ingress) error {
	if len(ing.Status.LoadBalancer.Ingress) != 1 ||
		ing.Status.LoadBalancer.Ingress[0].IP != "" ||
		ing.Status.LoadBalancer.Ingress[0].Hostname != lbDNS {
		ingOld := ing.DeepCopy()
		ing.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				Hostname: lbDNS,
			},
		}
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
	if err := r.setupWatches(ctx, c, ingressClassResourceAvailable, clientSet); err != nil {
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

func (r *groupReconciler) setupWatches(_ context.Context, c controller.Controller, ingressClassResourceAvailable bool, clientSet *kubernetes.Clientset) error {
	ingEventChan := make(chan event.GenericEvent)
	svcEventChan := make(chan event.GenericEvent)
	secretEventsChan := make(chan event.GenericEvent)
	ingEventHandler := eventhandlers.NewEnqueueRequestsForIngressEvent(r.groupLoader, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("ingress"))
	svcEventHandler := eventhandlers.NewEnqueueRequestsForServiceEvent(ingEventChan, r.k8sClient, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("service"))
	secretEventHandler := eventhandlers.NewEnqueueRequestsForSecretEvent(ingEventChan, svcEventChan, r.k8sClient, r.eventRecorder,
		r.logger.WithName("eventHandlers").WithName("secret"))
	if err := c.Watch(&source.Channel{Source: ingEventChan}, ingEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Channel{Source: svcEventChan}, svcEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &networking.Ingress{}}, ingEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, svcEventHandler); err != nil {
		return err
	}
	if err := c.Watch(&source.Channel{Source: secretEventsChan}, secretEventHandler); err != nil {
		return err
	}
	if ingressClassResourceAvailable {
		ingClassEventChan := make(chan event.GenericEvent)
		ingClassParamsEventHandler := eventhandlers.NewEnqueueRequestsForIngressClassParamsEvent(ingClassEventChan, r.k8sClient, r.eventRecorder,
			r.logger.WithName("eventHandlers").WithName("ingressClassParams"))
		ingClassEventHandler := eventhandlers.NewEnqueueRequestsForIngressClassEvent(ingEventChan, r.k8sClient, r.eventRecorder,
			r.logger.WithName("eventHandlers").WithName("ingressClass"))
		if err := c.Watch(&source.Channel{Source: ingClassEventChan}, ingClassEventHandler); err != nil {
			return err
		}
		if err := c.Watch(&source.Kind{Type: &elbv2api.IngressClassParams{}}, ingClassParamsEventHandler); err != nil {
			return err
		}
		if err := c.Watch(&source.Kind{Type: &networking.IngressClass{}}, ingClassEventHandler); err != nil {
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
