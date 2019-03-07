package ls

import (
	"context"
	"errors"
	"testing"

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

func Test_getDesiredRules(t *testing.T) {
	for _, tc := range []struct {
		name         string
		ingress      extensions.Ingress
		ingressAnnos *annotations.Ingress
		targetGroups tg.TargetGroupGroup

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
			name: "one path with an annotation backend",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/*",
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
			ingressAnnos: annotations.NewIngressDummy(),
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
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
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
			name: "one path with an annotation backend(refers to missing action)",
			ingress: extensions.Ingress{
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/*",
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
			ingressAnnos: annotations.NewIngressDummy(),
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
			ingressAnnos: annotations.NewIngressDummy(),
			targetGroups: tg.TargetGroupGroup{
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
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/path")},
					Actions: []*elbv2.Action{
						{
							Order:          aws.Int64(1),
							Type:           aws.String("forward"),
							TargetGroupArn: aws.String("tgArn"),
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
			ingressAnnos: annotations.NewIngressDummy(),
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
			ingressAnnos: annotations.NewIngressDummy(),
			targetGroups: tg.TargetGroupGroup{
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
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/path1")},
					Actions: []*elbv2.Action{
						{
							Order:          aws.Int64(1),
							Type:           aws.String("forward"),
							TargetGroupArn: aws.String("tgArn1"),
						},
					},
				},
				{
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("2"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/path2")},
					Actions: []*elbv2.Action{
						{
							Order:          aws.Int64(1),
							Type:           aws.String("forward"),
							TargetGroupArn: aws.String("tgArn2"),
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
			ingressAnnos: annotations.NewIngressDummy(),
			targetGroups: tg.TargetGroupGroup{
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
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/path1")},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("authenticate-oidc"),
							AuthenticateOidcConfig: &elbv2.AuthenticateOidcActionConfig{
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
							Order:          aws.Int64(2),
							Type:           aws.String("forward"),
							TargetGroupArn: aws.String("tgArn1"),
						},
					},
				},
				{
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("2"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/path2")},
					Actions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String("authenticate-cognito"),
							AuthenticateCognitoConfig: &elbv2.AuthenticateCognitoActionConfig{
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
							Order:          aws.Int64(2),
							Type:           aws.String("forward"),
							TargetGroupArn: aws.String("tgArn2"),
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
			ingressAnnos: annotations.NewIngressDummy(),
			targetGroups: tg.TargetGroupGroup{
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
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("1"),
					Conditions: []*elbv2.RuleCondition{condition("host-header", "www.example.com"), condition("path-pattern", "/path")},
					Actions: []*elbv2.Action{
						{
							Order:          aws.Int64(1),
							Type:           aws.String("forward"),
							TargetGroupArn: aws.String("tgArn"),
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
			ingressAnnos: annotations.NewIngressDummy(),
			targetGroups: tg.TargetGroupGroup{
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
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
					Actions: []*elbv2.Action{
						{
							Order:          aws.Int64(1),
							Type:           aws.String("forward"),
							TargetGroupArn: aws.String("tgArn"),
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
			ingressAnnos: annotations.NewIngressDummy(),
			targetGroups: tg.TargetGroupGroup{
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
					IsDefault:  aws.Bool(false),
					Priority:   aws.String("1"),
					Conditions: []*elbv2.RuleCondition{condition("host-header", "www.example.com")},
					Actions: []*elbv2.Action{
						{
							Order:          aws.Int64(1),
							Type:           aws.String("forward"),
							TargetGroupArn: aws.String("tgArn"),
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

			controller := &RulesController{
				cloud:      cloud,
				authModule: mockAuthModule,
			}

			results, err := controller.getDesiredRules(context.Background(), &elbv2.Listener{}, &tc.ingress, tc.ingressAnnos, tc.targetGroups)
			assert.Equal(t, tc.expected, results)
			assert.Equal(t, tc.expectedError, err)
			cloud.AssertExpectations(t)
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
					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
				},
			}},
			expected: []elbv2.Rule{
				{

					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
				},
			},
		},
		{
			name: "DescribeRulesRequest returns four rules, default rule is ignored",
			getRulesCall: &GetRulesCall{Output: []*elbv2.Rule{
				{
					Priority:   aws.String("default"),
					IsDefault:  aws.Bool(true),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
				},
				{
					Priority:   aws.String("1"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
				},
				{
					Priority:   aws.String("3"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/2*")},
				},
				{
					Priority:   aws.String("4"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumFixedResponse)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/3*")},
				},
			}},
			expected: []elbv2.Rule{
				{

					Priority: aws.String("1"),
					Actions:  []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{
						condition("path-pattern", "/*"),
					},
				},
				{

					Priority:   aws.String("3"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumForward), TargetGroupArn: aws.String(tgArn)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/2*")},
				},
				{

					Priority:   aws.String("4"),
					Actions:    []*elbv2.Action{{Type: aws.String(elbv2.ActionTypeEnumFixedResponse)}},
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/3*")},
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
			controller := &RulesController{
				cloud: cloud,
			}
			results, err := controller.getCurrentRules(ctx, listenerArn)
			assert.Equal(t, tc.expected, results)
			assert.Equal(t, tc.expectedError, err)
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

					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("1"),
				},
			},
			desired: []elbv2.Rule{
				{

					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("1"),
				},
				{

					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/path/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("2"),
				},
			},
			createRuleCall: &CreateRuleCall{
				Input: &elbv2.CreateRuleInput{
					ListenerArn: listenerArn,
					Priority:    aws.Int64(2),
					Conditions:  []*elbv2.RuleCondition{condition("path-pattern", "/path/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
				},
			},
		},
		{
			name:    "CreateRule error",
			current: []elbv2.Rule{},
			desired: []elbv2.Rule{
				{

					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/path/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("1"),
				},
			},
			createRuleCall: &CreateRuleCall{
				Input: &elbv2.CreateRuleInput{
					ListenerArn: listenerArn,
					Priority:    aws.Int64(1),
					Conditions:  []*elbv2.RuleCondition{condition("path-pattern", "/path/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
				},
				Error: errors.New("create rule error"),
			},
			expectedError: errors.New("failed creating rule 1 on lsArn due to create rule error"),
		}, {
			name: "Remove one rule",
			current: []elbv2.Rule{
				{
					RuleArn:    aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("1"),
				},
				{

					RuleArn:    aws.String("RuleArn2"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/path/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("2"),
				},
			},
			desired: []elbv2.Rule{
				{
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
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
					RuleArn:    aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
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
					RuleArn:    aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("1"),
				},
			},
			desired: []elbv2.Rule{
				{
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/new/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("1"),
				},
			},
			modifyRuleCall: &ModifyRuleCall{
				Input: &elbv2.ModifyRuleInput{
					RuleArn:    aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/new/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
				},
			},
		},
		{
			name: "ModifyRule error",
			current: []elbv2.Rule{
				{
					RuleArn:    aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("1"),
				},
			},
			desired: []elbv2.Rule{
				{
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/new/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
					}},
					Priority: aws.String("1"),
				},
			},
			modifyRuleCall: &ModifyRuleCall{
				Input: &elbv2.ModifyRuleInput{
					RuleArn:    aws.String("RuleArn1"),
					Conditions: []*elbv2.RuleCondition{condition("path-pattern", "/new/*")},
					Actions: []*elbv2.Action{{
						Order:          aws.Int64(1),
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: tgArn,
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

			controller := &RulesController{
				cloud: cloud,
			}
			err := controller.reconcileRules(context.Background(), *listenerArn, tc.current, tc.desired)
			assert.Equal(t, tc.expectedError, err)
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
					condition("host-header", "reused.hostname"),
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
					condition("host-header", "old.hostname"),
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
					condition("path-pattern", "/path/reused"),
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
					condition("path-pattern", "/path/old"),
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

func Test_condition(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Field    string
		Values   []string
		Expected *elbv2.RuleCondition
	}{
		{
			Name:     "one value",
			Field:    "field name",
			Values:   []string{"val1"},
			Expected: &elbv2.RuleCondition{Field: aws.String("field name"), Values: aws.StringSlice([]string{"val1"})},
		},
		{
			Name:     "three Values",
			Field:    "field name",
			Values:   []string{"val1", "val2", "val3"},
			Expected: &elbv2.RuleCondition{Field: aws.String("field name"), Values: aws.StringSlice([]string{"val1", "val2", "val3"})},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			o := condition(tc.Field, tc.Values...)
			assert.Equal(t, tc.Expected, o)
		})
	}
}

func Test_sortConditions(t *testing.T) {
	for _, tc := range []struct {
		name               string
		conditions         []*elbv2.RuleCondition
		expectedConditions []*elbv2.RuleCondition
	}{
		{
			name: "sort condition values",
			conditions: []*elbv2.RuleCondition{
				{
					Field:  aws.String("path-pattern"),
					Values: aws.StringSlice([]string{"/path2", "/path1"}),
				},
			},
			expectedConditions: []*elbv2.RuleCondition{
				{
					Field:  aws.String("path-pattern"),
					Values: aws.StringSlice([]string{"/path1", "/path2"}),
				},
			},
		},
		{
			name: "sort condition fields",
			conditions: []*elbv2.RuleCondition{
				{
					Field:  aws.String("path-pattern"),
					Values: aws.StringSlice([]string{"/path"}),
				},
				{
					Field:  aws.String("host-header"),
					Values: aws.StringSlice([]string{"hostname"}),
				},
			},
			expectedConditions: []*elbv2.RuleCondition{
				{
					Field:  aws.String("host-header"),
					Values: aws.StringSlice([]string{"hostname"}),
				},
				{
					Field:  aws.String("path-pattern"),
					Values: aws.StringSlice([]string{"/path"}),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sortConditions(tc.conditions)
			assert.Equal(t, tc.expectedConditions, tc.conditions)
		})
	}
}

func Test_sortActions(t *testing.T) {
	for _, tc := range []struct {
		name            string
		actions         []*elbv2.Action
		expectedActions []*elbv2.Action
	}{
		{
			name: "sort based on action order",
			actions: []*elbv2.Action{
				{
					Order: aws.Int64(3),
				},
				{
					Order: aws.Int64(1),
				},
				{
					Order: aws.Int64(2),
				},
			},
			expectedActions: []*elbv2.Action{
				{
					Order: aws.Int64(1),
				},
				{
					Order: aws.Int64(2),
				},
				{
					Order: aws.Int64(3),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sortActions(tc.actions)
			assert.Equal(t, tc.expectedActions, tc.actions)
		})
	}
}
