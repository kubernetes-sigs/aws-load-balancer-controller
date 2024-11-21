package ingress

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

// buildIngressGroupLoadBalancerAttributes builds the LB attributes for a group of Ingresses.
func (t *defaultModelBuildTask) buildIngressGroupLoadBalancerAttributes(ingList []ClassifiedIngress) (map[string]string, error) {
	ingGroupAttributes := make(map[string]string)
	ingClassAttributes := make(map[string]string)
	if len(ingList) > 0 {
		var err error
		ingClassAttributes, err = t.buildIngressClassLoadBalancerAttributes(ingList[0].IngClassConfig)
		if err != nil {
			return nil, err
		}
	}
	for _, ing := range ingList {
		ingAttributes, err := t.buildIngressLoadBalancerAttributes(ing)
		if err != nil {
			return nil, err
		}
		// check for conflict attribute values
		for attrKey, attrValue := range ingAttributes {
			existingAttrValue, exists := ingGroupAttributes[attrKey]
			if exists && existingAttrValue != attrValue {
				if ingClassValue, exists := ingClassAttributes[attrKey]; exists {
					// Conflict is resolved by ingClassAttributes, show a warning
					t.logger.Info("load balancer attribute conflict resolved by ingress class",
						"attributeKey", attrKey,
						"existingValue", existingAttrValue,
						"newValue", attrValue,
						"ingClassValue", ingClassValue)
				} else {
					// Conflict is not resolved by ingClassAttributes, return an error
					return nil, errors.Errorf("conflicting load balancer attributes %v: %v | %v", attrKey, existingAttrValue, attrValue)
				}
			}
			ingGroupAttributes[attrKey] = attrValue
		}
	}
	ingGroupAttributes = algorithm.MergeStringMap(ingClassAttributes, ingGroupAttributes)
	return ingGroupAttributes, nil
}

// buildIngressLoadBalancerAttributes builds the LB attributes used for a single Ingress
// Note: the Attributes specified via IngressClass takes higher priority than the attributes specified via annotation on Ingress or Service.
func (t *defaultModelBuildTask) buildIngressLoadBalancerAttributes(ing ClassifiedIngress) (map[string]string, error) {
	var annotationAttributes map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixLoadBalancerAttributes, &annotationAttributes, ing.Ing.Annotations); err != nil {
		return nil, err
	}
	return annotationAttributes, nil
}

// buildIngressClassLoadBalancerAttributes builds the LB attributes for an IngressClass.
func (t *defaultModelBuildTask) buildIngressClassLoadBalancerAttributes(ingClassConfig ClassConfiguration) (map[string]string, error) {
	if ingClassConfig.IngClassParams == nil || len(ingClassConfig.IngClassParams.Spec.LoadBalancerAttributes) == 0 {
		return nil, nil
	}
	ingClassAttributes := make(map[string]string, len(ingClassConfig.IngClassParams.Spec.LoadBalancerAttributes))
	for _, attr := range ingClassConfig.IngClassParams.Spec.LoadBalancerAttributes {
		ingClassAttributes[attr.Key] = attr.Value
	}
	return ingClassAttributes, nil
}
