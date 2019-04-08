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

func (a *targetGroupActuator) reconcileTGInstanceAttributes(ctx context.Context, tgArn string, tgAttributes api.TargetGroupAttributes) error {
	resp, err := a.cloud.ELBV2().DescribeTargetGroupAttributesWithContext(ctx, &elbv2.DescribeTargetGroupAttributesInput{
		TargetGroupArn: aws.String(tgArn),
	})
	if err != nil {
		return err
	}
	actualAttrs, _, err := build.ParseTargetGroupAttributes(resp.Attributes)
	if err != nil {
		return errors.Wrapf(err, "failed to parse targetGroup attributes for %v", tgArn)
	}
	changeSet := computeTGAttributesChangeSet(actualAttrs, tgAttributes)

	if len(changeSet) > 0 {
		logging.FromContext(ctx).Info("modifying targetGroup attribute", "arn", tgArn, "changeSet", awsutil.Prettify(changeSet))
		if _, err = a.cloud.ELBV2().ModifyTargetGroupAttributesWithContext(ctx, &elbv2.ModifyTargetGroupAttributesInput{
			TargetGroupArn: aws.String(tgArn),
			Attributes:     changeSet,
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("modified targetGroup attribute", "arn", tgArn)
	}
	return nil
}

func computeTGAttributesChangeSet(actual api.TargetGroupAttributes, desired api.TargetGroupAttributes) []*elbv2.TargetGroupAttribute {
	var changeSet []*elbv2.TargetGroupAttribute
	if actual.DeregistrationDelay.TimeoutSeconds != desired.DeregistrationDelay.TimeoutSeconds {
		changeSet = append(changeSet,
			buildTGAttribute(build.DeregistrationDelayTimeoutSecondsKey, fmt.Sprintf("%v", desired.DeregistrationDelay.TimeoutSeconds)))
	}
	if actual.SlowStart.DurationSeconds != desired.SlowStart.DurationSeconds {
		changeSet = append(changeSet,
			buildTGAttribute(build.SlowStartDurationSecondsKey, fmt.Sprintf("%v", desired.SlowStart.DurationSeconds)))
	}
	if actual.Stickiness.Enabled != desired.Stickiness.Enabled {
		changeSet = append(changeSet,
			buildTGAttribute(build.StickinessEnabledKey, fmt.Sprintf("%v", desired.Stickiness.Enabled)))
	}
	if actual.Stickiness.Type != desired.Stickiness.Type {
		changeSet = append(changeSet,
			buildTGAttribute(build.StickinessTypeKey, desired.Stickiness.Type.String()))
	}
	if actual.Stickiness.LBCookie.DurationSeconds != desired.Stickiness.LBCookie.DurationSeconds {
		changeSet = append(changeSet,
			buildTGAttribute(build.StickinessLbCookieDurationSecondsKey, fmt.Sprintf("%v", desired.Stickiness.LBCookie.DurationSeconds)))
	}

	return changeSet
}

func buildTGAttribute(key, value string) *elbv2.TargetGroupAttribute {
	return &elbv2.TargetGroupAttribute{Key: aws.String(key), Value: aws.String(value)}
}
