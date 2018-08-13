package action

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

type Config struct {
	Actions map[string]*elbv2.Action
}

type action struct {
	r resolver.Resolver
}

// NewParser creates a new target group annotation parser
func NewParser(r resolver.Resolver) parser.IngressAnnotation {
	return action{r}
}

// Parse parses the annotations contained in the resource
func (a action) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	actions := make(map[string]*elbv2.Action)
	annos, err := parser.GetStringAnnotations("actions", ing)
	if err != nil {
		return nil, err
	}

	for serviceName, raw := range annos {
		var data *elbv2.Action
		err := json.Unmarshal([]byte(raw), &data)
		if err != nil {
			return nil, err
		}
		switch *data.Type {
		case "fixed-response":
			if data.FixedResponseConfig == nil {
				return nil, fmt.Errorf("%v is type fixed-response but did not include a valid FixedResponseConfig configuration", serviceName)
			}
		case "redirect":
			if data.RedirectConfig == nil {
				return nil, fmt.Errorf("%v is type redirect but did not include a valid RedirectConfig configuration", serviceName)
			}
		default:
			return nil, fmt.Errorf("an invalid action type %v was configured in %v", *data.Type, serviceName)
		}
		setDefaults(data)
		actions[serviceName] = data
	}

	return &Config{
		Actions: actions,
	}, nil
}

func setDefaults(d *elbv2.Action) {
	if d.RedirectConfig != nil {
		if d.RedirectConfig.Host == nil {
			d.RedirectConfig.Host = aws.String("#{host}")
		}
		if d.RedirectConfig.Path == nil {
			d.RedirectConfig.Path = aws.String("/#{path}")
		}
		if d.RedirectConfig.Port == nil {
			d.RedirectConfig.Port = aws.String("#{port}")
		}
		if d.RedirectConfig.Protocol == nil {
			d.RedirectConfig.Protocol = aws.String("#{protocol}")
		}
		if d.RedirectConfig.Query == nil {
			d.RedirectConfig.Query = aws.String("#{query}")
		}
	}
}

func Dummy() *Config {
	return &Config{
		Actions: map[string]*elbv2.Action{
			"fixed-response-action": &elbv2.Action{
				Type: aws.String("fixed-response"),
				FixedResponseConfig: &elbv2.FixedResponseActionConfig{
					ContentType: aws.String("text/plain"),
					StatusCode:  aws.String("503"),
					MessageBody: aws.String("message body"),
				},
			},
		},
	}
}
