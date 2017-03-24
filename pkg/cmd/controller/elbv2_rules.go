package controller

import "github.com/aws/aws-sdk-go/service/elbv2"

type Rules []*Rule

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
