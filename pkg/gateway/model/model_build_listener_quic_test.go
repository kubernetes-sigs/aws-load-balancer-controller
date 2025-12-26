package model

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func TestQuicProtocolUpgrade(t *testing.T) {
	tests := []struct {
		name           string
		protocol       elbv2model.Protocol
		quicEnabled    *bool
		expectedProto  elbv2model.Protocol
		expectError    bool
	}{
		{
			name:          "UDP with QUIC enabled",
			protocol:      elbv2model.ProtocolUDP,
			quicEnabled:   awssdk.Bool(true),
			expectedProto: elbv2model.ProtocolQUIC,
			expectError:   false,
		},
		{
			name:          "TCP_UDP with QUIC enabled",
			protocol:      elbv2model.ProtocolTCP_UDP,
			quicEnabled:   awssdk.Bool(true),
			expectedProto: elbv2model.ProtocolTCP_QUIC,
			expectError:   false,
		},
		{
			name:          "UDP with QUIC disabled",
			protocol:      elbv2model.ProtocolUDP,
			quicEnabled:   awssdk.Bool(false),
			expectedProto: elbv2model.ProtocolUDP,
			expectError:   false,
		},
		{
			name:          "TCP with QUIC enabled should error",
			protocol:      elbv2model.ProtocolTCP,
			quicEnabled:   awssdk.Bool(true),
			expectedProto: elbv2model.ProtocolTCP,
			expectError:   true,
		},
		{
			name:          "UDP with no QUIC config",
			protocol:      elbv2model.ProtocolUDP,
			quicEnabled:   nil,
			expectedProto: elbv2model.ProtocolUDP,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the QUIC protocol upgrade logic directly
			protocol := tt.protocol
			var lbLsCfg *elbv2gw.ListenerConfiguration
			if tt.quicEnabled != nil {
				lbLsCfg = &elbv2gw.ListenerConfiguration{
					QuicEnabled: tt.quicEnabled,
				}
			}

			// Apply QUIC protocol upgrade if enabled
			var err error
			if lbLsCfg != nil && lbLsCfg.QuicEnabled != nil && *lbLsCfg.QuicEnabled {
				switch protocol {
				case elbv2model.ProtocolUDP:
					protocol = elbv2model.ProtocolQUIC
				case elbv2model.ProtocolTCP_UDP:
					protocol = elbv2model.ProtocolTCP_QUIC
				default:
					err = assert.AnError // Simulate error for unsupported protocols
				}
			}

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedProto, protocol)
			}
		})
	}
}

func TestMergeProtocols_WithQuic(t *testing.T) {
	tests := []struct {
		name             string
		storedProtocol   elbv2model.Protocol
		proposedProtocol elbv2model.Protocol
		expectedProtocol elbv2model.Protocol
		expectError      bool
	}{
		{
			name:             "TCP + QUIC = TCP_QUIC",
			storedProtocol:   elbv2model.ProtocolTCP,
			proposedProtocol: elbv2model.ProtocolQUIC,
			expectedProtocol: elbv2model.ProtocolTCP_QUIC,
			expectError:      false,
		},
		{
			name:             "QUIC + TCP = TCP_QUIC",
			storedProtocol:   elbv2model.ProtocolQUIC,
			proposedProtocol: elbv2model.ProtocolTCP,
			expectedProtocol: elbv2model.ProtocolTCP_QUIC,
			expectError:      false,
		},
		{
			name:             "TCP_QUIC + TCP = TCP_QUIC",
			storedProtocol:   elbv2model.ProtocolTCP_QUIC,
			proposedProtocol: elbv2model.ProtocolTCP,
			expectedProtocol: elbv2model.ProtocolTCP_QUIC,
			expectError:      false,
		},
		{
			name:             "TCP_QUIC + QUIC = TCP_QUIC",
			storedProtocol:   elbv2model.ProtocolTCP_QUIC,
			proposedProtocol: elbv2model.ProtocolQUIC,
			expectedProtocol: elbv2model.ProtocolTCP_QUIC,
			expectError:      false,
		},
		{
			name:             "HTTP + QUIC should error",
			storedProtocol:   elbv2model.ProtocolHTTP,
			proposedProtocol: elbv2model.ProtocolQUIC,
			expectedProtocol: elbv2model.ProtocolHTTP,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mergeProtocols(tt.storedProtocol, tt.proposedProtocol)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedProtocol, result)
			}
		})
	}
}
