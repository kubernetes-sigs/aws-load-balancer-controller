package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

// type Rule struct {

// 	// The actions.
// 	Actions []*Action `type:"list"`

// 	// The conditions.
// 	Conditions []*RuleCondition `type:"list"`

// 	// Indicates whether this is the default rule.
// 	IsDefault *bool `type:"boolean"`

// 	// The priority.
// 	Priority *string `type:"string"`

// 	// The Amazon Resource Name (ARN) of the rule.
// 	RuleArn *string `type:"string"`
// 	// contains filtered or unexported fields
// }

type Rule struct {
	CurrentRule *elbv2.Rule
	DesiredRule *elbv2.Rule
}

type Rules []*Rule

func NewRule(targetGroupArn, path *string) *elbv2.Rule {
	r := &elbv2.Rule{
		Actions: []*elbv2.Action{
			&elbv2.Action{
				TargetGroupArn: targetGroupArn,
				Type:           aws.String("forward"),
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

	return r
}

// Equals returns true if the two CurrentRule and target rule are the same
// Does not compare priority, since this is not supported by the ingress spec
func (r *Rule) Equals(target *elbv2.Rule) bool {
	switch {
	case r.CurrentRule == nil && target != nil:
		return false
	case r.CurrentRule != nil && target == nil:
		return false
	case !awsutil.DeepEqual(r.CurrentRule.Actions, target.Actions):
		return false
	case !awsutil.DeepEqual(r.CurrentRule.IsDefault, target.IsDefault):
		return false
	case !awsutil.DeepEqual(r.CurrentRule.Conditions, target.Conditions):
		return false
	}
	return true
}

func (r Rules) find(rule *Rule) int {
	for p, v := range r {
		if rule.Equals(v.CurrentRule) {
			return p
		}
	}
	return -1
}
