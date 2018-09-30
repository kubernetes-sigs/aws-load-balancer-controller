package tg

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
)

// Targets contains the targets for a target group.
type Targets struct {
	// TgArn is the ARN of the target group
	TgArn string

	// Targets are the targets for the target group
	Targets []*elbv2.TargetDescription

	// TargetType is the type of targets, either ip or instance
	TargetType string

	// Ingress is the ingress for the targets
	Ingress *extensions.Ingress

	// Backend is the ingress backend for the targets
	Backend *extensions.IngressBackend
}

// NewTargets returns a new Targets poitner
func NewTargets(targetType string, ingress *extensions.Ingress, backend *extensions.IngressBackend) *Targets {
	return &Targets{
		TargetType: targetType,
		Ingress:    ingress,
		Backend:    backend,
	}
}

// TargetsController provides functionality to manage targets
type TargetsController interface {
	// Reconcile ensures the target group targets in AWS matches the targets configured in the ingress backend.
	Reconcile(context.Context, *Targets) error
}

// NewTargetsController constructs a new attributes controller
func NewTargetsController(store store.Storer, elbv2 elbv2iface.ELBV2API) TargetsController {
	return &targetsController{
		store: store,
		elbv2: elbv2,
	}
}

type targetsController struct {
	store store.Storer
	elbv2 elbv2iface.ELBV2API
}

func (c *targetsController) Reconcile(ctx context.Context, t *Targets) error {
	var current []*elbv2.TargetDescription

	opts := &elbv2.DescribeTargetHealthInput{TargetGroupArn: aws.String(t.TgArn)}
	if r, err := c.elbv2.DescribeTargetHealth(opts); err == nil {
		for _, thd := range r.TargetHealthDescriptions {
			current = append(current, thd.Target)
		}
	} else {
		return err
	}

	endpointResolver := backend.NewEndpointResolver(c.store, t.TargetType)
	desired, err := endpointResolver.Resolve(t.Ingress, t.Backend)
	if err != nil {
		return err
	}

	additions, removals := targetChangeSets(current, desired)

	if len(additions) > 0 {
		albctx.GetLogger(ctx).Infof("Adding targets to %v: %v", t.TgArn, tdsString(additions))
		in := &elbv2.RegisterTargetsInput{
			TargetGroupArn: aws.String(t.TgArn),
			Targets:        additions,
		}

		if _, err := c.elbv2.RegisterTargets(in); err != nil {
			albctx.GetLogger(ctx).Errorf("Error adding targets to %v: %v", t.TgArn, err.Error())
			if eventf, ok := albctx.GetEventf(ctx); ok {
				eventf(api.EventTypeWarning, "ERROR", "Error adding targets to target group %s: %s", t.TgArn, err.Error())
			}
			return err
		}
	}

	if len(removals) > 0 {
		albctx.GetLogger(ctx).Infof("Removing targets from %v: %v", t.TgArn, tdsString(removals))
		in := &elbv2.DeregisterTargetsInput{
			TargetGroupArn: aws.String(t.TgArn),
			Targets:        removals,
		}

		if _, err := c.elbv2.DeregisterTargets(in); err != nil {
			albctx.GetLogger(ctx).Errorf("Error removing targets from %v: %v", t.TgArn, err.Error())
			if eventf, ok := albctx.GetEventf(ctx); ok {
				eventf(api.EventTypeWarning, "ERROR", "Error removing targets from target group %s: %s", t.TgArn, err.Error())
			}
			return err
		}
	}
	return nil
}

// targetChangeSets compares b to a, returning a list of targets to add and remove from a to match b
func targetChangeSets(a, b []*elbv2.TargetDescription) (add []*elbv2.TargetDescription, remove []*elbv2.TargetDescription) {
	aMap := map[string]bool{}
	bMap := map[string]bool{}

	for i := range a {
		aMap[a[i].String()] = true
	}
	for i := range b {
		bMap[b[i].String()] = true
	}

	for i := range b {
		if _, ok := aMap[b[i].String()]; !ok {
			add = append(add, b[i])
		}
	}

	for i := range a {
		if _, ok := bMap[a[i].String()]; !ok {
			remove = append(remove, a[i])
		}
	}

	return add, remove
}

func tdsString(tds []*elbv2.TargetDescription) string {
	var s []string
	for i := range tds {
		n := aws.StringValue(tds[i].Id)
		if tds[i].Port != nil {
			n = n + fmt.Sprintf(":%v", aws.Int64Value(tds[i].Port))
		}
		s = append(s, n)
	}
	return strings.Join(s, ", ")
}
