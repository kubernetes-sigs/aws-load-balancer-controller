package gateway

import (
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sort"
)

func mergeAttributes[T interface{}](highPriority, lowPriority []T, keyFn func(T) string, valueFn func(T) string, constructor func(string, string) T) []T {
	baseAttributesMap := make(map[string]string)

	for _, attr := range highPriority {
		k := keyFn(attr)
		v := valueFn(attr)
		baseAttributesMap[k] = v
	}

	for _, attr := range lowPriority {
		k := keyFn(attr)
		v := valueFn(attr)
		_, found := baseAttributesMap[k]
		if !found {
			baseAttributesMap[k] = v
		}
	}

	mergedAttributes := make([]T, 0, len(baseAttributesMap))
	if len(baseAttributesMap) > 0 {
		for k, v := range baseAttributesMap {
			mergedAttributes = append(mergedAttributes, constructor(k, v))
		}

		sort.Slice(mergedAttributes, func(i, j int) bool {
			return keyFn(mergedAttributes[i]) < keyFn(mergedAttributes[j])
		})
	}
	return mergedAttributes
}

// Hack to work around this error..
// /Users/nixozach/go/bin/controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."
// sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1:-: invalid type: interface{GetKey() string; GetValue() string}
// Error: not all generators ran successfully

func loadBalancerAttributeConstructor(k, v string) elbv2gw.LoadBalancerAttribute {
	return elbv2gw.LoadBalancerAttribute{
		Key:   k,
		Value: v,
	}
}

func loadBalancerAttributeKeyFn(a elbv2gw.LoadBalancerAttribute) string {
	return a.Key
}

func loadBalancerAttributeValueFn(a elbv2gw.LoadBalancerAttribute) string {
	return a.Value
}

func targetGroupAttributeConstructor(k, v string) elbv2gw.TargetGroupAttribute {
	return elbv2gw.TargetGroupAttribute{
		Key:   k,
		Value: v,
	}
}

func targetGroupAttributeKeyFn(a elbv2gw.TargetGroupAttribute) string {
	return a.Key
}

func targetGroupAttributeValueFn(a elbv2gw.TargetGroupAttribute) string {
	return a.Value
}
