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

// NewTargets returns a new Targets pointer
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

// NewTargetsController constructs a new target group targets controller
func NewTargetsController(elbv2svc elbv2iface.ELBV2API, endpointResolver backend.EndpointResolver) TargetsController {
	return &targetsController{
		elbv2:            elbv2svc,
		endpointResolver: endpointResolver,
	}
}

type targetsController struct {
	elbv2            elbv2iface.ELBV2API
	endpointResolver backend.EndpointResolver
}

func (c *targetsController) Reconcile(ctx context.Context, t *Targets) error {
	desired, err := c.endpointResolver.Resolve(t.Ingress, t.Backend, t.TargetType)
	if err != nil {
		return err
	}
	current, err := c.getCurrentTargets(t.TgArn)
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
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error adding targets to target group %s: %s", t.TgArn, err.Error())
			return err
		}
		// TODO add Add events ?
	}

	if len(removals) > 0 {
		albctx.GetLogger(ctx).Infof("Removing targets from %v: %v", t.TgArn, tdsString(removals))
		in := &elbv2.DeregisterTargetsInput{
			TargetGroupArn: aws.String(t.TgArn),
			Targets:        removals,
		}

		if _, err := c.elbv2.DeregisterTargets(in); err != nil {
			albctx.GetLogger(ctx).Errorf("Error removing targets from %v: %v", t.TgArn, err.Error())
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error removing targets from target group %s: %s", t.TgArn, err.Error())
			return err
		}
		// TODO add Delete events ?
	}

	t.Targets = desired

	return nil
}

func (c *targetsController) getCurrentTargets(TgArn string) ([]*elbv2.TargetDescription, error) {
	opts := &elbv2.DescribeTargetHealthInput{TargetGroupArn: aws.String(TgArn)}
	resp, err := c.elbv2.DescribeTargetHealth(opts)
	if err != nil {
		return nil, err
	}

	var current []*elbv2.TargetDescription
	for _, thd := range resp.TargetHealthDescriptions {
		if aws.StringValue(thd.TargetHealth.State) == elbv2.TargetHealthStateEnumDraining {
			continue
		}
		current = append(current, thd.Target)
	}
	return current, nil
}

// targetChangeSets compares b to a, returning a list of targets to add and remove from a to match b
func targetChangeSets(current, desired []*elbv2.TargetDescription) (add []*elbv2.TargetDescription, remove []*elbv2.TargetDescription) {
	currentMap := map[string]bool{}
	desiredMap := map[string]bool{}

	for _, i := range current {
		currentMap[tdString(i)] = true
	}
	for _, i := range desired {
		desiredMap[tdString(i)] = true
	}

	for _, i := range desired {
		if _, ok := currentMap[tdString(i)]; !ok {
			add = append(add, i)
		}
	}

	for _, i := range current {
		if _, ok := desiredMap[tdString(i)]; !ok {
			remove = append(remove, i)
		}
	}

	return add, remove
}

func tdString(td *elbv2.TargetDescription) string {
	return fmt.Sprintf("%v:%v", aws.StringValue(td.Id), aws.Int64Value(td.Port))
}

func tdsString(tds []*elbv2.TargetDescription) string {
	var s []string
	for _, td := range tds {
		s = append(s, tdString(td))
	}
	return strings.Join(s, ", ")
}
