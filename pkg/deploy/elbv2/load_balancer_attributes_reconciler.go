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

// reconciler for LoadBalancer attributes
type LoadBalancerAttributeReconciler interface {
	// Reconcile loadBalancer attributes
	Reconcile(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error
}

// NewDefaultLoadBalancerAttributeReconciler constructs new defaultLoadBalancerAttributeReconciler.
func NewDefaultLoadBalancerAttributeReconciler(elbv2Client services.ELBV2, logger logr.Logger) *defaultLoadBalancerAttributeReconciler {
	return &defaultLoadBalancerAttributeReconciler{
		elbv2Client: elbv2Client,
		logger:      logger,
	}
}

var _ LoadBalancerAttributeReconciler = &defaultLoadBalancerAttributeReconciler{}

// default implementation for LoadBalancerAttributeReconciler
type defaultLoadBalancerAttributeReconciler struct {
	elbv2Client services.ELBV2
	logger      logr.Logger
}

func (r *defaultLoadBalancerAttributeReconciler) Reconcile(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	desiredAttrs := r.getDesiredLoadBalancerAttributes(ctx, resLB)
	currentAttrs, err := r.getCurrentLoadBalancerAttributes(ctx, sdkLB)
	if err != nil {
		return err
	}

	attributesToUpdate, _ := algorithm.DiffStringMap(desiredAttrs, currentAttrs)
	if len(attributesToUpdate) > 0 {
		req := &elbv2sdk.ModifyLoadBalancerAttributesInput{
			LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
			Attributes:      nil,
		}
		for _, attrKey := range sets.StringKeySet(attributesToUpdate).List() {
			req.Attributes = append(req.Attributes, &elbv2sdk.LoadBalancerAttribute{
				Key:   awssdk.String(attrKey),
				Value: awssdk.String(attributesToUpdate[attrKey]),
			})
		}

		r.logger.Info("modifying loadBalancer attributes",
			"stackID", resLB.Stack().StackID(),
			"resourceID", resLB.ID(),
			"arn", awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn),
			"change", attributesToUpdate)
		if _, err := r.elbv2Client.ModifyLoadBalancerAttributesWithContext(ctx, req); err != nil {
			return err
		}
		r.logger.Info("modified loadBalancer attributes",
			"stackID", resLB.Stack().StackID(),
			"resourceID", resLB.ID(),
			"arn", awssdk.StringValue(sdkLB.LoadBalancer.LoadBalancerArn))
	}
	return nil
}

func (r *defaultLoadBalancerAttributeReconciler) getDesiredLoadBalancerAttributes(ctx context.Context, resLB *elbv2model.LoadBalancer) map[string]string {
	lbAttributes := make(map[string]string, len(resLB.Spec.LoadBalancerAttributes))
	for _, attr := range resLB.Spec.LoadBalancerAttributes {
		lbAttributes[attr.Key] = attr.Value
	}
	return lbAttributes
}

func (r *defaultLoadBalancerAttributeReconciler) getCurrentLoadBalancerAttributes(ctx context.Context, sdkLB LoadBalancerWithTags) (map[string]string, error) {
	req := &elbv2sdk.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
	}
	resp, err := r.elbv2Client.DescribeLoadBalancerAttributesWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	lbAttributes := make(map[string]string, len(resp.Attributes))
	for _, attr := range resp.Attributes {
		lbAttributes[awssdk.StringValue(attr.Key)] = awssdk.StringValue(attr.Value)
	}
	return lbAttributes, nil
}
