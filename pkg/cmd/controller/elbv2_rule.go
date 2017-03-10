package controller

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
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

func (elb *ELBV2) describeRules(listenerArn *string) []*elbv2.Rule {
	describeRulesInput := &elbv2.DescribeRulesInput{
		ListenerArn: listenerArn,
	}

	describeRulesOutput, err := elb.svc.DescribeRules(describeRulesInput)
	if err != nil {
		glog.Fatal(err)
	}

	return describeRulesOutput.Rules
}
