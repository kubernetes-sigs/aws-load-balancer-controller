package aga

import (
	"context"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"

	"github.com/stretchr/testify/assert"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
)

func Test_globalAcceleratorValidator_ValidateCreate(t *testing.T) {
	// Protocol references for direct pointer usage
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP

	tests := []struct {
		name       string
		ga         *agaapi.GlobalAccelerator
		wantErr    string
		wantMetric bool
	}{
		{
			name: "valid global accelerator with no listeners",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: nil,
				},
			},
			wantErr:    "",
			wantMetric: false,
		},
		{
			name: "valid global accelerator with single listener",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
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
				},
			},
			wantErr:    "",
			wantMetric: false,
		},
		{
			name: "invalid global accelerator with single listener and overlapping ranges between listeners",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   8080,
								},
								{
									FromPort: 443,
									ToPort:   443,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "overlapping port ranges detected for protocol TCP, which is not allowed",
			wantMetric: true,
		},
		{
			name: "valid global accelerator with multiple listeners with different protocols and non-overlapping ranges",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
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
									FromPort: 443,
									ToPort:   443,
								},
							},
							ClientAffinity: agaapi.ClientAffinitySourceIP,
						},
					},
				},
			},
			wantErr:    "",
			wantMetric: false,
		},
		{
			name: "valid global accelerator with  with multiple listeners with different protocols and overlapping port ranges",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   90,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
						{
							Protocol: &protocolUDP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   90,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "",
			wantMetric: false,
		},
		{
			name: "valid global accelerator with single listener having multiple non-overlapping port ranges",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
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
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "",
			wantMetric: false,
		},
		{
			name: "valid global accelerator with multiple listeners having multiple non-overlapping port ranges",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
								{
									FromPort: 443,
									ToPort:   443,
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
								{
									FromPort: 123,
									ToPort:   123,
								},
							},
							ClientAffinity: agaapi.ClientAffinitySourceIP,
						},
					},
				},
			},
			wantErr:    "",
			wantMetric: false,
		},
		{
			name: "valid global accelerator with multiple listeners having multiple port ranges of the same protocol but no overlap",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
								{
									FromPort: 443,
									ToPort:   443,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 8080,
									ToPort:   8080,
								},
								{
									FromPort: 8443,
									ToPort:   8443,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "",
			wantMetric: false,
		},
		{
			name: "invalid global accelerator with multiple listeners having multiple port ranges with partial overlap",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
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
									FromPort: 8000,
									ToPort:   9000,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 1000,
									ToPort:   2000,
								},
								{
									FromPort: 8500,
									ToPort:   8600, // Overlaps with 8000-9000 in first listener
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "overlapping port ranges detected for protocol TCP, which is not allowed",
			wantMetric: true,
		},
		{
			name: "invalid global accelerator with wide port range overlapping with specific port",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 1000,
									ToPort:   2000, // Wide range
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 1500,
									ToPort:   1500, // Single port within the wide range
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "overlapping port ranges detected for protocol TCP, which is not allowed",
			wantMetric: true,
		},
		{
			name: "valid global accelerator with touching but not overlapping port ranges",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 1000,
									ToPort:   2000,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 2001, // Just after the previous range ends
									ToPort:   3000,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "",
			wantMetric: false,
		},
		{
			name: "invalid global accelerator with single listener having overlapping port ranges",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 1000,
									ToPort:   2000,
								},
								{
									FromPort: 1500, // Overlaps with the first range
									ToPort:   2500,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "overlapping port ranges detected for protocol TCP, which is not allowed",
			wantMetric: true,
		},
		{
			name: "invalid global accelerator with single listener and overlapping port ranges within listener",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   8080,
								},
								{
									FromPort: 443,
									ToPort:   443,
								},
								{
									FromPort: 1000,
									ToPort:   2000,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantErr:    "overlapping port ranges detected for protocol TCP, which is not allowed",
			wantMetric: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock dependencies
			logger := logr.New(&log.NullLogSink{})
			mockMetricsCollector := lbcmetrics.NewMockCollector()

			// Create the validator
			v := NewGlobalAcceleratorValidator(logger, mockMetricsCollector)

			// Run tests for both create and update
			t.Run("create", func(t *testing.T) {
				err := v.ValidateCreate(context.Background(), tt.ga)
				if tt.wantErr != "" {
					assert.EqualError(t, err, tt.wantErr)
				} else {
					assert.NoError(t, err)
				}
			})

			t.Run("update", func(t *testing.T) {
				err := v.ValidateUpdate(context.Background(), tt.ga, &agaapi.GlobalAccelerator{})
				if tt.wantErr != "" {
					assert.EqualError(t, err, tt.wantErr)
				} else {
					assert.NoError(t, err)
				}
			})

			// Verify metrics collection
			mockCollector := v.metricsCollector.(*lbcmetrics.MockCollector)
			if tt.wantMetric {
				// Should have 2 invocations, one for create and one for update
				assert.Equal(t, 2, len(mockCollector.Invocations[lbcmetrics.MetricWebhookValidationFailure]))
			} else {
				assert.Equal(t, 0, len(mockCollector.Invocations[lbcmetrics.MetricWebhookValidationFailure]))
			}
		})
	}
}

func Test_globalAcceleratorValidator_checkForOverlappingPortRanges(t *testing.T) {
	// Protocol references for direct pointer usage
	protocolTCP := agaapi.GlobalAcceleratorProtocolTCP
	protocolUDP := agaapi.GlobalAcceleratorProtocolUDP

	tests := []struct {
		name              string
		globalAccelerator *agaapi.GlobalAccelerator
		wantError         bool
		errorContains     string
	}{
		{
			name: "no listeners",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: nil,
				},
			},
			wantError: false,
		},
		{
			name: "single listener",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "two listeners with different protocols - no overlap",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
						{
							Protocol: &protocolUDP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "two TCP listeners with non-overlapping port ranges",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 443,
									ToPort:   443,
								},
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "two TCP listeners with directly overlapping port ranges",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
					},
				},
			},
			wantError:     true,
			errorContains: "overlapping port ranges detected for protocol",
		},
		{
			name: "overlapping port ranges with nil protocol should be skipped",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: nil, // Will be skipped
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
					},
				},
			},
			wantError: false, // No error because nil protocol listeners are skipped
		},
		{
			name: "multiple port ranges with partial overlap",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   100,
								},
								{
									FromPort: 200,
									ToPort:   300,
								},
							},
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 90,
									ToPort:   150,
								},
								{
									FromPort: 400,
									ToPort:   500,
								},
							},
						},
					},
				},
			},
			wantError:     true,
			errorContains: "overlapping port ranges detected for protocol",
		},
		{
			name: "port ranges with second range overlapping first",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 200,
									ToPort:   300,
								},
							},
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 250,
									ToPort:   350,
								},
							},
						},
					},
				},
			},
			wantError:     true,
			errorContains: "overlapping port ranges detected for protocol",
		},
		{
			name: "port ranges with edge case - touching but not overlapping",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 100,
									ToPort:   200,
								},
							},
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 201,
									ToPort:   300,
								},
							},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "example from task description",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
								{
									FromPort: 443,
									ToPort:   443,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   78, // Likely a mistake in the example, but should be caught as overlapping with 80
								},
								{
									FromPort: 443,
									ToPort:   443,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantError:     true,
			errorContains: "overlapping port ranges detected for protocol",
		},
		{
			name: "single listener with multiple non-overlapping port ranges",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
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
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "single listener with overlapping port ranges",
			globalAccelerator: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Listeners: &[]agaapi.GlobalAcceleratorListener{
						{
							Protocol: &protocolTCP,
							PortRanges: &[]agaapi.PortRange{
								{
									FromPort: 80,
									ToPort:   100,
								},
								{
									FromPort: 90, // Overlaps with previous range
									ToPort:   120,
								},
							},
							ClientAffinity: agaapi.ClientAffinityNone,
						},
					},
				},
			},
			wantError:     true,
			errorContains: "overlapping port ranges detected for protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.New(&log.NullLogSink{})

			// Create a mock metrics collector
			mockMetricsCollector := lbcmetrics.NewMockCollector()

			validator := &globalAcceleratorValidator{
				logger:           logger,
				metricsCollector: mockMetricsCollector,
			}

			err := validator.checkForOverlappingPortRanges(tt.globalAccelerator)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_portRangesOverlap(t *testing.T) {
	tests := []struct {
		name   string
		rangeA agaapi.PortRange
		rangeB agaapi.PortRange
		want   bool
	}{
		{
			name: "exactly matching ranges",
			rangeA: agaapi.PortRange{
				FromPort: 80,
				ToPort:   80,
			},
			rangeB: agaapi.PortRange{
				FromPort: 80,
				ToPort:   80,
			},
			want: true,
		},
		{
			name: "completely non-overlapping ranges",
			rangeA: agaapi.PortRange{
				FromPort: 80,
				ToPort:   90,
			},
			rangeB: agaapi.PortRange{
				FromPort: 100,
				ToPort:   110,
			},
			want: false,
		},
		{
			name: "A partially overlaps B (lower)",
			rangeA: agaapi.PortRange{
				FromPort: 80,
				ToPort:   100,
			},
			rangeB: agaapi.PortRange{
				FromPort: 90,
				ToPort:   110,
			},
			want: true,
		},
		{
			name: "A partially overlaps B (higher)",
			rangeA: agaapi.PortRange{
				FromPort: 90,
				ToPort:   110,
			},
			rangeB: agaapi.PortRange{
				FromPort: 80,
				ToPort:   100,
			},
			want: true,
		},
		{
			name: "A completely contains B",
			rangeA: agaapi.PortRange{
				FromPort: 80,
				ToPort:   120,
			},
			rangeB: agaapi.PortRange{
				FromPort: 90,
				ToPort:   110,
			},
			want: true,
		},
		{
			name: "B completely contains A",
			rangeA: agaapi.PortRange{
				FromPort: 90,
				ToPort:   110,
			},
			rangeB: agaapi.PortRange{
				FromPort: 80,
				ToPort:   120,
			},
			want: true,
		},
		{
			name: "Adjacent ranges (not overlapping)",
			rangeA: agaapi.PortRange{
				FromPort: 80,
				ToPort:   90,
			},
			rangeB: agaapi.PortRange{
				FromPort: 91,
				ToPort:   100,
			},
			want: false,
		},
		{
			name: "Touching ranges (should be considered overlap)",
			rangeA: agaapi.PortRange{
				FromPort: 80,
				ToPort:   90,
			},
			rangeB: agaapi.PortRange{
				FromPort: 90,
				ToPort:   100,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := portRangesOverlap(tt.rangeA, tt.rangeB)
			assert.Equal(t, tt.want, result)
		})
	}
}
