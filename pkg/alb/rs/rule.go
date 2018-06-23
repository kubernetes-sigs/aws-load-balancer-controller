package rs

import (
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	api "k8s.io/api/core/v1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

type NewDesiredRuleOptions struct {
	Priority         int
	Hostname         string
	IgnoreHostHeader bool
	Path             string
	SvcName          string
	Logger           *log.Logger
}

// NewDesiredRule returns an rule.Rule based on the provided parameters.
func NewDesiredRule(o *NewDesiredRuleOptions) *Rule {
	r := &elbv2.Rule{
		Actions: []*elbv2.Action{
			{
				TargetGroupArn: nil, // Populated at creation, since we create rules before we create rules
				Type:           aws.String("forward"),
			},
		},
	}

	if o.Priority == 0 {
		r.IsDefault = aws.Bool(true)
		r.Priority = aws.String("default")
	} else {
		r.IsDefault = aws.Bool(false)
		r.Priority = aws.String(fmt.Sprintf("%v", o.Priority))
	}

	if !*r.IsDefault {
		if o.Hostname != "" && !o.IgnoreHostHeader {
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
		svcname: svcname{desired: o.SvcName},
		rs:      rs{desired: r},
		logger:  o.Logger,
	}
}

type NewCurrentRuleOptions struct {
	SvcName string
	Rule    *elbv2.Rule
	Logger  *log.Logger
}

// NewCurrentRule creates a Rule from an elbv2.Rule
func NewCurrentRule(o *NewCurrentRuleOptions) *Rule {
	return &Rule{
		svcname: svcname{current: o.SvcName},
		rs:      rs{current: o.Rule},
		logger:  o.Logger,
	}
}

// Reconcile compares the current and desired state of this Rule instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS Rule to
// satisfy the ingress's current state.
func (r *Rule) Reconcile(rOpts *ReconcileOptions) error {
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

	case *r.rs.desired.IsDefault: // rule is default (attached to listener), do nothing
		r.logger.Debugf("Found desired rule that is a default and is already created with its respective listener. Rule: %s",
			log.Prettify(r.rs.desired))
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
		r.logger.Infof("Start Rule modification.")
		if err := r.modify(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%s rule modified", *r.rs.current.Priority)
		r.logger.Infof("Completed Rule modification. Rule Priority: %s | Condition: %s",
			log.Prettify(r.rs.current.Priority),
			log.Prettify(r.rs.current.Conditions))

	default:
		r.logger.Debugf("No rule modification required.")
	}

	return nil
}

func (r *Rule) TargetGroupArn(tgs tg.TargetGroups) *string {
	i := tgs.LookupBySvc(r.svcname.desired)
	if i < 0 {
		r.logger.Errorf("Failed to locate TargetGroup related to this service: %s", r.svcname.desired)
		return nil
	}
	arn := tgs[i].CurrentARN()
	if arn == nil {
		r.logger.Errorf("Located TargetGroup but no known (current) state found: %s", r.svcname.desired)
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

	in.Actions[0].TargetGroupArn = r.TargetGroupArn(rOpts.TargetGroups)

	o, err := albelbv2.ELBV2svc.CreateRule(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %v rule: %s", *in.Priority, err.Error())
		r.logger.Errorf("Failed Rule creation. Rule: %s | Error: %s",
			log.Prettify(r.rs.desired), err.Error())
		return err
	}
	r.rs.current = o.Rules[0]
	r.svcname.current = r.svcname.desired

	return nil
}

func (r *Rule) modify(rOpts *ReconcileOptions) error {
	in := &elbv2.ModifyRuleInput{
		Actions:    r.rs.desired.Actions,
		Conditions: r.rs.desired.Conditions,
		RuleArn:    r.rs.current.RuleArn,
	}
	in.Actions[0].TargetGroupArn = r.TargetGroupArn(rOpts.TargetGroups)

	o, err := albelbv2.ELBV2svc.ModifyRule(in)
	if err != nil {
		msg := fmt.Sprintf("Error modifying rule %s: %s", *r.rs.current.RuleArn, err.Error())
		rOpts.Eventf(api.EventTypeWarning, "ERROR", msg)
		r.logger.Errorf(msg)
		return err
	}
	if len(o.Rules) > 0 {
		r.rs.current = o.Rules[0]
	}
	r.svcname.current = r.svcname.desired

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
		r.logger.Infof("Failed Rule deletion. Error: %s", err.Error())
		return err
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
	// TODO: We need to sort these because they're causing false positives
	case !conditionsEqual(crs.Conditions, drs.Conditions):
		r.logger.Debugf("Conditions needs to be changed (%v != %v)", log.Prettify(crs.Conditions), log.Prettify(drs.Conditions))
		return true
	case r.svcname.current != r.svcname.desired:
		r.logger.Debugf("SvcName needs to be changed (%v != %v)", r.svcname.current, r.svcname.desired)
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
