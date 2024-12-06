package ingress

import (
	"context"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"strconv"
)

// buildLoadBalancerMinimumCapacity builds the minimum load balancer capacity for load balancer
func (t *defaultModelBuildTask) buildLoadBalancerMinimumCapacity(_ context.Context) (*elbv2model.MinimumLoadBalancerCapacity, error) {
	if !t.featureGates.Enabled(config.LBCapacityReservation) {
		return nil, nil
	}
	ingGroupCapacityUnits, err := t.buildIngressGroupLoadBalancerMinimumCapacity(t.ingGroup.Members)
	if err != nil {
		return nil, err
	}
	var minimumLoadBalancerCapacity *elbv2model.MinimumLoadBalancerCapacity
	var capacityUnits int64
	for key, value := range ingGroupCapacityUnits {
		if key != elbv2model.CapacityUnits {
			return nil, errors.Errorf("invalid key to set the capacity: %v, Expected key: %v", key, elbv2model.CapacityUnits)
		}
		capacityUnits, _ = strconv.ParseInt(value, 10, 64)
		minimumLoadBalancerCapacity = &elbv2model.MinimumLoadBalancerCapacity{
			CapacityUnits: int32(capacityUnits),
		}

	}
	return minimumLoadBalancerCapacity, nil
}

// buildIngressGroupLoadBalancerMinimumCapacity builds the minimum load balancer capacity for ingresses within a group.
// Note: the capacity reservation specified via IngressClass takes higher priority than the capacity specified via annotation on Ingress.
func (t *defaultModelBuildTask) buildIngressGroupLoadBalancerMinimumCapacity(ingList []ClassifiedIngress) (map[string]string, error) {
	if len(ingList) > 0 {
		ingClassCapacityUnits, _ := t.buildIngressClassLoadBalancerMinimumCapacity(ingList[0].IngClassConfig)
		if ingClassCapacityUnits != nil {
			return ingClassCapacityUnits, nil
		}
	}
	ingGroupCapacityUnits := make(map[string]string)
	for _, ing := range ingList {
		ingGroupCapacity, err := t.buildIngressLoadBalancerMinimumCapacity(ing)
		if err != nil {
			return nil, err
		}
		// check for conflict capacity values
		for capacityKey, capacityValue := range ingGroupCapacity {
			existingCapacityValue, exists := ingGroupCapacityUnits[capacityKey]
			if exists && existingCapacityValue != capacityValue {
				return nil, errors.Errorf("conflicting capacity reservation %v: %v | %v", capacityKey, existingCapacityValue, capacityValue)
			}
			ingGroupCapacityUnits[capacityKey] = capacityValue
		}
	}
	return ingGroupCapacityUnits, nil
}

// buildIngressLoadBalancerMinimumCapacity builds the minimum load balancer capacity used for a single ingress within a group
func (t *defaultModelBuildTask) buildIngressLoadBalancerMinimumCapacity(ing ClassifiedIngress) (map[string]string, error) {
	var annotationCapacity map[string]string
	if _, err := t.annotationParser.ParseStringMapAnnotation(annotations.IngressSuffixLoadBalancerCapacityReservation, &annotationCapacity, ing.Ing.Annotations); err != nil {
		return nil, err
	}
	return annotationCapacity, nil
}

// buildIngressClassLoadBalancerMinimumCapacity builds the minimum load balancer capacity for an IngressClass.
func (t *defaultModelBuildTask) buildIngressClassLoadBalancerMinimumCapacity(ingClassConfig ClassConfiguration) (map[string]string, error) {
	if ingClassConfig.IngClassParams == nil || ingClassConfig.IngClassParams.Spec.MinimumLoadBalancerCapacity == nil {
		return nil, nil
	}
	capacityUnits := strconv.Itoa(int(ingClassConfig.IngClassParams.Spec.MinimumLoadBalancerCapacity.CapacityUnits))
	ingClassCapacityUnits := make(map[string]string)
	ingClassCapacityUnits[elbv2model.CapacityUnits] = capacityUnits
	return ingClassCapacityUnits, nil
}
