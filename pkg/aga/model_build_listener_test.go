package aga

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

func TestDefaultListenerBuilder_Build(t *testing.T) {
	// Protocol references for direct pointer usage
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP

	tests := []struct {
		name          string
		listeners     []agaapi.GlobalAcceleratorListener
		wantListeners int
		wantErr       bool
	}{
		{
			name:          "with nil listeners",
			listeners:     nil,
			wantListeners: 0,
			wantErr:       false,
		},
		{
			name:          "with empty listeners",
			listeners:     []agaapi.GlobalAcceleratorListener{},
			wantListeners: 0,
			wantErr:       false,
		},
		{
			name: "with single TCP listener",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					Protocol: &protocolTCP,
					PortRanges: &[]agaapi.PortRange{
						{
							FromPort: 80,
							ToPort:   80,
						},
					},
					ClientAffinity: agaapi.ClientAffinityNone,
				},
			},
			wantListeners: 1,
			wantErr:       false,
		},
		{
			name: "with single UDP listener",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					Protocol: &protocolUDP,
					PortRanges: &[]agaapi.PortRange{
						{
							FromPort: 53,
							ToPort:   53,
						},
					},
					ClientAffinity: agaapi.ClientAffinitySourceIP,
				},
			},
			wantListeners: 1,
			wantErr:       false,
		},
		{
			name: "with multiple listeners",
			listeners: []agaapi.GlobalAcceleratorListener{
				{
					Protocol: &protocolTCP,
					PortRanges: &[]agaapi.PortRange{
						{
							FromPort: 80,
							ToPort:   80,
						},
					},
					ClientAffinity: agaapi.ClientAffinityNone,
				},
				{
					Protocol: &protocolUDP,
					PortRanges: &[]agaapi.PortRange{
						{
							FromPort: 53,
							ToPort:   53,
						},
					},
					ClientAffinity: agaapi.ClientAffinitySourceIP,
				},
			},
			wantListeners: 2,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test context
			ctx := context.Background()
			stack := core.NewDefaultStack(core.StackID{Namespace: "test-ns", Name: "test-name"})
			accelerator := createTestAccelerator(stack)

			// Create listener builder and build listeners
			builder := NewListenerBuilder()
			listeners, err := builder.Build(ctx, stack, accelerator, tt.listeners)

			// Check results
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.wantListeners == 0 {
					assert.Nil(t, listeners)
				} else {
					assert.Equal(t, tt.wantListeners, len(listeners))
				}
			}
		})
	}
}

func TestDefaultListenerBuilder_buildListenerSpec(t *testing.T) {
	// Protocol references for direct pointer usage
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP

	// Setup test context
	ctx := context.Background()
	stack := core.NewDefaultStack(core.StackID{Namespace: "test-ns", Name: "test-name"})
	accelerator := createTestAccelerator(stack)

	tests := []struct {
		name         string
		listener     agaapi.GlobalAcceleratorListener
		wantProtocol agamodel.Protocol
		wantAffinity agamodel.ClientAffinity
		wantPorts    []agamodel.PortRange
		wantErr      bool
	}{
		{
			name: "with TCP protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &protocolTCP,
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 80,
						ToPort:   80,
					},
				},
				ClientAffinity: agaapi.ClientAffinityNone,
			},
			wantProtocol: agamodel.ProtocolTCP,
			wantAffinity: agamodel.ClientAffinityNone,
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
			},
			wantErr: false,
		},
		{
			name: "with UDP protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &protocolUDP,
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 53,
						ToPort:   53,
					},
				},
				ClientAffinity: agaapi.ClientAffinitySourceIP,
			},
			wantProtocol: agamodel.ProtocolUDP,
			wantAffinity: agamodel.ClientAffinitySourceIP,
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 53,
					ToPort:   53,
				},
			},
			wantErr: false,
		},
		{
			name: "with nil protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: nil,
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 80,
						ToPort:   80,
					},
				},
				ClientAffinity: agaapi.ClientAffinityNone,
			},
			wantProtocol: "",
			wantAffinity: agamodel.ClientAffinityNone,
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
			},
			wantErr: true,
		},
		{
			name: "with nil port ranges",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol:       &protocolTCP,
				PortRanges:     nil,
				ClientAffinity: agaapi.ClientAffinityNone,
			},
			wantProtocol: agamodel.ProtocolTCP,
			wantAffinity: agamodel.ClientAffinityNone,
			wantPorts:    []agamodel.PortRange{},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Build listener spec
			spec, err := buildListenerSpec(ctx, accelerator, tt.listener)

			// Check results
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantProtocol, spec.Protocol)
				assert.Equal(t, tt.wantAffinity, spec.ClientAffinity)
				assert.Equal(t, tt.wantPorts, spec.PortRanges)
				// AcceleratorARN is a token that will be resolved later, not a direct string
				assert.NotNil(t, spec.AcceleratorARN)
			}
		})
	}
}

// Helper function to create a test accelerator
func createTestAccelerator(stack core.Stack) *agamodel.Accelerator {
	spec := agamodel.AcceleratorSpec{
		Name:    "test-accelerator",
		Enabled: awssdk.Bool(true),
		Tags:    map[string]string{"Key": "Value"},
	}

	accelerator := agamodel.NewAccelerator(stack, "test-accelerator", spec, &agaapi.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ga",
			Namespace: "default",
		},
	})

	// Set the accelerator status to simulate it being fulfilled
	accelerator.SetStatus(agamodel.AcceleratorStatus{
		AcceleratorARN: "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
		DNSName:        "a1234abcd5678efghi.awsglobalaccelerator.com",
		Status:         "DEPLOYED",
	})

	return accelerator
}

func TestBuildListenerProtocol(t *testing.T) {
	// Protocol references for direct pointer usage
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP
	invalidProtocol := agaapi.GlobalAcceleratorProtocol("INVALID")

	tests := []struct {
		name          string
		listener      agaapi.GlobalAcceleratorListener
		wantProtocol  agamodel.Protocol
		wantErr       bool
		wantErrString string
	}{
		{
			name: "with nil protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: nil,
			},
			wantProtocol: "",
			wantErr:      true,
		},
		{
			name: "with TCP protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &protocolTCP,
			},
			wantProtocol: agamodel.ProtocolTCP,
			wantErr:      false,
		},
		{
			name: "with UDP protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &protocolUDP,
			},
			wantProtocol: agamodel.ProtocolUDP,
			wantErr:      false,
		},
		{
			name: "with invalid protocol",
			listener: agaapi.GlobalAcceleratorListener{
				Protocol: &invalidProtocol,
			},
			wantProtocol:  "",
			wantErr:       true,
			wantErrString: "unsupported protocol: INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test context
			ctx := context.Background()

			// Call function
			protocol, err := buildListenerProtocol(ctx, tt.listener)

			// Check results
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrString != "" {
					assert.Contains(t, err.Error(), tt.wantErrString)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantProtocol, protocol)
			}
		})
	}
}

func TestBuildListenerPortRanges(t *testing.T) {
	tests := []struct {
		name      string
		listener  agaapi.GlobalAcceleratorListener
		wantPorts []agamodel.PortRange
		wantErr   bool
	}{
		{
			name: "with nil port ranges",
			listener: agaapi.GlobalAcceleratorListener{
				PortRanges: nil,
			},
			wantPorts: []agamodel.PortRange{},
			wantErr:   true,
		},
		{
			name: "with single port range",
			listener: agaapi.GlobalAcceleratorListener{
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 443,
						ToPort:   443,
					},
				},
			},
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 443,
					ToPort:   443,
				},
			},
			wantErr: false,
		},
		{
			name: "with multiple port ranges",
			listener: agaapi.GlobalAcceleratorListener{
				PortRanges: &[]agaapi.PortRange{
					{
						FromPort: 80,
						ToPort:   80,
					},
					{
						FromPort: 443,
						ToPort:   443,
					},
					{
						FromPort: 8080,
						ToPort:   8090,
					},
				},
			},
			wantPorts: []agamodel.PortRange{
				{
					FromPort: 80,
					ToPort:   80,
				},
				{
					FromPort: 443,
					ToPort:   443,
				},
				{
					FromPort: 8080,
					ToPort:   8090,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test context
			ctx := context.Background()

			// Call function
			portRanges, err := buildListenerPortRanges(ctx, tt.listener)

			// Check results
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPorts, portRanges)
			}
		})
	}
}

func TestBuildListenerClientAffinity(t *testing.T) {
	tests := []struct {
		name         string
		listener     agaapi.GlobalAcceleratorListener
		wantAffinity agamodel.ClientAffinity
	}{
		{
			name: "with NONE client affinity",
			listener: agaapi.GlobalAcceleratorListener{
				ClientAffinity: agaapi.ClientAffinityNone,
			},
			wantAffinity: agamodel.ClientAffinityNone,
		},
		{
			name: "with SOURCE_IP client affinity",
			listener: agaapi.GlobalAcceleratorListener{
				ClientAffinity: agaapi.ClientAffinitySourceIP,
			},
			wantAffinity: agamodel.ClientAffinitySourceIP,
		},
		{
			name: "with invalid client affinity (should default to NONE)",
			listener: agaapi.GlobalAcceleratorListener{
				ClientAffinity: "INVALID",
			},
			wantAffinity: agamodel.ClientAffinityNone,
		},
		{
			name: "with empty client affinity (should default to NONE)",
			listener: agaapi.GlobalAcceleratorListener{
				ClientAffinity: "",
			},
			wantAffinity: agamodel.ClientAffinityNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test context
			ctx := context.Background()

			// Call function
			clientAffinity := buildListenerClientAffinity(ctx, tt.listener)

			// Check results
			assert.Equal(t, tt.wantAffinity, clientAffinity)
		})
	}
}
