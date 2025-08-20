package elbv2

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/log"

	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
)

func TestGlobalAcceleratorValidatorValidateSpec(t *testing.T) {
	logger := logr.New(&log.NullLogSink{})
	metricsCollector := lbcmetrics.NewMockCollector()
	validator := NewGlobalAcceleratorValidator(logger, metricsCollector)

	testCases := []struct {
		name          string
		spec          elbv2api.GlobalAcceleratorSpec
		expectedError bool
		errorMessage  string
	}{
		{
			name: "valid spec",
			spec: elbv2api.GlobalAcceleratorSpec{
				Listeners: []elbv2api.GlobalAcceleratorListener{
					{
						Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
						PortRanges: []elbv2api.PortRange{
							{
								FromPort: 80,
								ToPort:   80,
							},
						},
					},
				},
				EndpointGroups: []elbv2api.EndpointGroup{
					{
						Region: "us-west-2",
						Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
							{
								EndpointID: "test-endpoint",
							},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "no listeners",
			spec: elbv2api.GlobalAcceleratorSpec{
				EndpointGroups: []elbv2api.EndpointGroup{
					{
						Region: "us-west-2",
						Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
							{
								EndpointID: "test-endpoint",
							},
						},
					},
				},
			},
			expectedError: true,
			errorMessage:  "at least one listener is required",
		},
		{
			name: "no endpoint groups",
			spec: elbv2api.GlobalAcceleratorSpec{
				Listeners: []elbv2api.GlobalAcceleratorListener{
					{
						Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
						PortRanges: []elbv2api.PortRange{
							{
								FromPort: 80,
								ToPort:   80,
							},
						},
					},
				},
			},
			expectedError: true,
			errorMessage:  "at least one endpoint group is required",
		},
		{
			name: "insufficient endpoint groups for listeners",
			spec: elbv2api.GlobalAcceleratorSpec{
				Listeners: []elbv2api.GlobalAcceleratorListener{
					{
						Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
						PortRanges: []elbv2api.PortRange{
							{
								FromPort: 80,
								ToPort:   80,
							},
						},
					},
					{
						Protocol: elbv2api.GlobalAcceleratorProtocolUDP,
						PortRanges: []elbv2api.PortRange{
							{
								FromPort: 53,
								ToPort:   53,
							},
						},
					},
				},
				EndpointGroups: []elbv2api.EndpointGroup{
					{
						Region: "us-west-2",
						Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
							{
								EndpointID: "test-endpoint",
							},
						},
					},
				},
			},
			expectedError: true,
			errorMessage:  "number of endpoint groups (1) must be at least the number of listeners (2)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateSpec(tc.spec)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorMessage != "" {
					assert.Contains(t, err.Error(), tc.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGlobalAcceleratorValidatorValidateListener(t *testing.T) {
	logger := logr.New(&log.NullLogSink{})
	metricsCollector := lbcmetrics.NewMockCollector()
	validator := NewGlobalAcceleratorValidator(logger, metricsCollector)

	testCases := []struct {
		name          string
		listener      elbv2api.GlobalAcceleratorListener
		expectedError bool
		errorMessage  string
	}{
		{
			name: "valid TCP listener",
			listener: elbv2api.GlobalAcceleratorListener{
				Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
				PortRanges: []elbv2api.PortRange{
					{
						FromPort: 80,
						ToPort:   80,
					},
				},
			},
			expectedError: false,
		},
		{
			name: "valid UDP listener",
			listener: elbv2api.GlobalAcceleratorListener{
				Protocol: elbv2api.GlobalAcceleratorProtocolUDP,
				PortRanges: []elbv2api.PortRange{
					{
						FromPort: 53,
						ToPort:   53,
					},
				},
			},
			expectedError: false,
		},
		{
			name: "invalid protocol",
			listener: elbv2api.GlobalAcceleratorListener{
				Protocol: "HTTP",
				PortRanges: []elbv2api.PortRange{
					{
						FromPort: 80,
						ToPort:   80,
					},
				},
			},
			expectedError: true,
			errorMessage:  "invalid protocol HTTP, must be TCP or UDP",
		},
		{
			name: "no port ranges",
			listener: elbv2api.GlobalAcceleratorListener{
				Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
			},
			expectedError: true,
			errorMessage:  "at least one port range is required",
		},
		{
			name: "invalid port range - fromPort too low",
			listener: elbv2api.GlobalAcceleratorListener{
				Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
				PortRanges: []elbv2api.PortRange{
					{
						FromPort: 0,
						ToPort:   80,
					},
				},
			},
			expectedError: true,
			errorMessage:  "fromPort must be between 1 and 65535",
		},
		{
			name: "invalid port range - fromPort > toPort",
			listener: elbv2api.GlobalAcceleratorListener{
				Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
				PortRanges: []elbv2api.PortRange{
					{
						FromPort: 90,
						ToPort:   80,
					},
				},
			},
			expectedError: true,
			errorMessage:  "fromPort (90) cannot be greater than toPort (80)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateListener(tc.listener, 0)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorMessage != "" {
					assert.Contains(t, err.Error(), tc.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGlobalAcceleratorValidatorValidateEndpointGroup(t *testing.T) {
	logger := logr.New(&log.NullLogSink{})
	metricsCollector := lbcmetrics.NewMockCollector()
	validator := NewGlobalAcceleratorValidator(logger, metricsCollector)

	testCases := []struct {
		name          string
		endpointGroup elbv2api.EndpointGroup
		expectedError bool
		errorMessage  string
	}{
		{
			name: "valid endpoint group",
			endpointGroup: elbv2api.EndpointGroup{
				Region: "us-west-2",
				Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
					{
						EndpointID: "test-endpoint",
					},
				},
			},
			expectedError: false,
		},
		{
			name: "empty region",
			endpointGroup: elbv2api.EndpointGroup{
				Region: "",
				Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
					{
						EndpointID: "test-endpoint",
					},
				},
			},
			expectedError: true,
			errorMessage:  "region is required",
		},
		{
			name: "no endpoints",
			endpointGroup: elbv2api.EndpointGroup{
				Region: "us-west-2",
			},
			expectedError: true,
			errorMessage:  "at least one endpoint is required",
		},
		{
			name: "invalid endpoint - empty ID",
			endpointGroup: elbv2api.EndpointGroup{
				Region: "us-west-2",
				Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
					{
						EndpointID: "",
					},
				},
			},
			expectedError: true,
			errorMessage:  "endpointID is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateEndpointGroup(tc.endpointGroup, 0)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorMessage != "" {
					assert.Contains(t, err.Error(), tc.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGlobalAcceleratorValidatorValidateGlobalAccelerator(t *testing.T) {
	logger := logr.New(&log.NullLogSink{})
	metricsCollector := lbcmetrics.NewMockCollector()
	validator := NewGlobalAcceleratorValidator(logger, metricsCollector)

	ctx := context.Background()

	validGA := &elbv2api.GlobalAccelerator{
		Spec: elbv2api.GlobalAcceleratorSpec{
			Listeners: []elbv2api.GlobalAcceleratorListener{
				{
					Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
					PortRanges: []elbv2api.PortRange{
						{
							FromPort: 80,
							ToPort:   80,
						},
					},
				},
			},
			EndpointGroups: []elbv2api.EndpointGroup{
				{
					Region: "us-west-2",
					Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
						{
							EndpointID: "test-endpoint",
						},
					},
				},
			},
		},
	}

	err := validator.validateGlobalAccelerator(ctx, validGA)
	assert.NoError(t, err)

	invalidGA := &elbv2api.GlobalAccelerator{
		Spec: elbv2api.GlobalAcceleratorSpec{
			// Missing listeners - should cause validation error
		},
	}

	err = validator.validateGlobalAccelerator(ctx, invalidGA)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one listener is required")
}
