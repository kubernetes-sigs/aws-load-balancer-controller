package action

import (
	"encoding/json"

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
		actions[serviceName] = data
	}

	return &Config{
		Actions: actions,
	}, nil
}

func Dummy() *Config {
	return &Config{
		Actions: make(map[string]*elbv2.Action),
	}
}
