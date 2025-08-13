package routeutils

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_buildHttpRedirectAction(t *testing.T) {
	scheme := "https"
	expectedScheme := "HTTPS"
	invalidScheme := "invalid"

	port := int32(80)
	portString := "80"
	statusCode := 301
	query := "test-query"
	replaceFullPath := "/new-path"
	replacePrefixPath := "/new-prefix-path"
	replacePrefixPathAfterProcessing := "/new-prefix-path/*"
	invalidPath := "/invalid-path*"

	tests := []struct {
		name           string
		filter         *gwv1.HTTPRequestRedirectFilter
		redirectConfig *elbv2gw.RedirectActionConfig
		want           []elbv2model.Action
		wantErr        bool
	}{
		{
			name: "redirect with all fields provided",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme:     &scheme,
				Hostname:   (*gwv1.PreciseHostname)(&hostname),
				Port:       (*gwv1.PortNumber)(&port),
				StatusCode: &statusCode,
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: &replaceFullPath,
				},
			},
			redirectConfig: &elbv2gw.RedirectActionConfig{
				Query: &query,
			},
			want: []elbv2model.Action{
				{
					Type: elbv2model.ActionTypeRedirect,
					RedirectConfig: &elbv2model.RedirectActionConfig{
						Host:       &hostname,
						Path:       &replaceFullPath,
						Port:       &portString,
						Protocol:   &expectedScheme,
						StatusCode: "HTTP_301",
						Query:      &query,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "redirect with prefix match",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: &replacePrefixPath,
				},
			},
			want: []elbv2model.Action{
				{
					Type: elbv2model.ActionTypeRedirect,
					RedirectConfig: &elbv2model.RedirectActionConfig{
						Path: &replacePrefixPathAfterProcessing,
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "redirect with no component provided",
			filter:  &gwv1.HTTPRequestRedirectFilter{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid scheme provided",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Scheme: &invalidScheme,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "path with wildcards in ReplaceFullPath",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:            gwv1.FullPathHTTPPathModifier,
					ReplaceFullPath: &invalidPath,
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "path with wildcards in ReplacePrefixMatch",
			filter: &gwv1.HTTPRequestRedirectFilter{
				Path: &gwv1.HTTPPathModifier{
					Type:               gwv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: &invalidPath,
				},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildHttpRedirectAction(tt.filter, tt.redirectConfig)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_BuildHttpRuleRedirectActionsBasedOnFilter(t *testing.T) {
	query := "test-query"

	redirectConfig := &elbv2gw.RedirectActionConfig{
		Query: &query,
	}

	tests := []struct {
		name           string
		filters        []gwv1.HTTPRouteFilter
		redirectConfig *elbv2gw.RedirectActionConfig
		wantErr        bool
		errContains    string
	}{
		{
			name: "request redirect filter",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
						Port: (*gwv1.PortNumber)(awssdk.Int32(80)),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "unsupported filter type",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterRequestHeaderModifier,
				},
			},
			wantErr:     true,
			errContains: "Unsupported filter type",
		},
		{
			name: "single ExtensionRef filter with redirectConfig should error",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterExtensionRef,
					ExtensionRef: &gwv1.LocalObjectReference{
						Kind: "ListenerRuleConfiguration",
						Name: "test-config",
					},
				},
			},
			redirectConfig: redirectConfig,
			wantErr:        true,
			errContains:    "HTTPRouteFilterRequestRedirect must be provided if RedirectActionConfig in ListenerRuleConfiguration is provided",
		},
		{
			name:           "empty filters should return nil",
			filters:        []gwv1.HTTPRouteFilter{},
			redirectConfig: nil,
			wantErr:        false,
		},
		{
			name: "multiple filters with ExtensionRef should continue processing",
			filters: []gwv1.HTTPRouteFilter{
				{
					Type: gwv1.HTTPRouteFilterExtensionRef,
					ExtensionRef: &gwv1.LocalObjectReference{
						Kind: "SomeOtherConfig",
						Name: "test-config",
					},
				},
				{
					Type: gwv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
						Hostname: (*gwv1.PreciseHostname)(awssdk.String("redirect.example.com")),
					},
				},
			},
			redirectConfig: nil,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := buildHttpRuleRedirectActionsBasedOnFilter(tt.filters, tt.redirectConfig)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, actions)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_buildFixedResponseRoutingActions(t *testing.T) {
	contentType := "text/plain"
	messageBody := "test-message-body"

	tests := []struct {
		name                string
		fixedResponseConfig *elbv2gw.FixedResponseActionConfig
		want                []elbv2model.Action
		wantErr             bool
	}{
		{
			name: "fixed response with all fields",
			fixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
				StatusCode:  404,
				ContentType: &contentType,
				MessageBody: &messageBody,
			},
			want: []elbv2model.Action{
				{
					Type: elbv2model.ActionTypeFixedResponse,
					FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
						StatusCode:  "404",
						ContentType: &contentType,
						MessageBody: &messageBody,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fixed response with only status code",
			fixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
				StatusCode: 503,
			},
			want: []elbv2model.Action{
				{
					Type: elbv2model.ActionTypeFixedResponse,
					FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
						StatusCode:  "503",
						ContentType: nil,
						MessageBody: nil,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fixed response with status code and content type only",
			fixedResponseConfig: &elbv2gw.FixedResponseActionConfig{
				StatusCode:  200,
				ContentType: &contentType,
			},
			want: []elbv2model.Action{
				{
					Type: elbv2model.ActionTypeFixedResponse,
					FixedResponseConfig: &elbv2model.FixedResponseActionConfig{
						StatusCode:  "200",
						ContentType: &contentType,
						MessageBody: nil,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildFixedResponseRoutingActions(tt.fixedResponseConfig)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
