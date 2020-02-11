package ls

import (
	"context"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/conditions"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/auth"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	mock_auth "github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks/aws-alb-ingress-controller/ingress/auth"
	"github.com/stretchr/testify/assert"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type AuthNewConfigCall struct {
	backend extensions.IngressBackend
	authCfg auth.Config
}

func Test_rulesController_getDesiredRules(t *testing.T) {
	fixedResponseAction := action.Action{
		Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
		FixedResponseConfig: &action.FixedResponseActionConfig{
			ContentType: aws.String("text/plain"),
			StatusCode:  aws.String("503"),
			MessageBody: aws.String("message body"),
		},
	}

	for _, tc := range []struct {
		name               string
		ingress            extensions.Ingress
		ingressAnnos       annotations.Ingress
		tgGroup            tg.TargetGroupGroup
		authNewConfigCalls []AuthNewConfigCall
		expected           []elbv2.Rule
		expectedError      error
	}{
		{
			name: "no path with default backend",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(80),
					},
				},
			},
			expected:      nil,
			expectedError: nil,
		},
		{
			name: "one path with empty HTTP rule value",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{},
						},
					},
				},
			},
			expected:      nil,
			expectedError: nil,
		},
		{
			name: "one empty path with an annotation backend",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "fixed-response-action",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"fixed-response-action": fixedResponseAction,
					},
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "fixed-response-action",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("fixed-response"),
							FixedResponseConfig: &elbv2.FixedResponseActionConfig{
								ContentType: aws.String("text/plain"),
								StatusCode:  aws.String("503"),
								MessageBody: aws.String("message body"),
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "one path with an annotation backend",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/homepage",
											Backend: extensions.IngressBackend{
												ServiceName: "fixed-response-action",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"fixed-response-action": fixedResponseAction,
					},
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "fixed-response-action",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/homepage"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumFixedResponse),
							FixedResponseConfig: &elbv2.FixedResponseActionConfig{
								ContentType: aws.String("text/plain"),
								StatusCode:  aws.String("503"),
								MessageBody: aws.String("message body"),
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "one path with an annotation backend(refers to missing action)",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/homepage",
											Backend: extensions.IngressBackend{
												ServiceName: "missing-action",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: nil,
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "missing-action",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expectedError: errors.New("backend with `servicePort: use-annotation` was configured with `serviceName: missing-action` but an action annotation for missing-action is not set"),
		},
		{
			name: "one path with an service backend",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/homepage",
											Backend: extensions.IngressBackend{
												ServiceName: "service",
												ServicePort: intstr.FromString("http"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: nil,
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/homepage"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path with an service backend(refers to missing service)",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/path",
											Backend: extensions.IngressBackend{
												ServiceName: "missing-service",
												ServicePort: intstr.FromString("http"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: nil,
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "missing-service",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expectedError: errors.New("unable to find targetGroup for backend missing-service:http"),
		},
		{
			name: "two path with an service backend(no auth)",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/path1",
											Backend: extensions.IngressBackend{
												ServiceName: "service1",
												ServicePort: intstr.FromString("http"),
											},
										},
										{
											Path: "/path2",
											Backend: extensions.IngressBackend{
												ServiceName: "service2",
												ServicePort: intstr.FromString("http"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: nil,
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service1", ServicePort: intstr.FromString("http")}: {Arn: "tgArn1"},
					{ServiceName: "service2", ServicePort: intstr.FromString("http")}: {Arn: "tgArn2"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "service1",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
				{
					backend: extensions.IngressBackend{
						ServiceName: "service2",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path1"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn1"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("2"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path2"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn2"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "two path with an service backend(different auth)",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/path1",
											Backend: extensions.IngressBackend{
												ServiceName: "service1",
												ServicePort: intstr.FromString("http"),
											},
										},
										{
											Path: "/path2",
											Backend: extensions.IngressBackend{
												ServiceName: "service2",
												ServicePort: intstr.FromString("http"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: nil,
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service1", ServicePort: intstr.FromString("http")}: {Arn: "tgArn1"},
					{ServiceName: "service2", ServicePort: intstr.FromString("http")}: {Arn: "tgArn2"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "service1",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{
						Type: auth.TypeOIDC,
						IDPOIDC: auth.IDPOIDC{
							AuthenticationRequestExtraParams: auth.AuthenticationRequestExtraParams{
								"param1": "value1",
								"param2": "value2",
							},
							Issuer:                "Issuer",
							AuthorizationEndpoint: "AuthorizationEndpoint",
							TokenEndpoint:         "TokenEndpoint",
							UserInfoEndpoint:      "UserInfoEndpoint",
							ClientId:              "clientId",
							ClientSecret:          "clientSecret",
						},
						Scope:                    "email",
						SessionCookie:            "cookie",
						SessionTimeout:           100,
						OnUnauthenticatedRequest: "authenticate",
					},
				},
				{
					backend: extensions.IngressBackend{
						ServiceName: "service2",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{
						Type: auth.TypeCognito,
						IDPCognito: auth.IDPCognito{
							AuthenticationRequestExtraParams: auth.AuthenticationRequestExtraParams{
								"param1": "value1",
								"param2": "value2",
							},
							UserPoolArn:      "UserPoolArn",
							UserPoolClientId: "UserPoolClientId",
							UserPoolDomain:   "UserPoolDomain",
						},
						Scope:                    "openid",
						SessionCookie:            "cookie",
						SessionTimeout:           100,
						OnUnauthenticatedRequest: "authenticate",
					},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path1"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("authenticate-oidc"),
							AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
								AuthenticationRequestExtraParams: aws.StringMap(auth.AuthenticationRequestExtraParams{
									"param1": "value1",
									"param2": "value2",
								}),
								Issuer:                   aws.String("Issuer"),
								AuthorizationEndpoint:    aws.String("AuthorizationEndpoint"),
								TokenEndpoint:            aws.String("TokenEndpoint"),
								UserInfoEndpoint:         aws.String("UserInfoEndpoint"),
								ClientId:                 aws.String("clientId"),
								ClientSecret:             aws.String("clientSecret"),
								Scope:                    aws.String("email"),
								SessionCookieName:        aws.String("cookie"),
								SessionTimeout:           aws.Int64(100),
								OnUnauthenticatedRequest: aws.String("authenticate"),
							},
						},
						{
							Order: aws.Int64(2),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn1"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("2"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path2"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("authenticate-cognito"),
							AuthenticateCognitoConfig: &elbv2.AuthenticateCognitoActionConfig{
								AuthenticationRequestExtraParams: aws.StringMap(auth.AuthenticationRequestExtraParams{
									"param1": "value1",
									"param2": "value2",
								}),
								UserPoolArn:              aws.String("UserPoolArn"),
								UserPoolClientId:         aws.String("UserPoolClientId"),
								UserPoolDomain:           aws.String("UserPoolDomain"),
								Scope:                    aws.String("openid"),
								SessionCookieName:        aws.String("cookie"),
								SessionTimeout:           aws.Int64(100),
								OnUnauthenticatedRequest: aws.String("authenticate"),
							},
						},
						{
							Order: aws.Int64(2),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn2"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path with host/path condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							Host: "www.example.com",
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/path",
											Backend: extensions.IngressBackend{
												ServiceName: "service",
												ServicePort: intstr.FromString("http"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: nil,
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
								Values: aws.StringSlice([]string{"www.example.com"}),
							},
						},
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path without host/path condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "service",
												ServicePort: intstr.FromString("http"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: nil,
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path with host condition but without path condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							Host: "www.example.com",
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "service",
												ServicePort: intstr.FromString("http"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: nil,
				},
				Conditions: &conditions.Config{
					Conditions: nil,
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromString("http"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
								Values: aws.StringSlice([]string{"www.example.com"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path without host/path and with annotation host condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "anno-svc",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"anno-svc": {
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &action.ForwardActionConfig{
								TargetGroups: []*action.TargetGroupTuple{
									{
										ServiceName: aws.String("service"),
										ServicePort: aws.String("http"),
										Weight:      aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Conditions: &conditions.Config{
					Conditions: map[string][]conditions.RuleCondition{
						"anno-svc": {
							{
								Field: aws.String(conditions.FieldHostHeader),
								HostHeaderConfig: &conditions.HostHeaderConditionConfig{
									Values: aws.StringSlice([]string{"anno.example.com"}),
								},
							},
						},
					},
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "anno-svc",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
								Values: aws.StringSlice([]string{"anno.example.com"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path without host/path and with annotation host condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "anno-svc",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"anno-svc": {
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &action.ForwardActionConfig{
								TargetGroups: []*action.TargetGroupTuple{
									{
										ServiceName: aws.String("service"),
										ServicePort: aws.String("http"),
										Weight:      aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Conditions: &conditions.Config{
					Conditions: map[string][]conditions.RuleCondition{
						"anno-svc": {
							{
								Field: aws.String(conditions.FieldHostHeader),
								HostHeaderConfig: &conditions.HostHeaderConditionConfig{
									Values: aws.StringSlice([]string{"anno.example.com"}),
								},
							},
						},
					},
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "anno-svc",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
								Values: aws.StringSlice([]string{"anno.example.com"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path without host/path and with annotation path condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "anno-svc",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"anno-svc": {
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &action.ForwardActionConfig{
								TargetGroups: []*action.TargetGroupTuple{
									{
										ServiceName: aws.String("service"),
										ServicePort: aws.String("http"),
										Weight:      aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Conditions: &conditions.Config{
					Conditions: map[string][]conditions.RuleCondition{
						"anno-svc": {
							{
								Field: aws.String(conditions.FieldPathPattern),
								PathPatternConfig: &conditions.PathPatternConditionConfig{
									Values: aws.StringSlice([]string{"/anno"}),
								},
							},
						},
					},
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "anno-svc",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/anno"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path with host/path and with annotation path condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							Host: "www.example.com",
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/path",
											Backend: extensions.IngressBackend{
												ServiceName: "anno-svc",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"anno-svc": {
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &action.ForwardActionConfig{
								TargetGroups: []*action.TargetGroupTuple{
									{
										ServiceName: aws.String("service"),
										ServicePort: aws.String("http"),
										Weight:      aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Conditions: &conditions.Config{
					Conditions: map[string][]conditions.RuleCondition{
						"anno-svc": {
							{
								Field: aws.String(conditions.FieldHostHeader),
								HostHeaderConfig: &conditions.HostHeaderConditionConfig{
									Values: aws.StringSlice([]string{"anno.example.com"}),
								},
							},
							{
								Field: aws.String(conditions.FieldPathPattern),
								PathPatternConfig: &conditions.PathPatternConditionConfig{
									Values: aws.StringSlice([]string{"/anno"}),
								},
							},
						},
					},
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "anno-svc",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
								Values: aws.StringSlice([]string{"www.example.com", "anno.example.com"}),
							},
						},
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path", "/anno"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path without host/path and with annotation http header condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "anno-svc",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"anno-svc": {
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &action.ForwardActionConfig{
								TargetGroups: []*action.TargetGroupTuple{
									{
										ServiceName: aws.String("service"),
										ServicePort: aws.String("http"),
										Weight:      aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Conditions: &conditions.Config{
					Conditions: map[string][]conditions.RuleCondition{
						"anno-svc": {
							{
								Field: aws.String(conditions.FieldHTTPHeader),
								HttpHeaderConfig: &conditions.HttpHeaderConditionConfig{
									HttpHeaderName: aws.String("headerKey"),
									Values:         aws.StringSlice([]string{"headerValue"}),
								},
							},
						},
					},
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "anno-svc",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHTTPHeader),
							HttpHeaderConfig: &elbv2.HttpHeaderConditionConfig{
								HttpHeaderName: aws.String("headerKey"),
								Values:         aws.StringSlice([]string{"headerValue"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path without host/path and with annotation http request method condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "anno-svc",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"anno-svc": {
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &action.ForwardActionConfig{
								TargetGroups: []*action.TargetGroupTuple{
									{
										ServiceName: aws.String("service"),
										ServicePort: aws.String("http"),
										Weight:      aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Conditions: &conditions.Config{
					Conditions: map[string][]conditions.RuleCondition{
						"anno-svc": {
							{
								Field: aws.String(conditions.FieldHTTPRequestMethod),
								HttpRequestMethodConfig: &conditions.HttpRequestMethodConditionConfig{
									Values: aws.StringSlice([]string{"GET", "HEAD"}),
								},
							},
						},
					},
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "anno-svc",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHTTPRequestMethod),
							HttpRequestMethodConfig: &elbv2.HttpRequestMethodConditionConfig{
								Values: aws.StringSlice([]string{"GET", "HEAD"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path without host/path and with annotation query string condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "anno-svc",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"anno-svc": {
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &action.ForwardActionConfig{
								TargetGroups: []*action.TargetGroupTuple{
									{
										ServiceName: aws.String("service"),
										ServicePort: aws.String("http"),
										Weight:      aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Conditions: &conditions.Config{
					Conditions: map[string][]conditions.RuleCondition{
						"anno-svc": {
							{
								Field: aws.String(conditions.FieldQueryString),
								QueryStringConfig: &conditions.QueryStringConditionConfig{
									Values: []*conditions.QueryStringKeyValuePair{
										{
											Key:   aws.String("paramKey"),
											Value: aws.String("paramValue"),
										},
									},
								},
							},
						},
					},
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "anno-svc",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldQueryString),
							QueryStringConfig: &elbv2.QueryStringConditionConfig{
								Values: []*elbv2.QueryStringKeyValuePair{
									{
										Key:   aws.String("paramKey"),
										Value: aws.String("paramValue"),
									},
								},
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "one path without host/path and with annotation source IP condition",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "anno-svc",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ingressAnnos: annotations.Ingress{
				Action: &action.Config{
					Actions: map[string]action.Action{
						"anno-svc": {
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &action.ForwardActionConfig{
								TargetGroups: []*action.TargetGroupTuple{
									{
										ServiceName: aws.String("service"),
										ServicePort: aws.String("http"),
										Weight:      aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Conditions: &conditions.Config{
					Conditions: map[string][]conditions.RuleCondition{
						"anno-svc": {
							{
								Field: aws.String(conditions.FieldSourceIP),
								SourceIpConfig: &conditions.SourceIpConditionConfig{
									Values: aws.StringSlice([]string{"192.168.0.0/16"}),
								},
							},
						},
					},
				},
			},
			tgGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{ServiceName: "service", ServicePort: intstr.FromString("http")}: {Arn: "tgArn"},
				},
			},
			authNewConfigCalls: []AuthNewConfigCall{
				{
					backend: extensions.IngressBackend{
						ServiceName: "anno-svc",
						ServicePort: intstr.FromString("use-annotation"),
					},
					authCfg: auth.Config{Type: auth.TypeNone},
				},
			},
			expected: []elbv2.Rule{
				{
					IsDefault: aws.Bool(false),
					Priority:  aws.String("1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldSourceIP),
							SourceIpConfig: &elbv2.SourceIpConditionConfig{
								Values: aws.StringSlice([]string{"192.168.0.0/16"}),
							},
						},
					},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("forward"),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cloud := &mocks.CloudAPI{}
			mockAuthModule := mock_auth.NewMockModule(ctrl)
			for _, call := range tc.authNewConfigCalls {
				mockAuthModule.EXPECT().NewConfig(gomock.Any(), &tc.ingress, call.backend, gomock.Any()).Return(call.authCfg, nil)
			}

			c := &rulesController{
				cloud:      cloud,
				authModule: mockAuthModule,
			}

			got, err := c.getDesiredRules(context.Background(), &elbv2.Listener{}, &tc.ingress, &tc.ingressAnnos, tc.tgGroup)
			assert.Equal(t, tc.expected, got)
			if tc.expectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError.Error())
			}
		})
	}
}

type GetRulesCall struct {
	Output []*elbv2.Rule
	Error  error
}

func Test_getCurrentRules(t *testing.T) {
	listenerArn := "listenerArn"
	tgArn := "tgArn"

	for _, tc := range []struct {
		name          string
		getRulesCall  *GetRulesCall
		expected      []elbv2.Rule
		expectedError error
	}{
		{
			name:          "DescribeRulesRequest returns an error",
			getRulesCall:  &GetRulesCall{Output: nil, Error: errors.New("Some error")},
			expectedError: errors.New("Some error"),
		},
		{
			name: "DescribeRulesRequest returns one rule",
			getRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
				},
			}},
			expected: []elbv2.Rule{
				{

					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
				},
			},
		},
		{
			name: "DescribeRulesRequest returns four rules, default rule is ignored",
			getRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:  aws.String("default"),
					IsDefault: aws.Bool(true),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/1"}),
							},
						},
					},
				},
				{
					Priority: aws.String("3"),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/3"}),
							},
						},
					},
				},
				{
					Priority: aws.String("4"),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/4"}),
							},
						},
					},
				},
			}},
			expected: []elbv2.Rule{
				{

					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/1"}),
							},
						},
					},
				},
				{

					Priority: aws.String("3"),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/3"}),
							},
						},
					},
				},
				{

					Priority: aws.String("4"),
					Actions: []*elbv2.Action{
						{
							Type: aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String(tgArn),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldHostHeader),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/4"}),
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.getRulesCall != nil {
				cloud.On("GetRules", ctx, listenerArn).Return(tc.getRulesCall.Output, tc.getRulesCall.Error)
			}
			controller := &rulesController{
				cloud: cloud,
			}
			results, err := controller.getCurrentRules(ctx, listenerArn)
			assert.Equal(t, tc.expected, results)
			if tc.expectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError.Error())
			}
			cloud.AssertExpectations(t)
		})
	}
}

type CreateRuleCall struct {
	Input *elbv2.CreateRuleInput
	Error error
}

type ModifyRuleCall struct {
	Input *elbv2.ModifyRuleInput
	Error error
}

type DeleteRuleCall struct {
	Input *elbv2.DeleteRuleInput
	Error error
}

func Test_reconcileRules(t *testing.T) {
	listenerArn := aws.String("lsArn")
	tgArn := aws.String("tgArn")
	for _, tc := range []struct {
		name           string
		current        []elbv2.Rule
		desired        []elbv2.Rule
		createRuleCall *CreateRuleCall
		modifyRuleCall *ModifyRuleCall
		deleteRuleCall *DeleteRuleCall
		expectedError  error
	}{
		{
			name:    "Empty ruleset for current and desired, no actions",
			current: []elbv2.Rule{},
			desired: []elbv2.Rule{},
		},
		{
			name: "Add one rule",
			current: []elbv2.Rule{
				{

					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
			},
			desired: []elbv2.Rule{
				{

					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
				{

					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("2"),
				},
			},
			createRuleCall: &CreateRuleCall{
				Input: &elbv2.CreateRuleInput{
					ListenerArn: listenerArn,
					Priority:    aws.Int64(2),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
				},
			},
		},
		{
			name:    "CreateRule error",
			current: []elbv2.Rule{},
			desired: []elbv2.Rule{
				{

					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
			},
			createRuleCall: &CreateRuleCall{
				Input: &elbv2.CreateRuleInput{
					ListenerArn: listenerArn,
					Priority:    aws.Int64(1),
					Conditions: []*elbv2.RuleCondition{{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/path/*"}),
						},
					}},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
				},
				Error: errors.New("create rule error"),
			},
			expectedError: errors.New("failed creating rule 1 on lsArn due to create rule error"),
		}, {
			name: "Remove one rule",
			current: []elbv2.Rule{
				{
					RuleArn: aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
				{

					RuleArn: aws.String("RuleArn2"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/path/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("2"),
				},
			},
			desired: []elbv2.Rule{
				{
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
			},
			deleteRuleCall: &DeleteRuleCall{
				Input: &elbv2.DeleteRuleInput{
					RuleArn: aws.String("RuleArn2"),
				},
			},
		},
		{
			name: "DeleteRule error",
			current: []elbv2.Rule{
				{
					RuleArn: aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
			},
			deleteRuleCall: &DeleteRuleCall{
				Input: &elbv2.DeleteRuleInput{
					RuleArn: aws.String("RuleArn1"),
				},
				Error: errors.New("delete rule error"),
			},
			expectedError: errors.New("failed deleting rule 1 on lsArn due to delete rule error"),
		},
		{
			name: "Modify one rule",
			current: []elbv2.Rule{
				{
					RuleArn: aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
			},
			desired: []elbv2.Rule{
				{
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/new/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
			},
			modifyRuleCall: &ModifyRuleCall{
				Input: &elbv2.ModifyRuleInput{
					RuleArn: aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/new/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
				},
			},
		},
		{
			name: "ModifyRule error",
			current: []elbv2.Rule{
				{
					RuleArn: aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
			},
			desired: []elbv2.Rule{
				{
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/new/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
					Priority: aws.String("1"),
				},
			},
			modifyRuleCall: &ModifyRuleCall{
				Input: &elbv2.ModifyRuleInput{
					RuleArn: aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{
						{
							Field: aws.String(conditions.FieldPathPattern),
							PathPatternConfig: &elbv2.PathPatternConditionConfig{
								Values: aws.StringSlice([]string{"/new/*"}),
							},
						},
					},
					Actions: []*elbv2.Action{{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: tgArn,
									Weight:         aws.Int64(1),
								},
							},
						},
					}},
				},
				Error: errors.New("modify rule error"),
			},
			expectedError: errors.New("failed modifying rule 1 on lsArn due to modify rule error"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.createRuleCall != nil {
				cloud.On("CreateRuleWithContext", ctx, tc.createRuleCall.Input).Return(nil, tc.createRuleCall.Error)
			}
			if tc.modifyRuleCall != nil {
				cloud.On("ModifyRuleWithContext", ctx, tc.modifyRuleCall.Input).Return(nil, tc.modifyRuleCall.Error)
			}
			if tc.deleteRuleCall != nil {
				cloud.On("DeleteRuleWithContext", ctx, tc.deleteRuleCall.Input).Return(nil, tc.deleteRuleCall.Error)
			}

			controller := &rulesController{
				cloud: cloud,
			}
			err := controller.reconcileRules(context.Background(), *listenerArn, tc.current, tc.desired)
			if tc.expectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError.Error())
			}
			cloud.AssertExpectations(t)
		})
	}
}

func Test_createsRedirectLoop(t *testing.T) {
	for _, tc := range []struct {
		name     string
		listener elbv2.Listener
		rule     elbv2.Rule

		expected bool
	}{
		{
			name:     "No RedirectConfig",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule:     elbv2.Rule{},
			expected: false,
		},
		{
			name:     "Host variable set to #{host}",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Host: aws.String("#{host}")}),
					},
				},
			},
			expected: true,
		},
		{
			name:     "Host variable set to same value as host-header",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Host: aws.String("reused.hostname")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"reused.hostname"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Host variable set to same value contained by host-header",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Host: aws.String("reused.hostname")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"anno.hostname", "reused.hostname"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Host variable set to new hostname (no host-header)",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Host: aws.String("new.hostname")}),
					},
				},
			},
			expected: false,
		},
		{
			name:     "Host variable set to new hostname (with host-header)",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Host: aws.String("new.hostname")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"old.hostname"}),
						},
					},
				},
			},
			expected: false,
		},
		{
			name:     "Path variable set to /#{path}",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
			},
			expected: true,
		},
		{
			name:     "Path variable set to same value as path-pattern",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/path/reused")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/path/reused"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Path variable set to same value contained by path-pattern",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/path/reused")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/path/anno", "/path/reused"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Path variable set to new path(no path-pattern)",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/path/new")}),
					},
				},
			},
			expected: false,
		},
		{
			name:     "Path variable set to new path(with path-pattern)",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/path/new")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/path/old"}),
						},
					},
				},
			},
			expected: false,
		},
		{
			name:     "Port variable set to #{port}",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Port: aws.String("#{port}")}),
					},
				},
			},
			expected: true,
		},
		{
			name:     "Port variable set to same port as listener",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Port: aws.String("80")}),
					},
				},
			},
			expected: true,
		},
		{
			name:     "Port variable set to different port as listener",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Port: aws.String("999")}),
					},
				},
			},
			expected: false,
		},
		{
			name:     "Query variable set to #{query}",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Query: aws.String("#{query}")}),
					},
				},
			},
			expected: true,
		},
		{
			name:     "Query variable set to new query",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Query: aws.String("query=new")}),
					},
				},
			},
			expected: false,
		},
		{
			name:     "Protocol variable set to #{protocol}",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Protocol: aws.String("#{protocol}")}),
					},
				},
			},
			expected: true,
		},
		{
			name:     "Protocol variable set to same protocol",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Protocol: aws.String("HTTP")}),
					},
				},
			},
			expected: true,
		},
		{
			name:     "Protocol variable set to new protocol",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Protocol: aws.String("HTTPS")}),
					},
				},
			},
			expected: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, createsRedirectLoop(&tc.listener, tc.rule))
		})
	}
}

func Test_isUnconditionalRedirect(t *testing.T) {
	for _, tc := range []struct {
		name     string
		listener elbv2.Listener
		rule     elbv2.Rule

		expected bool
	}{
		{
			name:     "No RedirectConfig",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule:     elbv2.Rule{},
			expected: false,
		},
		{
			name:     "No conditions",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
				Conditions: nil,
			},
			expected: true,
		},
		{
			name:     "No Path conditions",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"www.example.com", "anno.example.com"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Path condition set to /*",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/*"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Multiple Path conditions, one of which is /*",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/*", "/test", "/annothertest"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Multiple Path conditions, one of which is /*, different ordering",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/test", "/anothertest", "/*"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Multiple Path conditions, none of which is /*",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/test", "/anothertest", "anothertest2"}),
						},
					},
				},
			},
			expected: false,
		},
		{
			name:     "Path condition set to /* and Host condition is set ",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/*"}),
						},
					},
					{
						Field: aws.String(conditions.FieldHostHeader),
						HostHeaderConfig: &elbv2.HostHeaderConditionConfig{
							Values: aws.StringSlice([]string{"www.example.com", "anno.example.com"}),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Path condition set to /* but a SourceIP condition is also set",
			listener: elbv2.Listener{Protocol: aws.String("HTTP"), Port: aws.Int64(80)},
			rule: elbv2.Rule{
				Actions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumRedirect),
						RedirectConfig: redirectActionConfig(&elbv2.RedirectActionConfig{Path: aws.String("/#{path}")}),
					},
				},
				Conditions: []*elbv2.RuleCondition{
					{
						Field: aws.String(conditions.FieldPathPattern),
						PathPatternConfig: &elbv2.PathPatternConditionConfig{
							Values: aws.StringSlice([]string{"/*"}),
						},
					},
					{
						Field: aws.String(conditions.FieldSourceIP),
						SourceIpConfig: &elbv2.SourceIpConditionConfig{
							Values: aws.StringSlice([]string{"192.168.0.0/16"}),
						},
					},
				},
			},
			expected: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isUnconditionalRedirect(&tc.listener, tc.rule))
		})
	}
}

func redirectActionConfig(override *elbv2.RedirectActionConfig) *elbv2.RedirectActionConfig {
	r := &elbv2.RedirectActionConfig{
		Host:       aws.String("#{host}"),
		Path:       aws.String("/#{path}"),
		Port:       aws.String("#{port}"),
		Protocol:   aws.String("#{protocol}"),
		Query:      aws.String("#{query}"),
		StatusCode: aws.String("301"),
	}
	if override.Host != nil {
		r.Host = override.Host
	}
	if override.Path != nil {
		r.Path = override.Path
	}
	if override.Port != nil {
		r.Port = override.Port
	}
	if override.Protocol != nil {
		r.Protocol = override.Protocol
	}
	if override.Query != nil {
		r.Query = override.Query
	}
	if override.StatusCode != nil {
		r.StatusCode = override.StatusCode
	}
	return r
}
