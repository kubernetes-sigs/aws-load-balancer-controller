package deploy

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
)

func (a *loadBalancerActuator) reconcileLBInstanceAttributes(ctx context.Context, lbArn string, lbAttributes api.LoadBalancerAttributes) error {
	resp, err := a.cloud.ELBV2().DescribeLoadBalancerAttributesWithContext(ctx, &elbv2.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	if err != nil {
		return err
	}

	actualAttrs, _, err := build.ParseLoadBalancerAttributes(resp.Attributes)
	if err != nil {
		return errors.Wrapf(err, "failed to parse loadBalancer attributes for %v", lbArn)
	}

	changeSet := computeLBAttributesChangeSet(actualAttrs, lbAttributes)
	if len(changeSet) > 0 {
		logging.FromContext(ctx).Info("modifying loadBalancer attribute", "arn", lbArn, "changeSet", awsutil.Prettify(changeSet))
		if _, err = a.cloud.ELBV2().ModifyLoadBalancerAttributesWithContext(ctx, &elbv2.ModifyLoadBalancerAttributesInput{
			LoadBalancerArn: aws.String(lbArn),
			Attributes:      changeSet,
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("modified loadBalancer attribute", "arn", lbArn)
	}
	return nil
}

func computeLBAttributesChangeSet(actual api.LoadBalancerAttributes, desired api.LoadBalancerAttributes) []*elbv2.LoadBalancerAttribute {
	var changeSet []*elbv2.LoadBalancerAttribute
	if actual.DeletionProtection.Enabled != desired.DeletionProtection.Enabled {
		changeSet = append(changeSet,
			buildLBAttribute(build.DeletionProtectionEnabledKey, fmt.Sprintf("%v", desired.DeletionProtection.Enabled)))
	}

	if actual.AccessLogs.S3.Enabled != desired.AccessLogs.S3.Enabled {
		changeSet = append(changeSet,
			buildLBAttribute(build.AccessLogsS3EnabledKey, fmt.Sprintf("%v", desired.AccessLogs.S3.Enabled)))
	}
	// ELBV2 API forbids us to set bucket to an empty bucket, so we keep it unchanged if AccessLogsS3Enabled==false.
	if desired.AccessLogs.S3.Enabled {
		if actual.AccessLogs.S3.Bucket != desired.AccessLogs.S3.Bucket {
			changeSet = append(changeSet,
				buildLBAttribute(build.AccessLogsS3BucketKey, fmt.Sprintf("%v", desired.AccessLogs.S3.Bucket)))
		}

		if actual.AccessLogs.S3.Prefix != desired.AccessLogs.S3.Prefix {
			changeSet = append(changeSet,
				buildLBAttribute(build.AccessLogsS3PrefixKey, fmt.Sprintf("%v", desired.AccessLogs.S3.Prefix)))
		}
	}

	if actual.IdleTimeout.TimeoutSeconds != desired.IdleTimeout.TimeoutSeconds {
		changeSet = append(changeSet,
			buildLBAttribute(build.IdleTimeoutTimeoutSecondsKey, fmt.Sprintf("%v", desired.IdleTimeout.TimeoutSeconds)))
	}

	if actual.Routing.HTTP2.Enabled != desired.Routing.HTTP2.Enabled {
		changeSet = append(changeSet,
			buildLBAttribute(build.RoutingHTTP2EnabledKey, fmt.Sprintf("%v", desired.Routing.HTTP2.Enabled)))
	}

	return changeSet
}

func buildLBAttribute(key, value string) *elbv2.LoadBalancerAttribute {
	return &elbv2.LoadBalancerAttribute{Key: aws.String(key), Value: aws.String(value)}
}
