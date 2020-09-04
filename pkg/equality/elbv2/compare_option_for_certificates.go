package elbv2

import (
	"github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func CompareOptionForCertificate() cmp.Option {
	return cmpopts.IgnoreFields(elbv2sdk.Certificate{}, "IsDefault")
}

func CompareOptionForCertificates() cmp.Options {
	return cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(lhs *elbv2sdk.Certificate, rhs *elbv2sdk.Certificate) bool {
			return aws.StringValue(lhs.CertificateArn) < aws.StringValue(rhs.CertificateArn)
		}),
		CompareOptionForCertificate(),
	}
}
