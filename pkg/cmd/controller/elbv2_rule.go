package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type Rule struct {
	CurrentRule *elbv2.Rule
	DesiredRule *elbv2.Rule
}

func NewRule(path *string) *Rule {
	r := &elbv2.Rule{
		Actions: []*elbv2.Action{
			&elbv2.Action{
				// TargetGroupArn: targetGroupArn,
				Type: aws.String("forward"),
			},
		},
	}

	if *path == "/" {
		r.IsDefault = aws.Bool(true)
		r.Priority = aws.String("default")
	} else {
		r.IsDefault = aws.Bool(false)
		r.Conditions = []*elbv2.RuleCondition{
			&elbv2.RuleCondition{
				Field:  aws.String("path-pattern"),
				Values: []*string{path},
			},
		}
	}

	rule := &Rule{
		DesiredRule: r,
	}
	return rule
}

func (r *Rule) create(a *ALBIngress, l *Listener, tg *TargetGroup) error {
	glog.Infof("%s: Create %s Rule %s", a.Name(), *tg.id, *r.DesiredRule.Conditions[0].Values[0])

	createRuleInput := &elbv2.CreateRuleInput{
		Actions:     r.DesiredRule.Actions,
		Conditions:  r.DesiredRule.Conditions,
		ListenerArn: l.CurrentListener.ListenerArn,
		Priority:    aws.Int64(1),
	}
	createRuleInput.Actions[0].TargetGroupArn = tg.CurrentTargetGroup.TargetGroupArn

	createRuleOutput, err := elbv2svc.svc.CreateRule(createRuleInput)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateRule"}).Add(float64(1))
		return err
	}

	r.CurrentRule = createRuleOutput.Rules[0]
	return nil
}

func (r *Rule) modify(a *ALBIngress, l *Listener, tg *TargetGroup) error {
	if r.CurrentRule == nil {
		glog.Infof("%s: Rule.modify called with empty CurrentRule, assuming we need to make it", a.Name())
		return r.create(a, l, tg)
	}

	// check/change attributes
	if r.needsModification() {
		glog.Info("OK MODIFY THE ROOL")
	}
	return nil
}

func (r *Rule) delete(a *ALBIngress) error {
	glog.Infof("%s: Delete Rule %s", a.Name(), *r.CurrentRule.RuleArn)

	resp, err := elbv2svc.svc.DeleteRule(&elbv2.DeleteRuleInput{
		RuleArn: r.CurrentRule.RuleArn,
	})
	spew.Dump(resp)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteRule"}).Add(float64(1))
		return err
	}

	return nil

}

func (r *Rule) needsModification() bool {
	cr := r.CurrentRule
	dr := r.DesiredRule

	switch {
	case cr == nil:
		return true
		// TODO: If we can populate the TargetGroupArn in NewALBIngressFromIngress, we can enable this
	// case awsutil.Prettify(cr.Actions) != awsutil.Prettify(dr.Actions):
	// 	return true
	case awsutil.Prettify(cr.Conditions) != awsutil.Prettify(dr.Conditions):
		return true
	}

	return false
}

// Equals returns true if the two CurrentRule and target rule are the same
// Does not compare priority, since this is not supported by the ingress spec
func (r *Rule) Equals(target *elbv2.Rule) bool {
	switch {
	case r.CurrentRule == nil && target == nil:
		return false
	case r.CurrentRule == nil && target != nil:
		return false
	case r.CurrentRule != nil && target == nil:
		return false
		// a rule is tightly wound to a listener which is also bound to a single TG
		// action only has 2 values, tg arn and a type, type is _always_ forward
	// case !awsutil.DeepEqual(r.CurrentRule.Actions, target.Actions):
	// 	return false
	case !awsutil.DeepEqual(r.CurrentRule.IsDefault, target.IsDefault):
		return false
	case !awsutil.DeepEqual(r.CurrentRule.Conditions, target.Conditions):
		return false
	}
	return true
}
