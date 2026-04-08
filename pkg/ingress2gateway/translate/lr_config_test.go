package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gwconstants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestBuildListenerRuleConfiguration(t *testing.T) {
	lrc := buildListenerRuleConfiguration("my-ns", "my-ing", "my-action")

	assert.Equal(t, utils.LBConfigAPIVersion, lrc.APIVersion)
	assert.Equal(t, gwconstants.ListenerRuleConfiguration, lrc.Kind)
	assert.Equal(t, "my-ns", lrc.Namespace)
	assert.Equal(t, utils.GetLRConfigName("my-ns", "my-ing", "my-action"), lrc.Name)
	assert.Nil(t, lrc.Spec.Tags)
}

func TestExtensionRefFilter(t *testing.T) {
	filter := extensionRefFilter("my-lrc")

	assert.Equal(t, gwv1.HTTPRouteFilterExtensionRef, filter.Type)
	require.NotNil(t, filter.ExtensionRef)
	assert.Equal(t, gwv1.Group(utils.LBCGatewayAPIGroup), filter.ExtensionRef.Group)
	assert.Equal(t, gwv1.Kind(gwconstants.ListenerRuleConfiguration), filter.ExtensionRef.Kind)
	assert.Equal(t, gwv1.ObjectName("my-lrc"), filter.ExtensionRef.Name)
}
