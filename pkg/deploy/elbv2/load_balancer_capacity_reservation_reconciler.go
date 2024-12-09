package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"reflect"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

// reconciler for LoadBalancer Capacity Reservation
type LoadBalancerCapacityReservationReconciler interface {
	// Reconcile loadBalancer capacity reservation
	Reconcile(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error
}

// NewDefaultLoadBalancerCapacityReservationReconciler constructs new defaultLoadBalancerCapacityReservationReconciler.
func NewDefaultLoadBalancerCapacityReservationReconciler(elbv2Client services.ELBV2, featureGates config.FeatureGates, logger logr.Logger) *defaultLoadBalancerCapacityReservationReconciler {
	return &defaultLoadBalancerCapacityReservationReconciler{
		elbv2Client:  elbv2Client,
		logger:       logger,
		featureGates: featureGates,
	}
}

var _ LoadBalancerCapacityReservationReconciler = &defaultLoadBalancerCapacityReservationReconciler{}

// default implementation for LoadBalancerCapacityReservationReconciler
type defaultLoadBalancerCapacityReservationReconciler struct {
	elbv2Client  services.ELBV2
	logger       logr.Logger
	featureGates config.FeatureGates
}

func (r *defaultLoadBalancerCapacityReservationReconciler) Reconcile(ctx context.Context, resLB *elbv2model.LoadBalancer, sdkLB LoadBalancerWithTags) error {
	desiredCapacityReservation := resLB.Spec.MinimumLoadBalancerCapacity
	// If the annotation is missing or not set, skip the capacity reservation
	if desiredCapacityReservation == nil {
		return nil
	}
	//If the value of desired capacityUnits is zero then set desiredCapacityReservation to nil to reset the capacity
	if desiredCapacityReservation.CapacityUnits == 0 {
		desiredCapacityReservation = nil
	}
	currentCapacityReservation, err := r.getCurrentCapacityReservation(ctx, sdkLB)
	if err != nil {
		return err
	}
	isLBCapacityReservationDrifted := !reflect.DeepEqual(desiredCapacityReservation, currentCapacityReservation)
	if !isLBCapacityReservationDrifted {
		return nil
	}
	if desiredCapacityReservation == nil {
		//If the value of desired capacityUnits is nil then reset capacity
		req := &elbv2sdk.ModifyCapacityReservationInput{
			LoadBalancerArn:          sdkLB.LoadBalancer.LoadBalancerArn,
			ResetCapacityReservation: awssdk.Bool(true),
		}

		r.logger.Info("resetting loadBalancer capacity reservation",
			"stackID", resLB.Stack().StackID(),
			"resourceID", resLB.ID(),
			"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn))
		if _, err := r.elbv2Client.ModifyCapacityReservationWithContext(ctx, req); err != nil {
			return err
		}
		r.logger.Info("reset successful for loadBalancer capacity reservation",
			"stackID", resLB.Stack().StackID(),
			"resourceID", resLB.ID(),
			"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn))
	} else {
		req := &elbv2sdk.ModifyCapacityReservationInput{
			LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
			MinimumLoadBalancerCapacity: &elbv2types.MinimumLoadBalancerCapacity{
				CapacityUnits: awssdk.Int32(desiredCapacityReservation.CapacityUnits),
			},
		}
		r.logger.Info("modifying loadBalancer capacity reservation",
			"stackID", resLB.Stack().StackID(),
			"resourceID", resLB.ID(),
			"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn),
			"change", desiredCapacityReservation)
		if _, err := r.elbv2Client.ModifyCapacityReservationWithContext(ctx, req); err != nil {
			return err
		}
		r.logger.Info("modified loadBalancer capacity reservation",
			"stackID", resLB.Stack().StackID(),
			"resourceID", resLB.ID(),
			"arn", awssdk.ToString(sdkLB.LoadBalancer.LoadBalancerArn))
	}
	return nil
}

func (r *defaultLoadBalancerCapacityReservationReconciler) getCurrentCapacityReservation(ctx context.Context, sdkLB LoadBalancerWithTags) (*elbv2model.MinimumLoadBalancerCapacity, error) {
	req := &elbv2sdk.DescribeCapacityReservationInput{
		LoadBalancerArn: sdkLB.LoadBalancer.LoadBalancerArn,
	}
	resp, err := r.elbv2Client.DescribeCapacityReservationWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	var sdkLBMinimumCapacity = &elbv2model.MinimumLoadBalancerCapacity{}
	if (resp.CapacityReservationState == nil || len(resp.CapacityReservationState) == 0) && resp.MinimumLoadBalancerCapacity == nil {
		return nil, nil
	}
	sdkLBMinimumCapacity.CapacityUnits = awssdk.ToInt32(resp.MinimumLoadBalancerCapacity.CapacityUnits)
	return sdkLBMinimumCapacity, nil
}
