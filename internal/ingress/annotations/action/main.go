package action

import (
	"encoding/json"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"

	extensions "k8s.io/api/extensions/v1beta1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const UseActionAnnotation = "use-annotation"
const default404ServiceName = "Default 404"

type Config struct {
	Actions map[string]Action
}

type actionParser struct{}

// NewParser creates a new target group annotation parser
func NewParser() parser.IngressAnnotation {
	return &actionParser{}
}

// Parse parses the annotations contained in the resource
func (a *actionParser) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	actions := make(map[string]Action)
	annos, err := parser.GetStringAnnotations("actions", ing)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	for serviceName, raw := range annos {
		action := Action{}
		err := json.Unmarshal([]byte(raw), &action)
		if err != nil {
			return nil, err
		}
		if err := action.validate(); err != nil {
			return nil, err
		}
		action.setDefaults()
		actions[serviceName] = action
	}

	return &Config{
		Actions: actions,
	}, nil
}

// GetAction returns the action named serviceName configured by an annotation
func (c *Config) GetAction(serviceName string) (Action, error) {
	if serviceName == default404ServiceName {
		return default404Action(), nil
	}

	action, ok := c.Actions[serviceName]
	if !ok {
		return Action{}, errors.Errorf(
			"backend with `servicePort: %s` was configured with `serviceName: %v` but an action annotation for %v is not set",
			UseActionAnnotation, serviceName, serviceName)
	}
	return action, nil
}

// Use returns true if the parameter requested an annotation configured action
func Use(s string) bool {
	return s == UseActionAnnotation
}

func default404Action() Action {
	return Action{
		Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
		FixedResponseConfig: &FixedResponseActionConfig{
			ContentType: aws.String("text/plain"),
			StatusCode:  aws.String("404"),
		},
	}
}

// Default404Backend turns an IngressBackend that will return 404s
func Default404Backend() extensions.IngressBackend {
	return extensions.IngressBackend{
		ServiceName: default404ServiceName,
		ServicePort: intstr.FromString(UseActionAnnotation),
	}
}

func Dummy() *Config {
	redirectAction := Action{
		Type: aws.String(elbv2.ActionTypeEnumRedirect),
		RedirectConfig: &RedirectActionConfig{
			Protocol:   aws.String(elbv2.ProtocolEnumHttps),
			StatusCode: aws.String(elbv2.RedirectActionStatusCodeEnumHttp301),
		},
	}
	redirectAction.setDefaults()

	redirectPath2Action := Action{
		Type: aws.String(elbv2.ActionTypeEnumRedirect),
		RedirectConfig: &RedirectActionConfig{
			Path:       aws.String("/#{path}2"),
			StatusCode: aws.String(elbv2.RedirectActionStatusCodeEnumHttp301),
		},
	}
	redirectPath2Action.setDefaults()

	fixedResponseAction := Action{
		Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
		FixedResponseConfig: &FixedResponseActionConfig{
			ContentType: aws.String("text/plain"),
			StatusCode:  aws.String("503"),
			MessageBody: aws.String("message body"),
		},
	}
	fixedResponseAction.setDefaults()

	forwardAction := Action{
		Type:           aws.String(elbv2.ActionTypeEnumForward),
		TargetGroupArn: aws.String("legacy-tg-arn"),
	}
	forwardAction.setDefaults()

	return &Config{
		Actions: map[string]Action{
			"redirect":              redirectAction,
			"redirect-path2":        redirectPath2Action,
			"fixed-response-action": fixedResponseAction,
			"forward":               forwardAction,
		},
	}
}
