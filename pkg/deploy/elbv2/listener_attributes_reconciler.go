package elbv2

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

// Reconciler for Listener attributes
type ListenerAttributesReconciler interface {
	Reconcile(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) error
}

// NewListenerAttributesReconciler constructs new ListenerAttributesReconciler.
func NewDefaultListenerAttributesReconciler(elbv2Client services.ELBV2, logger logr.Logger) *defaultListenerAttributesReconciler {
	return &defaultListenerAttributesReconciler{
		elbv2Client: elbv2Client,
		logger:      logger,
	}
}

var _ ListenerAttributesReconciler = &defaultListenerAttributesReconciler{}

// default implementation for ListenerAttributeReconciler
type defaultListenerAttributesReconciler struct {
	elbv2Client services.ELBV2
	logger      logr.Logger
}

func (r *defaultListenerAttributesReconciler) Reconcile(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) error {
	desiredAttrs := r.getDesiredListenerAttributes(ctx, resLS)
	currentAttrs, err := r.getCurrentListenerAttributes(ctx, sdkLS)
	if err != nil {
		return err
	}
	attributesToUpdate, _ := algorithm.DiffStringMap(desiredAttrs, currentAttrs)
	if len(attributesToUpdate) > 0 {
		req := &elbv2sdk.ModifyListenerAttributesInput{
			ListenerArn: sdkLS.Listener.ListenerArn,
			Attributes:  nil,
		}
		for _, attrKey := range sets.StringKeySet(attributesToUpdate).List() {
			req.Attributes = append(req.Attributes, elbv2types.ListenerAttribute{
				Key:   awssdk.String(attrKey),
				Value: awssdk.String(attributesToUpdate[attrKey]),
			})
		}
		r.logger.Info("modifying listener attributes",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.ToString(sdkLS.Listener.ListenerArn),
			"change", attributesToUpdate)
		if _, err := r.elbv2Client.ModifyListenerAttributesWithContext(ctx, req); err != nil {
			return err
		}
		r.logger.Info("modified listener attribute",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.ToString(sdkLS.Listener.ListenerArn))

	}
	return nil

}

func (r *defaultListenerAttributesReconciler) getDesiredListenerAttributes(ctx context.Context, resLS *elbv2model.Listener) map[string]string {
	lsAttributes := make(map[string]string, len(resLS.Spec.ListenerAttributes))
	for _, attr := range resLS.Spec.ListenerAttributes {
		lsAttributes[attr.Key] = attr.Value
	}
	return lsAttributes
}

func (r *defaultListenerAttributesReconciler) getCurrentListenerAttributes(ctx context.Context, sdkLS ListenerWithTags) (map[string]string, error) {
	req := &elbv2sdk.DescribeListenerAttributesInput{
		ListenerArn: sdkLS.Listener.ListenerArn,
	}
	resp, err := r.elbv2Client.DescribeListenerAttributesWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	lsAttributes := make(map[string]string, len(resp.Attributes))
	for _, attr := range resp.Attributes {
		lsAttributes[awssdk.ToString(attr.Key)] = awssdk.ToString(attr.Value)
	}
	return lsAttributes, nil
}
