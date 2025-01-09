package elbv2

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func CompareOptionForCertificate() cmp.Option {
	return cmp.Options{
		cmpopts.IgnoreFields(elbv2types.Certificate{}, "IsDefault"),
		cmpopts.IgnoreUnexported(elbv2types.Certificate{}),
	}
}

func CompareOptionForCertificates() cmp.Options {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(lhs *elbv2types.Certificate, rhs *elbv2types.Certificate) bool {
			return aws.ToString(lhs.CertificateArn) < aws.ToString(rhs.CertificateArn)
		}),
		cmpopts.IgnoreUnexported(elbv2types.Certificate{}),
		CompareOptionForCertificate(),
	}
}
