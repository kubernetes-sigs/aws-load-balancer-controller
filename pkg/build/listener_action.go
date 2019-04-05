package build

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
)

const (
	ActionUseAnnotation = "use-annotation"
)

const (
	RedirectOriginalHost     = "#{host}"
	RedirectOriginalPath     = "/#{path}"
	RedirectOriginalPort     = "#{port}"
	RedirectOriginalProtocol = "#{protocol}"
	RedirectOriginalQuery    = "#{query}"
)

// buildListenerAction will build the listener action based on specified ingress & backend.
func (b *defaultBuilder) buildListenerActions(ctx context.Context, stack *LoadBalancingStack, groupID ingress.GroupID,
	ing *extensions.Ingress, backend extensions.IngressBackend, protocol api.Protocol) ([]api.ListenerAction, error) {

	if backend.ServicePort.String() == ActionUseAnnotation {
		authActions, err := b.authActionBuilder.Build(ctx, ing, map[string]string{}, protocol)
		if err != nil {
			return nil, err
		}
		actionByAnnotation, err := b.buildActionByAnnotation(ctx, backend.ServiceName, ing.Annotations)
		if err != nil {
			return nil, err
		}
		return append(authActions, actionByAnnotation), nil
	}

	svc := &corev1.Service{}
	if err := b.cache.Get(ctx, types.NamespacedName{Namespace: ing.Namespace, Name: backend.ServiceName}, svc); err != nil {
		return nil, err
	}
	authActions, err := b.authActionBuilder.Build(ctx, ing, svc.Annotations, protocol)
	if err != nil {
		return nil, err
	}

	tg, err := b.buildTargetGroup(ctx, stack, groupID, ing, svc, backend.ServicePort)
	if err != nil {
		return nil, err
	}
	forwardAction := b.buildForwardAction(api.TargetGroupReference{TargetGroupRef: k8s.LocalObjectReference(tg)})
	return append(authActions, forwardAction), nil
}

func (b *defaultBuilder) buildActionByAnnotation(ctx context.Context, actionName string, ingAnnotations map[string]string) (api.ListenerAction, error) {
	annotationSuffix := fmt.Sprintf(k8s.AnnotationSuffixActionPattern, actionName)
	rawAction := ""
	if exists := b.annotationParser.ParseStringAnnotation(annotationSuffix, &rawAction, ingAnnotations); !exists {
		return api.ListenerAction{}, errors.Errorf("action %s not found", actionName)
	}

	var action elbv2.Action
	if err := json.Unmarshal([]byte(rawAction), &action); err != nil {
		return api.ListenerAction{}, errors.Wrapf(err, "action %s is malformed", actionName)
	}
	if err := action.Validate(); err != nil {
		return api.ListenerAction{}, errors.Wrapf(err, "action %s is malformed", actionName)
	}

	switch actionType := api.ListenerActionType(*action.Type); actionType {
	case api.ListenerActionTypeFixedResponse:
		if action.FixedResponseConfig == nil {
			return api.ListenerAction{}, errors.Errorf("action %s is type %v, but did't include a valid %s configuration", actionName, api.ListenerActionTypeFixedResponse, "FixedResponseConfig")
		}
		return b.buildFixedResponseAction(*action.FixedResponseConfig), nil
	case api.ListenerActionTypeRedirect:
		if action.RedirectConfig == nil {
			return api.ListenerAction{}, errors.Errorf("action %s is type %v, but did't include a valid %s configuration", actionName, api.ListenerActionTypeRedirect, "RedirectConfig")
		}
		return b.buildRedirectAction(*action.RedirectConfig), nil
	case api.ListenerActionTypeForward:
		if action.TargetGroupArn == nil {
			return api.ListenerAction{}, errors.Errorf("action %s is type %v, but did't include a valid %s configuration", actionName, api.ListenerActionTypeForward, "TargetGroupArn")
		}
		return b.buildForwardAction(api.TargetGroupReference{TargetGroupARN: aws.StringValue(action.TargetGroupArn)}), nil
	default:
		return api.ListenerAction{}, errors.Errorf("action %s is type %v, which is unsupported", actionName, actionType)
	}
}

func (b *defaultBuilder) buildFixedResponseAction(config elbv2.FixedResponseActionConfig) api.ListenerAction {
	return api.ListenerAction{
		Type: api.ListenerActionTypeFixedResponse,
		FixedResponse: &api.FixedResponseConfig{
			ContentType: aws.StringValue(config.ContentType),
			MessageBody: aws.StringValue(config.MessageBody),
			StatusCode:  aws.StringValue(config.StatusCode),
		},
	}
}

func (b *defaultBuilder) buildRedirectAction(config elbv2.RedirectActionConfig) api.ListenerAction {
	action := api.ListenerAction{
		Type:     api.ListenerActionTypeRedirect,
		Redirect: &api.RedirectConfig{},
	}
	if config.Host != nil {
		action.Redirect.Host = aws.StringValue(config.Host)
	} else {
		action.Redirect.Host = RedirectOriginalHost
	}
	if config.Path != nil {
		action.Redirect.Path = aws.StringValue(config.Path)
	} else {
		action.Redirect.Path = RedirectOriginalPath
	}
	if config.Port != nil {
		action.Redirect.Port = aws.StringValue(config.Port)
	} else {
		action.Redirect.Port = RedirectOriginalPort
	}
	if config.Protocol != nil {
		action.Redirect.Protocol = aws.StringValue(config.Protocol)
	} else {
		action.Redirect.Protocol = RedirectOriginalProtocol
	}
	if config.Query != nil {
		action.Redirect.Query = aws.StringValue(config.Query)
	} else {
		action.Redirect.Query = RedirectOriginalQuery
	}
	action.Redirect.StatusCode = aws.StringValue(config.StatusCode)

	return action
}

func (b *defaultBuilder) buildForwardAction(tgRef api.TargetGroupReference) api.ListenerAction {
	return api.ListenerAction{
		Type: api.ListenerActionTypeForward,
		Forward: &api.ForwardConfig{
			TargetGroup: tgRef,
		},
	}
}

func (b *defaultBuilder) buildDefault404Action() api.ListenerAction {
	return b.buildFixedResponseAction(elbv2.FixedResponseActionConfig{
		ContentType: aws.String("text/plain"),
		StatusCode:  aws.String("404"),
	})
}
