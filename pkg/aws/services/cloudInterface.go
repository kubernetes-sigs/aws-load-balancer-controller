package services

import "context"

type Cloud interface {
	// EC2 provides API to AWS EC2
	EC2() EC2

	// ELBV2 provides API to AWS ELBV2
	ELBV2() ELBV2

	// ACM provides API to AWS ACM
	ACM() ACM

	// WAFv2 provides API to AWS WAFv2
	WAFv2() WAFv2

	// WAFRegional provides API to AWS WAFRegional
	WAFRegional() WAFRegional

	// Shield provides API to AWS Shield
	Shield() Shield

	// RGT provides API to AWS RGT
	RGT() RGT

	// GlobalAccelerator provides API to AWS GlobalAccelerator
	GlobalAccelerator() GlobalAccelerator

	// Region for the kubernetes cluster
	Region() string

	// VpcID for the LoadBalancer resources.
	VpcID() string

	GetAssumedRoleELBV2(ctx context.Context, assumeRoleArn string, externalId string) (ELBV2, error)
}
