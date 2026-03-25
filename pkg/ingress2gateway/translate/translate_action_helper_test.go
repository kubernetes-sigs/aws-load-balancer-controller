package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestParseActionAnnotation(t *testing.T) {
	tests := []struct {
		name    string
		annos   map[string]string
		svcName string
		wantErr bool
		check   func(t *testing.T, a *ingress.Action)
	}{
		{
			name: "fixed-response action",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/actions.response-503": `{"type":"fixed-response","fixedResponseConfig":{"contentType":"text/plain","statusCode":"503","messageBody":"503 error text"}}`,
			},
			svcName: "response-503",
			check: func(t *testing.T, a *ingress.Action) {
				assert.Equal(t, ingress.ActionTypeFixedResponse, a.Type)
				require.NotNil(t, a.FixedResponseConfig)
				assert.Equal(t, "503", a.FixedResponseConfig.StatusCode)
				assert.Equal(t, "503 error text", *a.FixedResponseConfig.MessageBody)
			},
		},
		{
			name: "redirect action",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/actions.redirect-to-eks": `{"type":"redirect","redirectConfig":{"host":"aws.amazon.com","path":"/eks/","port":"443","protocol":"HTTPS","query":"k=v","statusCode":"HTTP_302"}}`,
			},
			svcName: "redirect-to-eks",
			check: func(t *testing.T, a *ingress.Action) {
				assert.Equal(t, ingress.ActionTypeRedirect, a.Type)
				require.NotNil(t, a.RedirectConfig)
				assert.Equal(t, "aws.amazon.com", *a.RedirectConfig.Host)
				assert.Equal(t, "HTTP_302", a.RedirectConfig.StatusCode)
			},
		},
		{
			name: "forward single TG by ARN (simplified schema)",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/actions.forward-single-tg": `{"type":"forward","targetGroupARN":"arn:aws:elasticloadbalancing:us-west-2:123456789:targetgroup/my-tg/abc123"}`,
			},
			svcName: "forward-single-tg",
			check: func(t *testing.T, a *ingress.Action) {
				assert.Equal(t, ingress.ActionTypeForward, a.Type)
				require.NotNil(t, a.ForwardConfig)
				require.Len(t, a.ForwardConfig.TargetGroups, 1)
				require.NotNil(t, a.ForwardConfig.TargetGroups[0].TargetGroupARN)
				assert.Equal(t, "arn:aws:elasticloadbalancing:us-west-2:123456789:targetgroup/my-tg/abc123", *a.ForwardConfig.TargetGroups[0].TargetGroupARN)
			},
		},
		{
			name: "forward single TG by name (simplified schema)",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/actions.forward-by-name": `{"type":"forward","targetGroupName":"my-target-group"}`,
			},
			svcName: "forward-by-name",
			check: func(t *testing.T, a *ingress.Action) {
				assert.Equal(t, ingress.ActionTypeForward, a.Type)
				require.NotNil(t, a.ForwardConfig)
				require.Len(t, a.ForwardConfig.TargetGroups, 1)
				require.NotNil(t, a.ForwardConfig.TargetGroups[0].TargetGroupName)
				assert.Equal(t, "my-target-group", *a.ForwardConfig.TargetGroups[0].TargetGroupName)
			},
		},
		{
			name: "forward multiple TGs with weights and stickiness (advanced schema)",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/actions.forward-multiple-tg": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"service-1","servicePort":"http","weight":20},{"serviceName":"service-2","servicePort":80,"weight":20},{"targetGroupARN":"arn-of-your-non-k8s-target-group","weight":60}],"targetGroupStickinessConfig":{"enabled":true,"durationSeconds":200}}}`,
			},
			svcName: "forward-multiple-tg",
			check: func(t *testing.T, a *ingress.Action) {
				assert.Equal(t, ingress.ActionTypeForward, a.Type)
				require.NotNil(t, a.ForwardConfig)
				require.Len(t, a.ForwardConfig.TargetGroups, 3)
				require.NotNil(t, a.ForwardConfig.TargetGroupStickinessConfig)
				assert.Equal(t, true, *a.ForwardConfig.TargetGroupStickinessConfig.Enabled)
				assert.Equal(t, int32(200), *a.ForwardConfig.TargetGroupStickinessConfig.DurationSeconds)
			},
		},
		{
			name:    "missing annotation",
			annos:   map[string]string{},
			svcName: "nonexistent",
			wantErr: true,
		},
		{
			name: "invalid JSON",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/actions.bad": `{invalid`,
			},
			svcName: "bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := parseActionAnnotation(tt.annos, tt.svcName)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, a)
		})
	}
}

func TestTranslateAction_FixedResponse(t *testing.T) {
	a := &ingress.Action{
		Type: ingress.ActionTypeFixedResponse,
		FixedResponseConfig: &ingress.FixedResponseActionConfig{
			StatusCode:  "503",
			ContentType: strPtr("text/plain"),
			MessageBody: strPtr("503 error text"),
		},
	}
	result, err := translateAction(a, "default", "test-action", nil)
	require.NoError(t, err)
	require.NotNil(t, result.ListenerRuleConfiguration)
	require.Len(t, result.ListenerRuleConfiguration.Spec.Actions, 1)
	assert.Equal(t, gatewayv1beta1.ActionTypeFixedResponse, result.ListenerRuleConfiguration.Spec.Actions[0].Type)
	assert.Equal(t, int32(503), result.ListenerRuleConfiguration.Spec.Actions[0].FixedResponseConfig.StatusCode)
	require.Len(t, result.Filters, 1)
	assert.Equal(t, gwv1.HTTPRouteFilterExtensionRef, result.Filters[0].Type)
	assert.Empty(t, result.BackendRefs)
}

func TestTranslateAction_Redirect(t *testing.T) {
	a := &ingress.Action{
		Type: ingress.ActionTypeRedirect,
		RedirectConfig: &ingress.RedirectActionConfig{
			Host:       strPtr("aws.amazon.com"),
			Path:       strPtr("/eks/"),
			Port:       strPtr("443"),
			Protocol:   strPtr("HTTPS"),
			StatusCode: "HTTP_302",
		},
	}
	result, err := translateAction(a, "default", "test-action", nil)
	require.NoError(t, err)
	require.Len(t, result.Filters, 1)
	assert.Equal(t, gwv1.HTTPRouteFilterRequestRedirect, result.Filters[0].Type)
	redirect := result.Filters[0].RequestRedirect
	require.NotNil(t, redirect)
	assert.Equal(t, gwv1.PreciseHostname("aws.amazon.com"), *redirect.Hostname)
	assert.Equal(t, "https", *redirect.Scheme)
	assert.Equal(t, 302, *redirect.StatusCode)
	assert.Nil(t, result.ListenerRuleConfiguration)
	assert.Empty(t, result.BackendRefs)
}

func TestTranslateAction_RedirectWithQuery(t *testing.T) {
	query := "k=v"
	a := &ingress.Action{
		Type: ingress.ActionTypeRedirect,
		RedirectConfig: &ingress.RedirectActionConfig{
			Host:       strPtr("example.com"),
			Query:      &query,
			StatusCode: "HTTP_301",
		},
	}
	result, err := translateAction(a, "default", "test-action", nil)
	require.NoError(t, err)
	require.Len(t, result.Filters, 2)
	assert.Equal(t, gwv1.HTTPRouteFilterRequestRedirect, result.Filters[0].Type)
	assert.Equal(t, gwv1.HTTPRouteFilterExtensionRef, result.Filters[1].Type)
	require.NotNil(t, result.ListenerRuleConfiguration)
	require.Len(t, result.ListenerRuleConfiguration.Spec.Actions, 1)
	assert.Equal(t, gatewayv1beta1.ActionTypeRedirect, result.ListenerRuleConfiguration.Spec.Actions[0].Type)
	assert.Equal(t, "k=v", *result.ListenerRuleConfiguration.Spec.Actions[0].RedirectConfig.Query)
}

func TestTranslateAction_ForwardSingleService(t *testing.T) {
	svcName := "my-service"
	port80 := intstr.FromInt32(80)
	weight := int32(1)
	a := &ingress.Action{
		Type: ingress.ActionTypeForward,
		ForwardConfig: &ingress.ForwardActionConfig{
			TargetGroups: []ingress.TargetGroupTuple{
				{ServiceName: &svcName, ServicePort: &port80, Weight: &weight},
			},
		},
	}
	result, err := translateAction(a, "default", "test-action", nil)
	require.NoError(t, err)
	require.Len(t, result.BackendRefs, 1)
	assert.Equal(t, gwv1.ObjectName("my-service"), result.BackendRefs[0].Name)
	assert.Equal(t, gwv1.PortNumber(80), *result.BackendRefs[0].Port)
	assert.Nil(t, result.ListenerRuleConfiguration)
}

func TestTranslateAction_ForwardTargetGroupName(t *testing.T) {
	tgName := "my-external-tg"
	a := &ingress.Action{
		Type: ingress.ActionTypeForward,
		ForwardConfig: &ingress.ForwardActionConfig{
			TargetGroups: []ingress.TargetGroupTuple{
				{TargetGroupName: &tgName},
			},
		},
	}
	result, err := translateAction(a, "default", "test-action", nil)
	require.NoError(t, err)
	require.Len(t, result.BackendRefs, 1)
	assert.Equal(t, gwv1.ObjectName("my-external-tg"), result.BackendRefs[0].Name)
	kind := gwv1.Kind(utils.TargetGroupNameBackendKind)
	assert.Equal(t, &kind, result.BackendRefs[0].Kind)
}

func TestTranslateAction_ForwardTargetGroupARN(t *testing.T) {
	arn := "arn:aws:elasticloadbalancing:us-west-2:123456789:targetgroup/my-tg/abc123"
	a := &ingress.Action{
		Type: ingress.ActionTypeForward,
		ForwardConfig: &ingress.ForwardActionConfig{
			TargetGroups: []ingress.TargetGroupTuple{
				{TargetGroupARN: &arn},
			},
		},
	}
	result, err := translateAction(a, "default", "test-action", nil)
	require.NoError(t, err)
	require.Len(t, result.BackendRefs, 1)
	assert.Equal(t, gwv1.ObjectName("my-tg"), result.BackendRefs[0].Name)
	kind := gwv1.Kind(utils.TargetGroupNameBackendKind)
	assert.Equal(t, &kind, result.BackendRefs[0].Kind)
}

func TestTranslateAction_ForwardWithStickiness(t *testing.T) {
	svc1 := "service-1"
	svc2 := "service-2"
	port80 := intstr.FromInt32(80)
	w20 := int32(20)
	w80 := int32(80)
	enabled := true
	duration := int32(200)
	a := &ingress.Action{
		Type: ingress.ActionTypeForward,
		ForwardConfig: &ingress.ForwardActionConfig{
			TargetGroups: []ingress.TargetGroupTuple{
				{ServiceName: &svc1, ServicePort: &port80, Weight: &w20},
				{ServiceName: &svc2, ServicePort: &port80, Weight: &w80},
			},
			TargetGroupStickinessConfig: &ingress.TargetGroupStickinessConfig{
				Enabled:         &enabled,
				DurationSeconds: &duration,
			},
		},
	}
	result, err := translateAction(a, "default", "test-action", nil)
	require.NoError(t, err)
	require.Len(t, result.BackendRefs, 2)
	assert.Equal(t, int32(20), *result.BackendRefs[0].Weight)
	assert.Equal(t, int32(80), *result.BackendRefs[1].Weight)
	require.NotNil(t, result.ListenerRuleConfiguration)
	require.Len(t, result.ListenerRuleConfiguration.Spec.Actions, 1)
	assert.Equal(t, gatewayv1beta1.ActionTypeForward, result.ListenerRuleConfiguration.Spec.Actions[0].Type)
	assert.Equal(t, true, *result.ListenerRuleConfiguration.Spec.Actions[0].ForwardConfig.TargetGroupStickinessConfig.Enabled)
	assert.Equal(t, int32(200), *result.ListenerRuleConfiguration.Spec.Actions[0].ForwardConfig.TargetGroupStickinessConfig.DurationSeconds)
	require.Len(t, result.Filters, 1)
	assert.Equal(t, gwv1.HTTPRouteFilterExtensionRef, result.Filters[0].Type)
}

func TestExtractTGNameFromARN(t *testing.T) {
	tests := []struct {
		arn      string
		expected string
	}{
		{"arn:aws:elasticloadbalancing:us-west-2:123456789:targetgroup/my-tg/abc123", "my-tg"},
		{"arn:aws:elasticloadbalancing:us-east-1:999:targetgroup/another-tg/xyz", "another-tg"},
		{"not-an-arn", "not-an-arn"},
		{"arn:aws:elasticloadbalancing:us-west-2:123:targetgroup/name-only", "name-only"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, extractTGNameFromARN(tt.arn))
	}
}

func TestRedirectStatusCode(t *testing.T) {
	code, err := redirectStatusCode("HTTP_301")
	assert.NoError(t, err)
	assert.Equal(t, 301, code)

	code, err = redirectStatusCode("HTTP_302")
	assert.NoError(t, err)
	assert.Equal(t, 302, code)

	code, err = redirectStatusCode("301")
	assert.NoError(t, err)
	assert.Equal(t, 301, code)

	_, err = redirectStatusCode("unknown")
	assert.Error(t, err)
}

func TestIsUseAnnotation(t *testing.T) {
	assert.True(t, isUseAnnotation("use-annotation"))
	assert.False(t, isUseAnnotation("http"))
	assert.False(t, isUseAnnotation("80"))
	assert.False(t, isUseAnnotation(""))
}

func strPtr(s string) *string { return &s }
