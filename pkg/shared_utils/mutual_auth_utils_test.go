package shared_utils

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
)

func Test_validateMutualAuthenticationConfig(t *testing.T) {
	tests := []struct {
		name                 string
		protocol             elbv2model.Protocol
		port                 int32
		mode                 string
		trustStoreARN        string
		ignoreClientCert     *bool
		advertiseCANames     *string
		expectedErrorMessage *string
	}{
		{
			name:     "happy path no validation error off mode",
			protocol: elbv2model.ProtocolHTTPS,
			port:     800,
			mode:     string(elbv2model.MutualAuthenticationOffMode),
		},
		{
			name:     "happy path no validation error pass through mode",
			protocol: elbv2model.ProtocolHTTPS,
			port:     800,
			mode:     string(elbv2model.MutualAuthenticationPassthroughMode),
		},
		{
			name:          "happy path no validation error verify mode",
			protocol:      elbv2model.ProtocolHTTPS,
			port:          800,
			mode:          string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN: "truststore",
		},
		{
			name:             "happy path no validation error verify mode, with ignore client cert expiry",
			protocol:         elbv2model.ProtocolHTTPS,
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			ignoreClientCert: awssdk.Bool(true),
		},
		{
			name:             "happy path no validation error verify mode, with ignore client cert expiry false",
			protocol:         elbv2model.ProtocolHTTPS,
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			ignoreClientCert: awssdk.Bool(false),
		},
		{
			name:             "happy path no validation error verify mode, with advertise ca on",
			protocol:         elbv2model.ProtocolHTTPS,
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			advertiseCANames: awssdk.String("on"),
		},
		{
			name:             "happy path no validation error verify mode, with advertise ca off",
			protocol:         elbv2model.ProtocolHTTPS,
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			advertiseCANames: awssdk.String("off"),
		},
		{
			name:                 "no mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			expectedErrorMessage: awssdk.String("mutualAuthentication mode cannot be empty for port 800"),
		},
		{
			name:     "unknown mode",
			protocol: elbv2model.ProtocolHTTPS,

			port:                 800,
			mode:                 "foo",
			expectedErrorMessage: awssdk.String("mutualAuthentication mode value must be among"),
		},
		{
			name:                 "port invalid",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800000,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			expectedErrorMessage: awssdk.String("listen port must be within [1, 65535]: 800000"),
		},
		{
			name:                 "missing truststore arn for verify",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationVerifyMode),
			expectedErrorMessage: awssdk.String("trustStore is required when mutualAuthentication mode is verify for port 800"),
		},
		{
			name:                 "truststore arn set but mode not verify",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			trustStoreARN:        "truststore",
			expectedErrorMessage: awssdk.String("Mutual Authentication mode off does not support trustStore for port 800"),
		},
		{
			name:                 "ignore client cert expiry set for off mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			ignoreClientCert:     awssdk.Bool(true),
			expectedErrorMessage: awssdk.String("Mutual Authentication mode off does not support ignoring client certificate expiry for port 800"),
		},
		{
			name:                 "ignore client cert expiry set for passthrough mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationPassthroughMode),
			ignoreClientCert:     awssdk.Bool(true),
			expectedErrorMessage: awssdk.String("Mutual Authentication mode passthrough does not support ignoring client certificate expiry for port 800"),
		},
		{
			name:                 "advertise ca set for off mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			advertiseCANames:     awssdk.String("on"),
			expectedErrorMessage: awssdk.String("Authentication mode off does not support advertiseTrustStoreCaNames for port 800"),
		},
		{
			name:                 "advertise ca set for passthrough mode",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationPassthroughMode),
			advertiseCANames:     awssdk.String("on"),
			expectedErrorMessage: awssdk.String("Authentication mode passthrough does not support advertiseTrustStoreCaNames for port 800"),
		},
		{
			name:                 "advertise ca set with invalid value",
			protocol:             elbv2model.ProtocolHTTPS,
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:        "truststore",
			advertiseCANames:     awssdk.String("foo"),
			expectedErrorMessage: awssdk.String("advertiseTrustStoreCaNames only supports the values \"on\" and \"off\" got value foo for port 800"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := validateMutualAuthenticationConfig(tt.protocol, tt.port, tt.mode, tt.trustStoreARN, tt.ignoreClientCert, tt.advertiseCANames)

			if tt.expectedErrorMessage == nil {
				assert.Nil(t, res)
			} else {
				assert.Contains(t, res.Error(), *tt.expectedErrorMessage)
			}
		})
	}
}
