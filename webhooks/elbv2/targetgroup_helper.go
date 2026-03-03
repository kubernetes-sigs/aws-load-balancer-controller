package elbv2

import (
	"context"
	"strings"

	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/pkg/errors"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

// ELBV2ClientProvider returns an ELBV2 client configured for the given region.
// Used by webhooks to describe target groups in a non-default region.
type ELBV2ClientProvider func(region string) (services.ELBV2, error)

// regionFromTGARN extracts the AWS region from a target group ARN.
// ARN format: arn:aws:elasticloadbalancing:<region>:<account>:targetgroup/...
// Returns empty string if the ARN cannot be parsed.
func regionFromTGARN(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) >= 5 {
		return parts[3]
	}
	return ""
}

// resolveELBV2ForTGB returns the ELBV2 client that should be used for the given TGB.
// If the TGB's target group ARN refers to a different region, a regional client is obtained from the provider.
func resolveELBV2ForTGB(defaultClient services.ELBV2, defaultRegion string, provider ELBV2ClientProvider, tgARN string) (services.ELBV2, error) {
	if tgARN == "" || provider == nil {
		return defaultClient, nil
	}
	arnRegion := regionFromTGARN(tgARN)
	if arnRegion == "" || arnRegion == defaultRegion {
		return defaultClient, nil
	}
	return provider(arnRegion)
}

// getTargetGroupFromAWS returns the AWS target group corresponding to the arn
func getTargetGroupFromAWS(ctx context.Context, elbv2Client services.ELBV2, tgb *elbv2api.TargetGroupBinding) (*elbv2types.TargetGroup, error) {
	tgARN := tgb.Spec.TargetGroupARN
	req := &elbv2sdk.DescribeTargetGroupsInput{
		TargetGroupArns: []string{tgARN},
	}
	return getTargetGroupHelper(ctx, elbv2Client, tgb, tgARN, req)
}

// getTargetGroupsByNameFromAWS returns the AWS target group corresponding to the name
func getTargetGroupsByNameFromAWS(ctx context.Context, elbv2Client services.ELBV2, tgb *elbv2api.TargetGroupBinding) (*elbv2types.TargetGroup, error) {
	req := &elbv2sdk.DescribeTargetGroupsInput{
		Names: []string{tgb.Spec.TargetGroupName},
	}

	return getTargetGroupHelper(ctx, elbv2Client, tgb, tgb.Spec.TargetGroupName, req)
}

func getTargetGroupHelper(ctx context.Context, elbv2Client services.ELBV2, tgb *elbv2api.TargetGroupBinding, tgIdentifier string, req *elbv2sdk.DescribeTargetGroupsInput) (*elbv2types.TargetGroup, error) {
	clientToUse, err := elbv2Client.AssumeRole(ctx, tgb.Spec.IamRoleArnToAssume, tgb.Spec.AssumeRoleExternalId)

	if err != nil {
		return nil, err
	}

	tgList, err := clientToUse.DescribeTargetGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(tgList) != 1 {
		return nil, errors.Errorf("expecting a single targetGroup with query [%s] but got %v", tgIdentifier, len(tgList))
	}
	return &tgList[0], nil
}
