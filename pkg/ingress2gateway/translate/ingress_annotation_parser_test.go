package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
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
			},
		},
		{
			name: "forward multiple TGs with stickiness (advanced schema)",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/actions.forward-multiple-tg": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"service-1","servicePort":"http","weight":20},{"serviceName":"service-2","servicePort":80,"weight":20},{"targetGroupARN":"arn-of-tg","weight":60}],"targetGroupStickinessConfig":{"enabled":true,"durationSeconds":200}}}`,
			},
			svcName: "forward-multiple-tg",
			check: func(t *testing.T, a *ingress.Action) {
				assert.Equal(t, ingress.ActionTypeForward, a.Type)
				require.NotNil(t, a.ForwardConfig)
				require.Len(t, a.ForwardConfig.TargetGroups, 3)
				require.NotNil(t, a.ForwardConfig.TargetGroupStickinessConfig)
				assert.Equal(t, true, *a.ForwardConfig.TargetGroupStickinessConfig.Enabled)
			},
		},
		{
			name: "servicePort string is normalized to int (backwards compat)",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/actions.forward-str-port": `{"type":"forward","forwardConfig":{"targetGroups":[{"serviceName":"svc","servicePort":"80"}]}}`,
			},
			svcName: "forward-str-port",
			check: func(t *testing.T, a *ingress.Action) {
				assert.Equal(t, ingress.ActionTypeForward, a.Type)
				require.NotNil(t, a.ForwardConfig)
				require.Len(t, a.ForwardConfig.TargetGroups, 1)
				require.NotNil(t, a.ForwardConfig.TargetGroups[0].ServicePort)
				// "80" string should be normalized to int 80
				assert.Equal(t, int32(80), int32(a.ForwardConfig.TargetGroups[0].ServicePort.IntValue()))
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

func TestParseListenPorts(t *testing.T) {
	tests := []struct {
		name    string
		annos   map[string]string
		want    []listenPortEntry
		wantErr bool
	}{
		{
			name:  "not present",
			annos: map[string]string{},
			want:  nil,
		},
		{
			name: "single HTTP",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP": 80}]`,
			},
			want: []listenPortEntry{{Protocol: "HTTP", Port: 80}},
		},
		{
			name: "multiple ports",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP": 80}, {"HTTPS": 443}]`,
			},
			want: []listenPortEntry{
				{Protocol: "HTTP", Port: 80},
				{Protocol: "HTTPS", Port: 443},
			},
		},
		{
			name: "invalid JSON",
			annos: map[string]string{
				"alb.ingress.kubernetes.io/listen-ports": `invalid`,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseListenPorts(tt.annos)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}
