package elbv2

import (
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func CompareOptionsForMTLS() cmp.Options {
	return cmp.Options{
		cmpopts.IgnoreUnexported(elbv2types.MutualAuthenticationAttributes{}),
		cmpopts.IgnoreUnexported(elbv2.MutualAuthenticationAttributes{}),
		cmpopts.EquateComparable(elbv2types.Certificate{}),
		cmpopts.IgnoreFields(elbv2types.MutualAuthenticationAttributes{}, "TrustStoreAssociationStatus"),
	}
}
