package ingress

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

func (t *defaultModelBuildTask) buildIPv4IPAMPoolID() (*string, error) {
	// We give precedence to the IngressClass value for IPv4 IPAM pool ID.
	if len(t.ingGroup.Members) > 0 {
		poolId := t.getIPv4IPAMFromIngressClass(t.ingGroup.Members[0].IngClassConfig)
		if poolId != nil {
			return poolId, nil
		}
	}

	var poolIdToReturn *string

	for _, ing := range t.ingGroup.Members {
		poolId := t.getIPv4IPAMFromAnnotation(ing)

		if poolId != nil {
			if poolIdToReturn != nil && *poolId != *poolIdToReturn {
				return nil, errors.Errorf("conflicting ipv4 ipam pools %v: %v", *poolIdToReturn, *poolId)
			}
			poolIdToReturn = poolId
		}
	}

	return poolIdToReturn, nil
}

// getIPv4IPAMFromAnnotation gets the ipv4 ipam value from the ingress annotation
func (t *defaultModelBuildTask) getIPv4IPAMFromAnnotation(ing ClassifiedIngress) *string {
	var ipamPool string
	if exist := t.annotationParser.ParseStringAnnotation(annotations.IngressSuffixIPAMIPv4PoolId, &ipamPool, ing.Ing.Annotations); exist {
		if len(ipamPool) > 0 {
			return &ipamPool
		}
	}
	return nil
}

// buildIngressClassIPv4IPAM builds the ipv4 ipam pool id for an IngressClass.
func (t *defaultModelBuildTask) getIPv4IPAMFromIngressClass(ingClassConfig ClassConfiguration) *string {
	if ingClassConfig.IngClassParams == nil || ingClassConfig.IngClassParams.Spec.IPAMConfiguration == nil || ingClassConfig.IngClassParams.Spec.IPAMConfiguration.IPv4IPAMPoolId == nil {
		return nil
	}
	return ingClassConfig.IngClassParams.Spec.IPAMConfiguration.IPv4IPAMPoolId
}
