package alb

/* TODO: Due to data structure changes, need to redo this path
import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

func TestNewRule(t *testing.T) {
	setup()

	var tests = []struct {
		path *string
		rule *elbv2.Rule
		pass bool
	}{
		{ // Test defaults
			aws.String("/"),
			&elbv2.Rule{
				IsDefault: aws.Bool(true),
				Priority:  aws.String("default"),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
			},
			true,
		},
		{ // Test non-standard path
			aws.String("/test"),
			&elbv2.Rule{
				IsDefault: aws.Bool(false),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					&elbv2.RuleCondition{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/test")},
					},
				},
			},
			true,
		},
	}

	for _, tt := range tests {
		rule := NewRule(tt.path).DesiredRule
		r := &Rule{
			CurrentRule: rule,
		}
		if !r.Equals(tt.rule) && tt.pass {
			t.Errorf("NewRule(%v) returned an unexpected rule:\n%s\n!=\n%s", *tt.path, awsutil.Prettify(rule), awsutil.Prettify(tt.rule))
		}
	}
}

func TestRuleEquals(t *testing.T) {
	setup()

	var tests = []struct {
		rule   *elbv2.Rule
		target *elbv2.Rule
		pass   bool
	}{
		{ // Test equals: nil target
			&elbv2.Rule{
				IsDefault: aws.Bool(true),
				Priority:  aws.String("default"),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
			},
			nil,
			false,
		},
		{ // Test equals: nil source
			nil,
			&elbv2.Rule{
				IsDefault: aws.Bool(true),
				Priority:  aws.String("default"),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
			},
			false,
		},
		{ // Test equals: all equal
			&elbv2.Rule{
				IsDefault: aws.Bool(true),
				Priority:  aws.String("default"),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
			},
			&elbv2.Rule{
				IsDefault: aws.Bool(true),
				Priority:  aws.String("default"),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
			},
			true,
		},
		// { // Test equals: Actions
		// 	&elbv2.Rule{
		// 		IsDefault: aws.Bool(true),
		// 		Priority:  aws.String("default"),
		// 		Actions: []*elbv2.Action{
		// 			&elbv2.Action{
		// 				TargetGroupArn: aws.String("arn:blah"),
		// 				Type:           aws.String("forward"),
		// 			},
		// 		},
		// 	},
		// 	&elbv2.Rule{
		// 		IsDefault: aws.Bool(true),
		// 		Priority:  aws.String("default"),
		// 		Actions: []*elbv2.Action{
		// 			&elbv2.Action{
		// 				TargetGroupArn: aws.String("arn:wrong"),
		// 				Type:           aws.String("forward"),
		// 			},
		// 		},
		// 	},
		// 	false,
		// },
		{ // Test equals: IsDefault
			&elbv2.Rule{
				IsDefault: aws.Bool(true),
				Priority:  aws.String("default"),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
			},
			&elbv2.Rule{
				IsDefault: aws.Bool(false),
				Priority:  aws.String("default"),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
			},
			false,
		},
		{ // Test equals: Conditions
			&elbv2.Rule{
				IsDefault: aws.Bool(false),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					&elbv2.RuleCondition{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/test")},
					},
				},
			},
			&elbv2.Rule{
				IsDefault: aws.Bool(false),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					&elbv2.RuleCondition{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/test")},
					},
				},
			},
			true,
		},
		{ // Test equals: Conditions
			&elbv2.Rule{
				IsDefault: aws.Bool(false),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					&elbv2.RuleCondition{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/test")},
					},
				},
			},
			&elbv2.Rule{
				IsDefault: aws.Bool(false),
				Actions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("arn:blah"),
						Type:           aws.String("forward"),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					&elbv2.RuleCondition{
						Field:  aws.String("path-pattern"),
						Values: []*string{aws.String("/test_wrong")},
					},
				},
			},
			false,
		},
	}

	for testNum, tt := range tests {
		r := &Rule{
			CurrentRule: tt.rule,
		}
		if !r.Equals(tt.target) && tt.pass {
			t.Errorf("%d: r.Equalts(%v) returned false but should have passed", testNum, *tt.target)
		}
		if r.Equals(tt.target) && !tt.pass {
			t.Errorf("%d: r.Equalts(%v) returned true but should have falsed", testNum, *tt.target)
		}
	}
}

func TestRulesFind(t *testing.T) {
	setup()

	var tests = []struct {
		rule *elbv2.Rule
		pos  int
	}{
		{
			NewRule(aws.String("/")).DesiredRule,
			0,
		},
		{
			NewRule(aws.String("/altpath")).DesiredRule,
			1,
		},
		{
			NewRule(aws.String("/doesnt_exit")).DesiredRule,
			-1,
		},
	}

	rules := &Rules{
		&Rule{CurrentRule: NewRule(aws.String("/")).DesiredRule},
		&Rule{CurrentRule: NewRule(aws.String("/altpath")).DesiredRule},
	}

	for n, tt := range tests {
		pos := rules.Find(tt.rule)
		if pos != tt.pos {
			t.Errorf("%d: rules.Find(%v) returned %d, expected %d", n, awsutil.Prettify(tt.rule), pos, tt.pos)
		}
	}
}*/
