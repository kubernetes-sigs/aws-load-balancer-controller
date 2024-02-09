package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

// reconciler for TargetGroup attributes
type TargetGroupAttributesReconciler interface {
	// Reconcile TargetGroup attributes
	Reconcile(ctx context.Context, resTG *elbv2model.TargetGroup, sdkTG TargetGroupWithTags) error
}

// NewDefaultTargetGroupAttributesReconciler constructs new TargetGroupAttributesReconciler.
func NewDefaultTargetGroupAttributesReconciler(elbv2Client services.ELBV2, logger logr.Logger) *defaultTargetGroupAttributeReconciler {
	return &defaultTargetGroupAttributeReconciler{
		elbv2Client: elbv2Client,
		logger:      logger,
	}
}

var _ TargetGroupAttributesReconciler = &defaultTargetGroupAttributeReconciler{}

// default implementation for TargetGroupAttributesReconciler
type defaultTargetGroupAttributeReconciler struct {
	elbv2Client services.ELBV2
	logger      logr.Logger
}

func (r *defaultTargetGroupAttributeReconciler) Reconcile(ctx context.Context, resTG *elbv2model.TargetGroup, sdkTG TargetGroupWithTags) error {
	desiredAttrs := r.getDesiredTargetGroupAttributes(ctx, resTG)
	currentAttrs, err := r.getCurrentTargetGroupAttributes(ctx, sdkTG)
	if err != nil {
		return err
	}

	attributesToUpdate, _ := algorithm.DiffStringMap(desiredAttrs, currentAttrs)
	if len(attributesToUpdate) > 0 {
		req := &elbv2sdk.ModifyTargetGroupAttributesInput{
			TargetGroupArn: sdkTG.TargetGroup.TargetGroupArn,
			Attributes:     nil,
		}
		for _, attrKey := range sets.StringKeySet(attributesToUpdate).List() {
			req.Attributes = append(req.Attributes, &elbv2sdk.TargetGroupAttribute{
				Key:   awssdk.String(attrKey),
				Value: awssdk.String(attributesToUpdate[attrKey]),
			})
		}

		r.logger.Info("modifying targetGroup attributes",
			"stackID", resTG.Stack().StackID(),
			"resourceID", resTG.ID(),
			"arn", awssdk.StringValue(sdkTG.TargetGroup.TargetGroupArn),
			"change", attributesToUpdate)
		if _, err := r.elbv2Client.ModifyTargetGroupAttributesWithContext(ctx, req); err != nil {
			return err
		}
		r.logger.Info("modified targetGroup attributes",
			"stackID", resTG.Stack().StackID(),
			"resourceID", resTG.ID(),
			"arn", awssdk.StringValue(sdkTG.TargetGroup.TargetGroupArn))
	}
	return nil
}

func (r *defaultTargetGroupAttributeReconciler) getDesiredTargetGroupAttributes(ctx context.Context, resTG *elbv2model.TargetGroup) map[string]string {
	tgAttributes := make(map[string]string, len(resTG.Spec.TargetGroupAttributes))
	for _, attr := range resTG.Spec.TargetGroupAttributes {
		tgAttributes[attr.Key] = attr.Value
	}
	return tgAttributes
}

func (r *defaultTargetGroupAttributeReconciler) getCurrentTargetGroupAttributes(ctx context.Context, sdkTG TargetGroupWithTags) (map[string]string, error) {
	req := &elbv2sdk.DescribeTargetGroupAttributesInput{
		TargetGroupArn: sdkTG.TargetGroup.TargetGroupArn,
	}

	resp, err := r.elbv2Client.DescribeTargetGroupAttributesWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	tgAttributes := make(map[string]string, len(resp.Attributes))
	for _, attr := range resp.Attributes {
		tgAttributes[awssdk.StringValue(attr.Key)] = awssdk.StringValue(attr.Value)
	}
	return tgAttributes, nil
}
