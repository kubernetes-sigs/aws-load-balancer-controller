package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestBuildGatewayClass(t *testing.T) {
	gc := buildGatewayClass()
	assert.Equal(t, utils.GatewayClassName, gc.Name)
	assert.Equal(t, gwv1.GatewayController(gwconstants.ALBGatewayController), gc.Spec.ControllerName)
	assert.Equal(t, utils.GatewayClassKind, gc.Kind)
}

func TestBuildGateway(t *testing.T) {
	tests := []struct {
		name                    string
		gwName                  string
		namespace               string
		lbConfig                *gatewayv1beta1.LoadBalancerConfiguration
		listenPorts             []listenPortEntry
		crossNamespaceGroupName string
		wantListeners           int
		wantParamsRef           bool
		wantAllowedRoutes       bool
	}{
		{
			name:   "with LB config",
			gwName: "my-gw", namespace: "default",
			lbConfig: &gatewayv1beta1.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "my-lb-config", Namespace: "default"},
			},
			listenPorts:   []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantListeners: 1, wantParamsRef: true,
		},
		{
			name:   "without LB config",
			gwName: "bare-gw", namespace: "default",
			listenPorts:   []listenPortEntry{{Protocol: "HTTP", Port: 80}, {Protocol: "HTTPS", Port: 443}},
			wantListeners: 2, wantParamsRef: false,
		},
		{
			name:   "cross-namespace group sets allowedRoutes to All",
			gwName: "gw", namespace: "infra-ns",
			listenPorts:             []listenPortEntry{{Protocol: "HTTP", Port: 80}, {Protocol: "HTTPS", Port: 443}},
			crossNamespaceGroupName: "shared-alb",
			wantListeners:           2,
			wantAllowedRoutes:       true,
		},
		{
			name:   "same-namespace group has no allowedRoutes",
			gwName: "gw", namespace: "ns",
			listenPorts:   []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			wantListeners: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := buildGateway(tt.gwName, tt.namespace, tt.lbConfig, tt.listenPorts, tt.crossNamespaceGroupName)
			assert.Equal(t, tt.gwName, gw.Name)
			require.Len(t, gw.Spec.Listeners, tt.wantListeners)

			if tt.wantParamsRef {
				require.NotNil(t, gw.Spec.Infrastructure)
				assert.Equal(t, tt.lbConfig.Name, gw.Spec.Infrastructure.ParametersRef.Name)
			} else {
				assert.Nil(t, gw.Spec.Infrastructure)
			}

			for _, listener := range gw.Spec.Listeners {
				if tt.wantAllowedRoutes {
					require.NotNil(t, listener.AllowedRoutes, "listener %s should have AllowedRoutes", listener.Name)
					require.NotNil(t, listener.AllowedRoutes.Namespaces)
					require.NotNil(t, listener.AllowedRoutes.Namespaces.From)
					assert.Equal(t, gwv1.NamespacesFromAll, *listener.AllowedRoutes.Namespaces.From)
					assert.Nil(t, listener.AllowedRoutes.Namespaces.Selector)
				} else {
					assert.Nil(t, listener.AllowedRoutes, "listener %s should not have AllowedRoutes", listener.Name)
				}
			}
		})
	}
}

func TestListenerName(t *testing.T) {
	assert.Equal(t, "http-80", utils.GenerateSectionName("HTTP", 80))
	assert.Equal(t, "https-443", utils.GenerateSectionName("HTTPS", 443))
}

func TestToALBProtocol(t *testing.T) {
	assert.Equal(t, gwv1.HTTPProtocolType, toALBProtocol("HTTP"))
	assert.Equal(t, gwv1.HTTPSProtocolType, toALBProtocol("HTTPS"))
	assert.Equal(t, gwv1.HTTPProtocolType, toALBProtocol("UNKNOWN"))
}
