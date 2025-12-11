package aga

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"testing"
)

func Test_generateEndpointKey(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    agaapi.GlobalAcceleratorEndpoint
		gaNamespace string
		want        string
	}{
		{
			name: "endpoint with EndpointID type",
			endpoint: agaapi.GlobalAcceleratorEndpoint{
				Type:       agaapi.GlobalAcceleratorEndpointTypeEndpointID,
				EndpointID: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-alb/1234567890"),
			},
			gaNamespace: "default",
			want:        "EndpointID/arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-alb/1234567890",
		},
		{
			name: "endpoint with Service type and explicit namespace",
			endpoint: agaapi.GlobalAcceleratorEndpoint{
				Type:      agaapi.GlobalAcceleratorEndpointTypeService,
				Namespace: awssdk.String("test-namespace"),
				Name:      awssdk.String("test-service"),
			},
			gaNamespace: "default",
			want:        "Service/test-namespace/test-service",
		},
		{
			name: "endpoint with Service type and default namespace",
			endpoint: agaapi.GlobalAcceleratorEndpoint{
				Type: agaapi.GlobalAcceleratorEndpointTypeService,
				Name: awssdk.String("test-service"),
			},
			gaNamespace: "default",
			want:        "Service/default/test-service",
		},
		{
			name: "endpoint with Ingress type",
			endpoint: agaapi.GlobalAcceleratorEndpoint{
				Type:      agaapi.GlobalAcceleratorEndpointTypeIngress,
				Namespace: awssdk.String("ingress-ns"),
				Name:      awssdk.String("test-ingress"),
			},
			gaNamespace: "default",
			want:        "Ingress/ingress-ns/test-ingress",
		},
		{
			name: "endpoint with Gateway type",
			endpoint: agaapi.GlobalAcceleratorEndpoint{
				Type:      agaapi.GlobalAcceleratorEndpointTypeGateway,
				Namespace: awssdk.String("gateway-ns"),
				Name:      awssdk.String("test-gateway"),
			},
			gaNamespace: "default",
			want:        "Gateway/gateway-ns/test-gateway",
		},
		{
			name: "endpoint with nil name (should still work)",
			endpoint: agaapi.GlobalAcceleratorEndpoint{
				Type:      agaapi.GlobalAcceleratorEndpointTypeService,
				Namespace: awssdk.String("test-namespace"),
				Name:      nil,
			},
			gaNamespace: "default",
			want:        "Service/test-namespace/",
		},
		{
			name: "endpoint with both nil namespace and name",
			endpoint: agaapi.GlobalAcceleratorEndpoint{
				Type:      agaapi.GlobalAcceleratorEndpointTypeService,
				Namespace: nil,
				Name:      nil,
			},
			gaNamespace: "default",
			want:        "Service/default/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateEndpointKey(tt.endpoint, tt.gaNamespace)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultEndpointGroupBuilder_buildEndpointConfigurations(t *testing.T) {
	testLogger := logr.Discard()

	// Create test LoadedEndpoints
	createTestEndpoints := func() []*LoadedEndpoint {
		return []*LoadedEndpoint{
			{
				Type:        agaapi.GlobalAcceleratorEndpointTypeService,
				Name:        "test-service",
				Namespace:   "default",
				Weight:      100,
				ARN:         "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-service/1234567890",
				DNSName:     "test-service.default.svc.cluster.local",
				Status:      EndpointStatusLoaded,
				EndpointRef: &agaapi.GlobalAcceleratorEndpoint{Type: agaapi.GlobalAcceleratorEndpointTypeService, Name: awssdk.String("test-service")},
			},
			{
				Type:        agaapi.GlobalAcceleratorEndpointTypeIngress,
				Name:        "test-ingress",
				Namespace:   "ingress-ns",
				Weight:      200,
				ARN:         "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-ingress/0987654321",
				DNSName:     "test-ingress.example.com",
				Status:      EndpointStatusLoaded,
				EndpointRef: &agaapi.GlobalAcceleratorEndpoint{Type: agaapi.GlobalAcceleratorEndpointTypeIngress, Name: awssdk.String("test-ingress"), Namespace: awssdk.String("ingress-ns")},
			},
			{
				Type:        agaapi.GlobalAcceleratorEndpointTypeGateway,
				Name:        "test-gateway",
				Namespace:   "gateway-ns",
				Weight:      150,
				ARN:         "",
				DNSName:     "",
				Status:      EndpointStatusWarning,
				Error:       fmt.Errorf("gateway not found"),
				Message:     "Gateway resource not found",
				EndpointRef: &agaapi.GlobalAcceleratorEndpoint{Type: agaapi.GlobalAcceleratorEndpointTypeGateway, Name: awssdk.String("test-gateway"), Namespace: awssdk.String("gateway-ns")},
			},
			{
				Type:        agaapi.GlobalAcceleratorEndpointTypeEndpointID,
				Name:        "",
				Namespace:   "",
				Weight:      100,
				ARN:         "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-lb/abcdef1234",
				Status:      EndpointStatusLoaded,
				EndpointRef: &agaapi.GlobalAcceleratorEndpoint{Type: agaapi.GlobalAcceleratorEndpointTypeEndpointID, EndpointID: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-lb/abcdef1234")},
			},
		}
	}

	tests := []struct {
		name            string
		endpointGroup   agaapi.GlobalAcceleratorEndpointGroup
		loadedEndpoints []*LoadedEndpoint
		want            []agamodel.EndpointConfiguration
		wantErr         bool
	}{
		{
			name: "nil endpoints in endpoint group",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Endpoints: nil,
			},
			loadedEndpoints: createTestEndpoints(),
			want:            nil,
			wantErr:         false,
		},
		{
			name: "empty endpoints array in endpoint group",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{},
			},
			loadedEndpoints: createTestEndpoints(),
			want:            []agamodel.EndpointConfiguration{},
			wantErr:         false,
		},
		{
			name: "endpoint with EndpointID reference",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
					{
						Type:                        agaapi.GlobalAcceleratorEndpointTypeEndpointID,
						EndpointID:                  awssdk.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-lb/abcdef1234"),
						ClientIPPreservationEnabled: awssdk.Bool(true),
					},
				},
			},
			loadedEndpoints: createTestEndpoints(),
			want: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/test-lb/abcdef1234",
					Weight:                      awssdk.Int32(100),
					ClientIPPreservationEnabled: awssdk.Bool(true),
				},
			},
			wantErr: false,
		},
		{
			name: "endpoint with Service reference",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
					{
						Type:                        agaapi.GlobalAcceleratorEndpointTypeService,
						Name:                        awssdk.String("test-service"),
						ClientIPPreservationEnabled: awssdk.Bool(false),
					},
				},
			},
			loadedEndpoints: createTestEndpoints(),
			want: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-service/1234567890",
					Weight:                      awssdk.Int32(100),
					ClientIPPreservationEnabled: awssdk.Bool(false),
				},
			},
			wantErr: false,
		},
		{
			name: "endpoint with Ingress reference, no override weight",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
					{
						Type:                        agaapi.GlobalAcceleratorEndpointTypeIngress,
						Namespace:                   awssdk.String("ingress-ns"),
						Name:                        awssdk.String("test-ingress"),
						ClientIPPreservationEnabled: awssdk.Bool(true),
					},
				},
			},
			loadedEndpoints: createTestEndpoints(),
			want: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-ingress/0987654321",
					Weight:                      awssdk.Int32(200), // From the loaded endpoint, no override
					ClientIPPreservationEnabled: awssdk.Bool(true), // From the endpoint definition
				},
			},
			wantErr: false,
		},
		{
			name: "endpoint with warning status - not included",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
					{
						Type:      agaapi.GlobalAcceleratorEndpointTypeGateway,
						Namespace: awssdk.String("gateway-ns"),
						Name:      awssdk.String("test-gateway"),
					},
				},
			},
			loadedEndpoints: createTestEndpoints(),
			want:            []agamodel.EndpointConfiguration{}, // No endpoints should be added
			wantErr:         false,
		},
		{
			name: "endpoint not found in loaded endpoints",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
					{
						Type:      agaapi.GlobalAcceleratorEndpointTypeService,
						Namespace: awssdk.String("non-existent"),
						Name:      awssdk.String("non-existent-service"),
					},
				},
			},
			loadedEndpoints: createTestEndpoints(),
			want:            []agamodel.EndpointConfiguration{}, // No endpoints should be added
			wantErr:         false,
		},
		{
			name: "multiple endpoints with mixed types",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Endpoints: &[]agaapi.GlobalAcceleratorEndpoint{
					{
						Type: agaapi.GlobalAcceleratorEndpointTypeService,
						Name: awssdk.String("test-service"),
					},
					{
						Type:                        agaapi.GlobalAcceleratorEndpointTypeIngress,
						Namespace:                   awssdk.String("ingress-ns"),
						Name:                        awssdk.String("test-ingress"),
						ClientIPPreservationEnabled: awssdk.Bool(true),
					},
					{
						Type:      agaapi.GlobalAcceleratorEndpointTypeGateway,
						Namespace: awssdk.String("gateway-ns"),
						Name:      awssdk.String("test-gateway"), // Has warning status, should be skipped
					},
					{
						Type: agaapi.GlobalAcceleratorEndpointTypeService,
						Name: awssdk.String("non-existent-service"), // Not in loaded endpoints, should be skipped
					},
				},
			},
			loadedEndpoints: createTestEndpoints(),
			want: []agamodel.EndpointConfiguration{
				{
					EndpointID:                  "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-service/1234567890",
					Weight:                      awssdk.Int32(100),
					ClientIPPreservationEnabled: nil,
				},
				{
					EndpointID:                  "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/test-ingress/0987654321",
					Weight:                      awssdk.Int32(200), // From the loaded endpoint
					ClientIPPreservationEnabled: awssdk.Bool(true),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create endpointGroupBuilder
			builder := &defaultEndpointGroupBuilder{
				clusterRegion: "us-west-2",
				gaNamespace:   "default",
				logger:        testLogger,
			}

			// Call buildEndpointConfigurations
			got, err := builder.buildEndpointConfigurations(ctx, tt.endpointGroup, tt.loadedEndpoints)

			// Check for expected error
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.want, got) // Use ElementsMatch to ignore order
			}
		})
	}
}

func Test_defaultEndpointGroupBuilder_determineRegion(t *testing.T) {
	tests := []struct {
		name              string
		endpointGroup     agaapi.GlobalAcceleratorEndpointGroup
		clusterRegion     string
		expectedRegion    string
		expectError       bool
		expectErrorString string
	}{
		{
			name: "region specified in endpoint group",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Region: awssdk.String("us-west-2"),
			},
			clusterRegion:  "us-east-1",
			expectedRegion: "us-west-2",
			expectError:    false,
		},
		{
			name: "region specified in endpoint group even with empty cluster region",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Region: awssdk.String("eu-west-1"),
			},
			clusterRegion:  "",
			expectedRegion: "eu-west-1",
			expectError:    false,
		},
		{
			name:           "region not specified in endpoint group, use cluster region",
			endpointGroup:  agaapi.GlobalAcceleratorEndpointGroup{},
			clusterRegion:  "us-east-1",
			expectedRegion: "us-east-1",
			expectError:    false,
		},
		{
			name:              "neither region specified nor cluster region available",
			endpointGroup:     agaapi.GlobalAcceleratorEndpointGroup{},
			clusterRegion:     "",
			expectError:       true,
			expectErrorString: "region is required for endpoint group but neither specified in the endpoint group nor available from cluster configuration",
		},
		{
			name: "empty region string in endpoint group, fall back to cluster region",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Region: awssdk.String(""),
			},
			clusterRegion:  "ap-southeast-1",
			expectedRegion: "ap-southeast-1",
			expectError:    false,
		},
		{
			name: "empty region string in endpoint group and no cluster region",
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				Region: awssdk.String(""),
			},
			clusterRegion:     "",
			expectError:       true,
			expectErrorString: "region is required for endpoint group but neither specified in the endpoint group nor available from cluster configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create endpointGroupBuilder
			builder := &defaultEndpointGroupBuilder{
				clusterRegion: tt.clusterRegion,
			}

			// Call determineRegion
			region, err := builder.determineRegion(tt.endpointGroup)

			// Check if error was expected
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectErrorString != "" {
					assert.Contains(t, err.Error(), tt.expectErrorString)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRegion, region)
			}
		})
	}
}

func Test_defaultEndpointGroupBuilder_buildPortOverrides(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	// Helper function to create a listener with specific ID and port ranges
	createTestListener := func(id string, portRanges []agamodel.PortRange) *agamodel.Listener {
		return &agamodel.Listener{
			ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", id),
			Spec: agamodel.ListenerSpec{
				PortRanges: portRanges,
			},
		}
	}

	// Helper function to create port overrides
	createPortOverrides := func(overrides ...agaapi.PortOverride) *[]agaapi.PortOverride {
		if len(overrides) == 0 {
			return nil
		}
		return &overrides
	}

	tests := []struct {
		name           string
		listener       *agamodel.Listener
		endpointGroup  agaapi.GlobalAcceleratorEndpointGroup
		want           []agamodel.PortOverride
		expectErr      bool
		expectErrMatch string
	}{
		{
			name: "no port overrides",
			listener: createTestListener("listener-1", []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
			}),
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				PortOverrides: nil,
			},
			want:      nil,
			expectErr: false,
		},
		{
			name: "empty port overrides",
			listener: createTestListener("listener-1", []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
			}),
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				PortOverrides: &[]agaapi.PortOverride{},
			},
			want:      nil,
			expectErr: false,
		},
		{
			name: "valid single port override",
			listener: createTestListener("listener-1", []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
			}),
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				PortOverrides: createPortOverrides(
					agaapi.PortOverride{
						ListenerPort: 80,
						EndpointPort: 8080,
					},
				),
			},
			want: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
			},
			expectErr: false,
		},
		{
			name: "valid multiple port overrides",
			listener: createTestListener("listener-1", []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			}),
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				PortOverrides: createPortOverrides(
					agaapi.PortOverride{
						ListenerPort: 80,
						EndpointPort: 8080,
					},
					agaapi.PortOverride{
						ListenerPort: 443,
						EndpointPort: 8443,
					},
				),
			},
			want: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
				{
					ListenerPort: 443,
					EndpointPort: 8443,
				},
			},
			expectErr: false,
		},
		{
			name: "listener port outside range",
			listener: createTestListener("listener-1", []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
			}),
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				PortOverrides: createPortOverrides(
					agaapi.PortOverride{
						ListenerPort: 443, // Not in listener port range
						EndpointPort: 8443,
					},
				),
			},
			want:           nil,
			expectErr:      true,
			expectErrMatch: "port override listener port 443 is not within any listener port ranges",
		},
		{
			name: "duplicate listener port",
			listener: createTestListener("listener-1", []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			}),
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				PortOverrides: createPortOverrides(
					agaapi.PortOverride{
						ListenerPort: 80,
						EndpointPort: 8080,
					},
					agaapi.PortOverride{
						ListenerPort: 80, // Duplicate listener port
						EndpointPort: 9090,
					},
				),
			},
			want:           nil,
			expectErr:      true,
			expectErrMatch: "duplicate listener port 80 in port overrides",
		},
		{
			name: "duplicate endpoint port",
			listener: createTestListener("listener-1", []agamodel.PortRange{
				{FromPort: 80, ToPort: 80},
				{FromPort: 443, ToPort: 443},
			}),
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				PortOverrides: createPortOverrides(
					agaapi.PortOverride{
						ListenerPort: 80,
						EndpointPort: 8080,
					},
					agaapi.PortOverride{
						ListenerPort: 443,
						EndpointPort: 8080, // Duplicate endpoint port
					},
				),
			},
			want:           nil,
			expectErr:      true,
			expectErrMatch: "duplicate endpoint port 8080 in port overrides",
		},
		{
			name: "port range check for listener port",
			listener: createTestListener("listener-1", []agamodel.PortRange{
				{FromPort: 80, ToPort: 90},
			}),
			endpointGroup: agaapi.GlobalAcceleratorEndpointGroup{
				PortOverrides: createPortOverrides(
					agaapi.PortOverride{
						ListenerPort: 85, // Within the range
						EndpointPort: 8085,
					},
				),
			},
			want: []agamodel.PortOverride{
				{
					ListenerPort: 85,
					EndpointPort: 8085,
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create endpointGroupBuilder
			builder := &defaultEndpointGroupBuilder{
				clusterRegion: "us-west-2",
			}

			// Call buildPortOverrides
			got, err := builder.buildPortOverrides(ctx, tt.listener, tt.endpointGroup)

			// Check for expected error
			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectErrMatch != "" {
					assert.Contains(t, err.Error(), tt.expectErrMatch)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultEndpointGroupBuilder_validateEndpointPortOverridesWithinListener(t *testing.T) {
	tests := []struct {
		name               string
		endpointPort       int32
		listenerPortRanges map[string][]agamodel.PortRange
		wantErr            bool
		expectErrContains  string
	}{
		{
			name:         "endpoint port outside all listener port ranges",
			endpointPort: 8080,
			listenerPortRanges: map[string][]agamodel.PortRange{
				"l-1": {
					{
						FromPort: 80,
						ToPort:   80,
					},
				},
				"l-2": {
					{
						FromPort: 443,
						ToPort:   443,
					},
				},
			},
			wantErr: false,
		},
		{
			name:         "endpoint port inside a listener port range",
			endpointPort: 450,
			listenerPortRanges: map[string][]agamodel.PortRange{
				"l-1": {
					{
						FromPort: 80,
						ToPort:   80,
					},
				},
				"l-2": {
					{
						FromPort: 400,
						ToPort:   500, // Includes 450
					},
				},
			},
			wantErr:           true,
			expectErrContains: "endpoint port 450 conflicts with listener l-2 port range 400-500",
		},
		{
			name:         "endpoint port at boundary of listener port range",
			endpointPort: 400, // Exactly at FromPort boundary
			listenerPortRanges: map[string][]agamodel.PortRange{
				"l-1": {
					{
						FromPort: 400,
						ToPort:   500,
					},
				},
			},
			wantErr:           true,
			expectErrContains: "endpoint port 400 conflicts with listener l-1 port range 400-500",
		},
		{
			name:         "endpoint port at upper boundary of listener port range",
			endpointPort: 500, // Exactly at ToPort boundary
			listenerPortRanges: map[string][]agamodel.PortRange{
				"l-1": {
					{
						FromPort: 400,
						ToPort:   500,
					},
				},
			},
			wantErr:           true,
			expectErrContains: "endpoint port 500 conflicts with listener l-1 port range 400-500",
		},
		{
			name:         "multiple listener port ranges, endpoint port in one range",
			endpointPort: 1024,
			listenerPortRanges: map[string][]agamodel.PortRange{
				"l-1": {
					{
						FromPort: 80,
						ToPort:   80,
					},
					{
						FromPort: 443,
						ToPort:   443,
					},
				},
				"l-2": {
					{
						FromPort: 1000,
						ToPort:   2000, // Includes 1024
					},
					{
						FromPort: 3000,
						ToPort:   4000,
					},
				},
			},
			wantErr:           true,
			expectErrContains: "endpoint port 1024 conflicts with listener l-2 port range 1000-2000",
		},
		{
			name:               "empty listener port ranges",
			endpointPort:       8080,
			listenerPortRanges: map[string][]agamodel.PortRange{},
			wantErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &defaultEndpointGroupBuilder{
				clusterRegion: "us-west-2",
			}
			err := builder.validateEndpointPortOverridesWithinListener(tt.endpointPort, tt.listenerPortRanges)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_defaultEndpointGroupBuilder_validateNoDuplicatePorts(t *testing.T) {
	tests := []struct {
		name              string
		portOverrides     []agamodel.PortOverride
		wantErr           bool
		expectErrContains string
		portType          string // "listener" or "endpoint"
	}{
		{
			name:          "no port overrides",
			portOverrides: []agamodel.PortOverride{},
			wantErr:       false,
		},
		{
			name: "single port override",
			portOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
			},
			wantErr: false,
		},
		{
			name: "multiple port overrides with unique ports",
			portOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
				{
					ListenerPort: 443,
					EndpointPort: 8443,
				},
				{
					ListenerPort: 8000,
					EndpointPort: 9000,
				},
			},
			wantErr: false,
		},
		{
			name: "multiple port overrides with duplicate listener ports",
			portOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
				{
					ListenerPort: 80,   // Duplicate listener port
					EndpointPort: 9090, // Different endpoint port
				},
				{
					ListenerPort: 443,
					EndpointPort: 8443,
				},
			},
			wantErr:           true,
			expectErrContains: "duplicate listener port 80 in port overrides",
			portType:          "listener",
		},
		{
			name: "multiple duplicate listener ports",
			portOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
				{
					ListenerPort: 443,
					EndpointPort: 8443,
				},
				{
					ListenerPort: 80, // First duplicate
					EndpointPort: 9090,
				},
				{
					ListenerPort: 443, // Second duplicate
					EndpointPort: 9443,
				},
			},
			wantErr:           true,
			expectErrContains: "duplicate listener port 80 in port overrides",
			portType:          "listener",
		},
		{
			name: "multiple port overrides with duplicate endpoint ports",
			portOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
				{
					ListenerPort: 443,
					EndpointPort: 8080, // Duplicate endpoint port
				},
				{
					ListenerPort: 8000,
					EndpointPort: 9000,
				},
			},
			wantErr:           true,
			expectErrContains: "duplicate endpoint port 8080 in port overrides",
			portType:          "endpoint",
		},
		{
			name: "multiple duplicate endpoint ports",
			portOverrides: []agamodel.PortOverride{
				{
					ListenerPort: 80,
					EndpointPort: 8080,
				},
				{
					ListenerPort: 443,
					EndpointPort: 8443,
				},
				{
					ListenerPort: 8000,
					EndpointPort: 8080, // First duplicate
				},
				{
					ListenerPort: 9000,
					EndpointPort: 8443, // Second duplicate
				},
			},
			wantErr:           true,
			expectErrContains: "duplicate endpoint port 8080 in port overrides",
			portType:          "endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &defaultEndpointGroupBuilder{
				clusterRegion: "us-west-2",
			}
			err := builder.validateNoDuplicatePorts(tt.portOverrides)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_defaultEndpointGroupBuilder_validateEndpointPortOverridesCrossListeners(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	// Helper function to create a listener with specific ID and port ranges
	createTestListener := func(id string, portRanges []agamodel.PortRange) *agamodel.Listener {
		return &agamodel.Listener{
			ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::Listener", id),
			Spec: agamodel.ListenerSpec{
				PortRanges: portRanges,
			},
		}
	}

	// Helper function to create an endpoint group with specific listener and port overrides
	createTestEndpointGroup := func(id string, region string, listener *agamodel.Listener, portOverrides []agamodel.PortOverride) *agamodel.EndpointGroup {
		return &agamodel.EndpointGroup{
			ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", id),
			Listener:     listener,
			Spec: agamodel.EndpointGroupSpec{
				Region:        region,
				PortOverrides: portOverrides,
			},
		}
	}

	tests := []struct {
		name               string
		endpointGroups     []*agamodel.EndpointGroup
		listenerPortRanges map[string][]agamodel.PortRange
		wantErr            bool
		expectErrContains  string
	}{
		{
			name:               "no endpoint groups",
			endpointGroups:     []*agamodel.EndpointGroup{},
			listenerPortRanges: map[string][]agamodel.PortRange{},
			wantErr:            false,
		},
		{
			name: "single endpoint group - no conflicts possible",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 80},
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener, []agamodel.PortOverride{
						{ListenerPort: 80, EndpointPort: 8080},
					}),
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {{FromPort: 80, ToPort: 80}},
			},
			wantErr: false,
		},
		{
			name: "multiple endpoint groups, same listener - no conflicts",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 80},
					{FromPort: 443, ToPort: 443},
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener, []agamodel.PortOverride{
						{ListenerPort: 80, EndpointPort: 8080},
					}),
					createTestEndpointGroup("eg-2", "eu-west-1", listener, []agamodel.PortOverride{
						{ListenerPort: 443, EndpointPort: 8443},
					}),
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {
					{FromPort: 80, ToPort: 80},
					{FromPort: 443, ToPort: 443},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple endpoint groups, different listeners, no conflicts",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener1 := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 80},
				})
				listener2 := createTestListener("listener-2", []agamodel.PortRange{
					{FromPort: 443, ToPort: 443},
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener1, []agamodel.PortOverride{
						{ListenerPort: 80, EndpointPort: 8080},
					}),
					createTestEndpointGroup("eg-2", "eu-west-1", listener2, []agamodel.PortOverride{
						{ListenerPort: 443, EndpointPort: 9090}, // Different endpoint port
					}),
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {{FromPort: 80, ToPort: 80}},
				"listener-2": {{FromPort: 443, ToPort: 443}},
			},
			wantErr: false,
		},
		{
			name: "endpoint port in listener port range - conflict",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener1 := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 90}, // Range includes 85
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener1, []agamodel.PortOverride{
						{ListenerPort: 80, EndpointPort: 85}, // Endpoint port is within listener port range
					}),
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {{FromPort: 80, ToPort: 90}},
			},
			wantErr:           true,
			expectErrContains: "endpoint port 85 conflicts with listener listener-1 port range 80-90",
		},
		{
			name: "duplicate endpoint port across different listeners - conflict",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener1 := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 80},
				})
				listener2 := createTestListener("listener-2", []agamodel.PortRange{
					{FromPort: 443, ToPort: 443},
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener1, []agamodel.PortOverride{
						{ListenerPort: 80, EndpointPort: 8080},
					}),
					createTestEndpointGroup("eg-2", "eu-west-1", listener2, []agamodel.PortOverride{
						{ListenerPort: 443, EndpointPort: 8080}, // Same endpoint port as eg-1
					}),
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {{FromPort: 80, ToPort: 80}},
				"listener-2": {{FromPort: 443, ToPort: 443}},
			},
			wantErr:           true,
			expectErrContains: "duplicate endpoint port 8080: the same endpoint port cannot be used in port overrides from different listeners",
		},
		{
			name: "multiple duplicate endpoint ports across different listeners",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener1 := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 80},
					{FromPort: 443, ToPort: 443},
				})
				listener2 := createTestListener("listener-2", []agamodel.PortRange{
					{FromPort: 8080, ToPort: 8080},
					{FromPort: 8443, ToPort: 8443},
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener1, []agamodel.PortOverride{
						{ListenerPort: 80, EndpointPort: 9090},
						{ListenerPort: 443, EndpointPort: 9091},
					}),
					createTestEndpointGroup("eg-2", "eu-west-1", listener2, []agamodel.PortOverride{
						{ListenerPort: 8080, EndpointPort: 9090}, // Duplicate with listener-1
						{ListenerPort: 8443, EndpointPort: 7070}, // Unique
					}),
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {
					{FromPort: 80, ToPort: 80},
					{FromPort: 443, ToPort: 443},
				},
				"listener-2": {
					{FromPort: 8080, ToPort: 8080},
					{FromPort: 8443, ToPort: 8443},
				},
			},
			wantErr:           true,
			expectErrContains: "duplicate endpoint port 9090: the same endpoint port cannot be used in port overrides from different listeners",
		},
		{
			name: "multiple endpoint groups with mixed conflicts",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener1 := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 80},
				})
				listener2 := createTestListener("listener-2", []agamodel.PortRange{
					{FromPort: 443, ToPort: 443},
				})
				listener3 := createTestListener("listener-3", []agamodel.PortRange{
					{FromPort: 9500, ToPort: 9999}, // Range does not include 8080
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener1, []agamodel.PortOverride{
						{ListenerPort: 80, EndpointPort: 8080},
					}),
					createTestEndpointGroup("eg-2", "eu-west-1", listener2, []agamodel.PortOverride{
						{ListenerPort: 443, EndpointPort: 8080}, // Duplicate endpoint port with listener1
					}),
					createTestEndpointGroup("eg-3", "ap-southeast-1", listener3, []agamodel.PortOverride{
						{ListenerPort: 8080, EndpointPort: 9999}, // Endpoint port outside of any listener range
					}),
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {{FromPort: 80, ToPort: 80}},
				"listener-2": {{FromPort: 443, ToPort: 443}},
				"listener-3": {{FromPort: 9500, ToPort: 9999}},
			},
			wantErr:           true,
			expectErrContains: "duplicate endpoint port 8080: the same endpoint port cannot be used in port overrides from different listeners",
		},
		{
			name: "same endpoint port in different regions - still a conflict",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener1 := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 80},
				})
				listener2 := createTestListener("listener-2", []agamodel.PortRange{
					{FromPort: 443, ToPort: 443},
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener1, []agamodel.PortOverride{
						{ListenerPort: 80, EndpointPort: 8080},
					}),
					createTestEndpointGroup("eg-2", "eu-west-1", listener2, []agamodel.PortOverride{
						// Even though in different regions, same port across listeners is still a conflict
						{ListenerPort: 443, EndpointPort: 8080},
					}),
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {{FromPort: 80, ToPort: 80}},
				"listener-2": {{FromPort: 443, ToPort: 443}},
			},
			wantErr:           true,
			expectErrContains: "duplicate endpoint port 8080: the same endpoint port cannot be used in port overrides from different listeners",
		},
		{
			name: "endpoint groups with no port overrides - no conflicts",
			endpointGroups: func() []*agamodel.EndpointGroup {
				listener1 := createTestListener("listener-1", []agamodel.PortRange{
					{FromPort: 80, ToPort: 80},
				})
				listener2 := createTestListener("listener-2", []agamodel.PortRange{
					{FromPort: 443, ToPort: 443},
				})
				return []*agamodel.EndpointGroup{
					createTestEndpointGroup("eg-1", "us-west-2", listener1, nil), // No port overrides
					createTestEndpointGroup("eg-2", "eu-west-1", listener2, nil), // No port overrides
				}
			}(),
			listenerPortRanges: map[string][]agamodel.PortRange{
				"listener-1": {{FromPort: 80, ToPort: 80}},
				"listener-2": {{FromPort: 443, ToPort: 443}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create endpointGroupBuilder
			builder := &defaultEndpointGroupBuilder{
				clusterRegion: "us-west-2",
			}

			// Run the validation function
			err := builder.validateEndpointPortOverridesCrossListeners(tt.endpointGroups, tt.listenerPortRanges)

			// Check if error was expected
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
