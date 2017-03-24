package controller

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
)

type Rules []*Rule

func (r Rules) modify(a *ALBIngress, l *Listener, tg *TargetGroup) Rules {
	var rules Rules

	for _, rule := range r {
		switch {
		case rule.DesiredRule == nil:
			rule.delete(a)
			continue
		case rule.CurrentRule == nil:
			if err := rule.create(a, l, tg); err != nil {
				glog.Errorf("%s: Error when creating %s rule %s: %s", a.Name(), *tg.id, *rule.DesiredRule.Conditions[0].Values[0], err)
			}
		case rule.needsModification():
			if err := rule.modify(a, l, tg); err != nil {
				glog.Errorf("%s: Error when modifying rule %s: %s", a.Name(), *rule.CurrentRule.RuleArn, err)
			}
		}
		rules = append(rules, rule)
	}
	return rules
}

func (r Rules) find(rule *elbv2.Rule) int {
	for p, v := range r {
		if v.Equals(rule) {
			return p
		}
	}
	return -1
}

func (r Rules) StripDesiredState() {
	for _, rule := range r {
		rule.DesiredRule = nil
	}
}
