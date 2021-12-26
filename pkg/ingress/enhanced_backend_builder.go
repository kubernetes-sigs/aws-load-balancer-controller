package ingress

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	magicServicePortUseAnnotation = "use-annotation"

	// the message body of fixed 503 response used when referencing a non-existent Kubernetes service as backend.
	nonExistentBackendServiceMessageBody = "Backend service does not exist"
	// the message body of fixed 503 response used when referencing a non-existent annotation Action as backend.
	nonExistentBackendActionMessageBody = "Backend action does not exist"
	// by default, we tolerate a missing backend service, and use a fixed 503 response instead.
	defaultTolerateNonExistentBackendService = true
	// by default, we tolerate a missing backend action, and use a fixed 503 response instead.
	defaultTolerateNonExistentBackendAction = true
)

// EnhancedBackend is an enhanced version of Ingress backend.
// It contains additional routing conditions and authentication configurations we parsed from annotations.
// Also, when magic string `use-annotation` is specified as backend, the actions will be parsed from annotations as well.
type EnhancedBackend struct {
	Conditions []RuleCondition
	Action     Action
	AuthConfig AuthConfig
}

type EnhancedBackendBuildOptions struct {
	// whether to load backend services
	LoadBackendServices bool

	// BackendServices contains all services referenced in Action, indexed by service's key.
	// Note: we support to pass BackendServices during backend build, so that we can use the same service snapshot for same service during entire Ingress build process.
	BackendServices map[types.NamespacedName]*corev1.Service

	// whether to load auth configuration. when load authConfiguration, LoadBackendServices must be enabled as well.
	LoadAuthConfig bool
}

type EnhancedBackendBuildOption func(opts *EnhancedBackendBuildOptions)

func (opts *EnhancedBackendBuildOptions) ApplyOptions(options ...EnhancedBackendBuildOption) {
	for _, option := range options {
		option(opts)
	}
}

// WithLoadBackendServices is a option that sets the WithLoadBackendServices and BackendServices.
func WithLoadBackendServices(loadBackendServices bool, backendServices map[types.NamespacedName]*corev1.Service) EnhancedBackendBuildOption {
	return func(opts *EnhancedBackendBuildOptions) {
		opts.LoadBackendServices = loadBackendServices
		opts.BackendServices = backendServices
	}
}

// WithLoadAuthConfig is a option that sets the LoadAuthConfig.
func WithLoadAuthConfig(loadAuthConfig bool) EnhancedBackendBuildOption {
	return func(opts *EnhancedBackendBuildOptions) {
		opts.LoadAuthConfig = loadAuthConfig
	}
}

// EnhancedBackendBuilder is capable of build EnhancedBackend for Ingress backend.
type EnhancedBackendBuilder interface {
	Build(ctx context.Context, ing *networking.Ingress, backend networking.IngressBackend, opts ...EnhancedBackendBuildOption) (EnhancedBackend, error)
}

// NewDefaultEnhancedBackendBuilder constructs new defaultEnhancedBackendBuilder.
func NewDefaultEnhancedBackendBuilder(k8sClient client.Client, annotationParser annotations.Parser, authConfigBuilder AuthConfigBuilder) *defaultEnhancedBackendBuilder {
	return &defaultEnhancedBackendBuilder{
		k8sClient:         k8sClient,
		annotationParser:  annotationParser,
		authConfigBuilder: authConfigBuilder,

		tolerateNonExistentBackendService: defaultTolerateNonExistentBackendAction,
		tolerateNonExistentBackendAction:  defaultTolerateNonExistentBackendService,
	}
}

var _ EnhancedBackendBuilder = &defaultEnhancedBackendBuilder{}

// default implementation for defaultEnhancedBackendBuilder
type defaultEnhancedBackendBuilder struct {
	k8sClient         client.Client
	annotationParser  annotations.Parser
	authConfigBuilder AuthConfigBuilder

	// whether to tolerate misconfiguration that used a non-existent backend service.
	// when tolerate, If a single backend service is used and it's non-existent, a fixed 503 response will be used instead.
	tolerateNonExistentBackendService bool
	// whether to tolerate misconfiguration that used a non-existent backend action.
	// when tolerate, If the backend action annotation is non-existent, a fixed 503 response will be used instead.
	tolerateNonExistentBackendAction bool
}

func (b *defaultEnhancedBackendBuilder) Build(ctx context.Context, ing *networking.Ingress, backend networking.IngressBackend, opts ...EnhancedBackendBuildOption) (EnhancedBackend, error) {
	buildOpts := EnhancedBackendBuildOptions{
		LoadBackendServices: true,
		LoadAuthConfig:      true,
		BackendServices:     map[types.NamespacedName]*corev1.Service{},
	}
	buildOpts.ApplyOptions(opts...)

	if backend.Service == nil {
		return EnhancedBackend{}, errors.New("missing required \"service\" field")
	}

	conditions, err := b.buildConditions(ctx, ing.Annotations, backend.Service.Name)
	if err != nil {
		return EnhancedBackend{}, err
	}

	var action Action
	if backend.Service.Port.Name == magicServicePortUseAnnotation {
		action, err = b.buildActionViaAnnotation(ctx, ing.Annotations, backend.Service.Name)
		if err != nil {
			return EnhancedBackend{}, err
		}
	} else if backend.Service.Port.Name != "" {
		action = b.buildActionViaServiceAndServicePort(ctx, backend.Service.Name, intstr.FromString(backend.Service.Port.Name))
	} else {
		action = b.buildActionViaServiceAndServicePort(ctx, backend.Service.Name, intstr.FromInt(int(backend.Service.Port.Number)))
	}

	var authCfg AuthConfig
	if buildOpts.LoadBackendServices {
		if err := b.loadBackendServices(ctx, &action, ing.Namespace, buildOpts.BackendServices); err != nil {
			return EnhancedBackend{}, err
		}

		if buildOpts.LoadAuthConfig {
			authCfg, err = b.buildAuthConfig(ctx, action, ing.Namespace, ing.Annotations, buildOpts.BackendServices)
			if err != nil {
				return EnhancedBackend{}, err
			}
		}
	}

	return EnhancedBackend{
		Conditions: conditions,
		Action:     action,
		AuthConfig: authCfg,
	}, nil
}

func (b *defaultEnhancedBackendBuilder) buildConditions(_ context.Context, ingAnnotation map[string]string, svcName string) ([]RuleCondition, error) {
	var conditions []RuleCondition
	annotationKey := fmt.Sprintf("conditions.%v", svcName)
	_, err := b.annotationParser.ParseJSONAnnotation(annotationKey, &conditions, ingAnnotation)
	if err != nil {
		return nil, err
	}
	for _, condition := range conditions {
		if err := condition.validate(); err != nil {
			return nil, err
		}
	}
	return conditions, nil
}

// buildActionViaAnnotation will build the backend action specified via actions annotation.
func (b *defaultEnhancedBackendBuilder) buildActionViaAnnotation(ctx context.Context, ingAnnotation map[string]string, svcName string) (Action, error) {
	action := Action{}
	annotationKey := fmt.Sprintf("actions.%v", svcName)
	exists, err := b.annotationParser.ParseJSONAnnotation(annotationKey, &action, ingAnnotation)
	if err != nil {
		return Action{}, err
	}
	if !exists {
		if b.tolerateNonExistentBackendAction {
			return b.build503ResponseAction(nonExistentBackendActionMessageBody), nil
		}
		return Action{}, errors.Errorf("missing %v configuration", annotationKey)
	}
	if err := action.validate(); err != nil {
		return Action{}, err
	}
	b.normalizeSimplifiedSchemaForwardAction(ctx, &action)
	b.normalizeServicePortForBackwardsCompatibility(ctx, &action)
	return action, nil
}

// buildActionViaServiceAndServicePort will build the backend Action that forward to specified Kubernetes Service.
func (b *defaultEnhancedBackendBuilder) buildActionViaServiceAndServicePort(_ context.Context, svcName string, svcPort intstr.IntOrString) Action {
	action := Action{
		Type: ActionTypeForward,
		ForwardConfig: &ForwardActionConfig{
			TargetGroups: []TargetGroupTuple{
				{
					ServiceName: &svcName,
					ServicePort: &svcPort,
				},
			},
		},
	}
	return action
}

// normalizeSimplifiedSchemaForwardAction will normalize to the advanced schema for forward action to share common processing logic.
// we support a simplified schema in action annotation when configure forward to a single TargetGroup.
func (b *defaultEnhancedBackendBuilder) normalizeSimplifiedSchemaForwardAction(_ context.Context, action *Action) {
	if action.Type == ActionTypeForward && action.TargetGroupARN != nil {
		*action = Action{
			Type: ActionTypeForward,
			ForwardConfig: &ForwardActionConfig{
				TargetGroups: []TargetGroupTuple{
					{
						TargetGroupARN: action.TargetGroupARN,
					},
				},
			},
		}
	}
}

// normalizeServicePortForBackwardsCompatibility will normalize servicePort to be int type if possible.
// this is for backwards-compatibility with old AWSALBIngressController, where ServicePort is defined as Type string.
func (b *defaultEnhancedBackendBuilder) normalizeServicePortForBackwardsCompatibility(_ context.Context, action *Action) {
	if action.Type == ActionTypeForward && action.ForwardConfig != nil {
		for _, tgt := range action.ForwardConfig.TargetGroups {
			if tgt.ServicePort != nil {
				normalizedSVCPort := intstr.Parse(tgt.ServicePort.String())
				*tgt.ServicePort = normalizedSVCPort
			}
		}
	}
}

// loadBackendServices will load referenced backend services into backendServices.
// when tolerateNonExistentBackendService==true, and forward to a single non-existent Kubernetes Service, a fixed 503 response instead.
func (b *defaultEnhancedBackendBuilder) loadBackendServices(ctx context.Context, action *Action, namespace string,
	backendServices map[types.NamespacedName]*corev1.Service) error {
	if action.Type == ActionTypeForward && action.ForwardConfig != nil {
		svcNames := sets.NewString()
		for _, tgt := range action.ForwardConfig.TargetGroups {
			if tgt.ServiceName != nil {
				svcNames.Insert(awssdk.StringValue(tgt.ServiceName))
			}
		}
		forwardToSingleSvc := (len(action.ForwardConfig.TargetGroups) == 1) && (svcNames.Len() == 1)
		tolerateNonExistentBackendService := b.tolerateNonExistentBackendService && forwardToSingleSvc
		for svcName := range svcNames {
			svcKey := types.NamespacedName{Namespace: namespace, Name: svcName}
			if _, ok := backendServices[svcKey]; ok {
				continue
			}

			svc := &corev1.Service{}
			if err := b.k8sClient.Get(ctx, svcKey, svc); err != nil {
				if apierrors.IsNotFound(err) && tolerateNonExistentBackendService {
					*action = b.build503ResponseAction(nonExistentBackendServiceMessageBody)
					return nil
				}
				return err
			}
			backendServices[svcKey] = svc
		}
	}
	return nil
}

func (b *defaultEnhancedBackendBuilder) buildAuthConfig(ctx context.Context, action Action, namespace string, ingAnnotation map[string]string, backendServices map[types.NamespacedName]*corev1.Service) (AuthConfig, error) {
	svcAndIngAnnotations := ingAnnotation
	// when forward to a single Service, the auth annotations on that Service will be merged in.
	if action.Type == ActionTypeForward &&
		action.ForwardConfig != nil &&
		len(action.ForwardConfig.TargetGroups) == 1 &&
		action.ForwardConfig.TargetGroups[0].ServiceName != nil {
		svcName := awssdk.StringValue(action.ForwardConfig.TargetGroups[0].ServiceName)
		svcKey := types.NamespacedName{Namespace: namespace, Name: svcName}
		svc := backendServices[svcKey]
		svcAndIngAnnotations = algorithm.MergeStringMap(svc.Annotations, svcAndIngAnnotations)
	}

	return b.authConfigBuilder.Build(ctx, svcAndIngAnnotations)
}

// build503ResponseAction generates a 503 fixed response action when forward to a single non-existent Kubernetes Service.
func (b *defaultEnhancedBackendBuilder) build503ResponseAction(messageBody string) Action {
	return Action{
		Type: ActionTypeFixedResponse,
		FixedResponseConfig: &FixedResponseActionConfig{
			ContentType: awssdk.String("text/plain"),
			StatusCode:  "503",
			MessageBody: awssdk.String(messageBody),
		},
	}
}
