package conditions

import (
	"encoding/json"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
)

const UseConditionAnnotation = "use-annotation"

type Config struct {
	Conditions map[string][]RuleCondition
}

// NewParser creates a new target group annotation parser
func NewParser() parser.IngressAnnotation {
	return &conditionsParser{}
}

type conditionsParser struct {
}

// Parse parses the annotations contained in the resource
func (p *conditionsParser) Parse(ing parser.AnnotationInterface) (interface{}, error) {
	conditionsByName := make(map[string][]RuleCondition)
	annos, err := parser.GetStringAnnotations("conditions", ing)
	if err != nil {
		if errors.IsMissingAnnotations(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	for serviceName, raw := range annos {
		var conditions []RuleCondition
		err := json.Unmarshal([]byte(raw), &conditions)
		if err != nil {
			return nil, err
		}
		for _, condition := range conditions {
			if err := condition.validate(); err != nil {
				return nil, err
			}
		}

		conditionsByName[serviceName] = conditions
	}

	return &Config{
		Conditions: conditionsByName,
	}, nil
}

// GetConditions returns the conditions named serviceName configured by an annotation
func (c *Config) GetConditions(serviceName string) []RuleCondition {
	conditions, ok := c.Conditions[serviceName]
	if !ok {
		return nil
	}
	return conditions
}

// Use returns true if the parameter requested an annotation configured action
func Use(s string) bool {
	return s == UseConditionAnnotation
}
