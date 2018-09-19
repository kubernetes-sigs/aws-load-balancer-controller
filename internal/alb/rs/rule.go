package rs

import (
	"fmt"
	"strconv"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

type NewDesiredRuleOptions struct {
	Ingress          *extensions.Ingress
	Store            store.Storer
	Priority         int
	Hostname         string
	IgnoreHostHeader *bool
	Path             string
	SvcName          string
	SvcPort          intstr.IntOrString
	Logger           *log.Logger
}

// NewDesiredRule returns an rule.Rule based on the provided parameters.
func NewDesiredRule(o *NewDesiredRuleOptions) (*Rule, error) {
	r := &elbv2.Rule{
		IsDefault: aws.Bool(false),
		Priority:  aws.String(fmt.Sprintf("%v", o.Priority)),
		Actions: []*elbv2.Action{
			{
				TargetGroupArn: nil, // Populated at creation, since we create rules before we create rules
				Type:           aws.String(elbv2.ActionTypeEnumForward),
			},
		},
	}

	// Requested an `use-annotation` type rule
	if o.Ingress != nil && action.Use(o.SvcPort.String()) {
		annos, err := o.Store.GetIngressAnnotations(k8s.MetaNamespaceKey(o.Ingress))
		if err != nil {
			return nil, err
		}

		actionConfig, err := annos.Action.GetAction(o.SvcName)
		if err != nil {
			return nil, err
		}

		r.Actions[0] = actionConfig
	}

	if !*r.IsDefault {
		if o.Hostname != "" && ((o.IgnoreHostHeader != nil && !*o.IgnoreHostHeader) || o.IgnoreHostHeader == nil) {
			r.Conditions = append(r.Conditions, &elbv2.RuleCondition{
				Field:  aws.String("host-header"),
				Values: []*string{aws.String(o.Hostname)},
			})
		}

		if o.Path != "" {
			r.Conditions = append(r.Conditions, &elbv2.RuleCondition{
				Field:  aws.String("path-pattern"),
				Values: []*string{aws.String(o.Path)},
			})
		}
	}

	return &Rule{
		svc:    svc{desired: service{name: o.SvcName, port: o.SvcPort}},
		rs:     rs{desired: r},
		logger: o.Logger,
	}, nil
}

type NewCurrentRuleOptions struct {
	SvcName string
	SvcPort intstr.IntOrString
	Rule    *elbv2.Rule
	Logger  *log.Logger
}

// NewCurrentRule creates a Rule from an elbv2.Rule
func NewCurrentRule(o *NewCurrentRuleOptions) *Rule {
	return &Rule{
		svc:    svc{current: service{name: o.SvcName, port: o.SvcPort}},
		rs:     rs{current: o.Rule},
		logger: o.Logger,
	}
}

// Reconcile compares the current and desired state of this Rule instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS Rule to
// satisfy the ingress's current state.
func (r *Rule) Reconcile(rOpts *ReconcileOptions) error {
	// If there is a desired rule, set some of the ARNs which are not available when we assemble the desired state
	if r.rs.desired != nil {
		for i := range r.rs.desired.Actions {
			if *r.rs.desired.Actions[i].Type != elbv2.ActionTypeEnumForward {
				continue
			}
			r.rs.desired.Actions[i].TargetGroupArn = r.TargetGroupArn(rOpts.TargetGroups)
		}
	}

	switch {
	case r.rs.desired == nil: // rule should be deleted
		if r.rs.current == nil {
			break
		}
		if *r.rs.current.IsDefault == true {
			break
		}
		r.logger.Infof("Start Rule deletion.")
		if err := r.delete(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%s rule deleted", *r.rs.current.Priority)
		r.logger.Infof("Completed Rule deletion. Rule Priority: %s | Condition: %s",
			log.Prettify(r.rs.current.Priority),
			log.Prettify(r.rs.current.Conditions))

	case *r.rs.desired.IsDefault:
		// rule is default (attached to listener), do nothing
		// Seems to happen automatically, if we try to change it we get an error:
		// OperationNotPermitted: Default rule '<arn>' cannot be modified
		r.rs.current = r.rs.desired

	case r.rs.current == nil: // rule doesn't exist and should be created
		r.logger.Infof("Start Rule creation.")
		if err := r.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%s rule created", *r.rs.current.Priority)
		r.logger.Infof("Completed Rule creation. Rule Priority: %s | Condition: %s",
			log.Prettify(r.rs.current.Priority),
			log.Prettify(r.rs.current.Conditions))

	case r.needsModification(): // diff between current and desired, modify rule
		if err := r.modify(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s rule modified", *r.rs.current.Priority)
	}

	return nil
}

func (r *Rule) TargetGroupArn(tgs tg.TargetGroups) *string {
	i := tgs.LookupByBackend(extensions.IngressBackend{ServiceName: r.svc.desired.name, ServicePort: r.svc.desired.port})
	if i < 0 {
		r.logger.Errorf("Failed to locate TargetGroup related to this service: %s:%s", r.svc.desired.name, r.svc.desired.port.String())
		return nil
	}
	arn := tgs[i].CurrentARN()
	if arn == nil {
		r.logger.Errorf("Located TargetGroup but no known (current) state found: %s:%s", r.svc.desired.name, r.svc.desired.port.String())
	}
	return arn
}

func (r *Rule) create(rOpts *ReconcileOptions) error {
	in := &elbv2.CreateRuleInput{
		Actions:     r.rs.desired.Actions,
		Conditions:  r.rs.desired.Conditions,
		ListenerArn: rOpts.ListenerArn,
		Priority:    priority(r.rs.desired.Priority),
	}

	o, err := albelbv2.ELBV2svc.CreateRule(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %v rule: %s", *in.Priority, err.Error())
		return fmt.Errorf("Failed Rule creation. Rule: %s | Error: %s", log.Prettify(r.rs.desired), err.Error())
	}
	r.rs.current = o.Rules[0]
	r.svc.current = r.svc.desired

	return nil
}

func (r *Rule) modify(rOpts *ReconcileOptions) error {
	in := &elbv2.ModifyRuleInput{
		Actions:    r.rs.desired.Actions,
		Conditions: r.rs.desired.Conditions,
		RuleArn:    r.rs.current.RuleArn,
	}

	o, err := albelbv2.ELBV2svc.ModifyRule(in)
	if err != nil {
		msg := fmt.Sprintf("Error modifying rule %s: %s", *r.rs.current.RuleArn, err.Error())
		rOpts.Eventf(api.EventTypeWarning, "ERROR", msg)
		return fmt.Errorf(msg)
	}
	if len(o.Rules) > 0 {
		r.rs.current = o.Rules[0]
	}
	r.svc.current = r.svc.desired

	return nil
}

func (r *Rule) delete(rOpts *ReconcileOptions) error {
	if r.rs.current == nil {
		r.logger.Infof("Rule entered delete with no Current to delete. Rule: %s",
			log.Prettify(r))
		return nil
	}

	// If the current rule was a default, it's bound to the listener and won't be deleted from here.
	if *r.rs.current.IsDefault {
		r.logger.Debugf("Deletion hit for default rule, which is bound to the Listener. It will not be deleted from here. Rule. Rule: %s",
			log.Prettify(r))
		return nil
	}

	in := &elbv2.DeleteRuleInput{RuleArn: r.rs.current.RuleArn}
	if _, err := albelbv2.ELBV2svc.DeleteRule(in); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %s rule: %s", *r.rs.current.Priority, err.Error())
		return fmt.Errorf("Failed Rule deletion. Error: %s", err.Error())
	}

	r.deleted = true
	return nil
}

func (r *Rule) needsModification() bool {
	crs := r.rs.current
	drs := r.rs.desired

	switch {
	case crs == nil:
		r.logger.Debugf("Current is nil")
		return true
	case !util.DeepEqual(crs.Actions, drs.Actions):
		r.logger.Debugf("Actions needs to be changed (%v != %v)", log.Prettify(crs.Actions), log.Prettify(drs.Actions))
		return true
	case !conditionsEqual(crs.Conditions, drs.Conditions):
		r.logger.Debugf("Conditions needs to be changed (%v != %v)", log.Prettify(crs.Conditions), log.Prettify(drs.Conditions))
		return true
	case r.svc.current.name != r.svc.desired.name && r.svc.current.port.String() != action.UseActionAnnotation:
		r.logger.Debugf("SvcName needs to be changed (%v != %v)", r.svc.current.name, r.svc.desired.name)
		return true
	case r.svc.current.port.String() != r.svc.desired.port.String():
		r.logger.Debugf("SvcPort needs to be changed (%v != %v)", r.svc.current.port.String(), r.svc.desired.port.String())
		return true
	}

	return false
}

// conditionsEqual returns true if c1 and c2 are identical conditions.
func conditionsEqual(c1 []*elbv2.RuleCondition, c2 []*elbv2.RuleCondition) bool {
	cMap1 := conditionToMap(c1)
	cMap2 := conditionToMap(c2)

	for k, v := range cMap1 {
		val, ok := cMap2[k]
		// If key didn't exist, mod is needed
		if !ok {
			return false
		}
		// If key existed but values were diff, mod is needed
		if !util.DeepEqual(v, val) {
			return false
		}
	}

	return true
}

// conditionsToMap converts a elbv2.Conditions struct into a map[string]string representation
func conditionToMap(cs []*elbv2.RuleCondition) map[string][]*string {
	cMap := make(map[string][]*string)
	for _, c := range cs {
		cMap[*c.Field] = c.Values
	}
	return cMap
}

// stripDesiredState removes the desired state from the rule.
func (r *Rule) stripDesiredState() {
	r.rs.desired = nil
}

// stripCurrentState removes the current state from the rule.
func (r *Rule) stripCurrentState() {
	r.rs.current = nil
}

func priority(s *string) *int64 {
	if *s == "default" {
		return aws.Int64(0)
	}
	i, _ := strconv.ParseInt(*s, 10, 64)
	return aws.Int64(i)
}

// IsDesiredDefault returns true if the desired rule is the default rule
func (r Rule) IsDesiredDefault() bool {
	if r.rs.desired == nil {
		return false
	}
	return *r.rs.desired.IsDefault
}

func (r Rule) valid(listenerPort int64, listenerProtocol *string) bool {
	if r.rs.desired.Actions[0].RedirectConfig != nil {
		var host, path *string
		rc := r.rs.desired.Actions[0].RedirectConfig

		for _, c := range r.rs.desired.Conditions {
			if *c.Field == "host-header" {
				host = c.Values[0]
			}
			if *c.Field == "path-pattern" {
				path = c.Values[0]
			}
		}

		if host == nil && *rc.Host != "#{host}" {
			return true
		}
		if host != nil && *rc.Host != *host && *rc.Host != "#{host}" {
			return true
		}
		if path == nil && *rc.Path != "/#{path}" {
			return true
		}
		if path != nil && *rc.Path != *path && *rc.Path != "/#{path}" {
			return true
		}
		if *rc.Port != "#{port}" && *rc.Port != fmt.Sprintf("%v", listenerPort) {
			return true
		}
		if *rc.Query != "#{query}" {
			return true
		}
		if listenerProtocol != nil && *rc.Protocol != "#{protocol}" && *rc.Protocol != *listenerProtocol {
			return true
		}
		return false
	}
	return true
}
