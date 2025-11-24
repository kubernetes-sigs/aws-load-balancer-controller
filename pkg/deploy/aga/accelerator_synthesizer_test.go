package aga

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_acceleratorSynthesizer_describeAcceleratorByARN(t *testing.T) {
	// Setup controller and mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create test ARN
	testARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"

	tests := []struct {
		name              string
		arn               string
		setupExpectations func(mockGAClient *services.MockGlobalAccelerator)
		wantAccelerator   *agatypes.Accelerator
		wantTags          map[string]string
		wantError         bool
	}{
		{
			name: "Successfully describe accelerator with tags",
			arn:  testARN,
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Expect DescribeAcceleratorWithContext call
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &agatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator"),
							IpAddressType:  agatypes.IpAddressTypeIpv4,
							Enabled:        aws.Bool(true),
							DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
							Status:         agatypes.AcceleratorStatusDeployed,
						},
					}, nil)

				// Expect ListTagsForResourceWithContext call
				mockGAClient.EXPECT().
					ListTagsForResourceWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.ListTagsForResourceInput{
						ResourceArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.ListTagsForResourceOutput{
						Tags: []agatypes.Tag{
							{
								Key:   aws.String("aga.k8s.aws/resource"),
								Value: aws.String("test-accelerator"),
							},
							{
								Key:   aws.String("Environment"),
								Value: aws.String("test"),
							},
						},
					}, nil)
			},
			wantAccelerator: &agatypes.Accelerator{
				AcceleratorArn: aws.String(testARN),
				Name:           aws.String("test-accelerator"),
				IpAddressType:  agatypes.IpAddressTypeIpv4,
				Enabled:        aws.Bool(true),
				DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
				Status:         agatypes.AcceleratorStatusDeployed,
			},
			wantTags: map[string]string{
				"aga.k8s.aws/resource": "test-accelerator",
				"Environment":          "test",
			},
			wantError: false,
		},
		{
			name: "Error describing accelerator",
			arn:  testARN,
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Expect DescribeAcceleratorWithContext call with error
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(nil, errors.New("describe accelerator error"))
			},
			wantAccelerator: nil,
			wantTags:        nil,
			wantError:       true,
		},
		{
			name: "Error listing tags",
			arn:  testARN,
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Expect DescribeAcceleratorWithContext call
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &agatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator"),
							IpAddressType:  agatypes.IpAddressTypeIpv4,
							Enabled:        aws.Bool(true),
							DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
							Status:         agatypes.AcceleratorStatusDeployed,
						},
					}, nil)

				// Expect ListTagsForResourceWithContext call with error
				mockGAClient.EXPECT().
					ListTagsForResourceWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.ListTagsForResourceInput{
						ResourceArn: aws.String(testARN),
					})).
					Return(nil, errors.New("list tags error"))
			},
			wantAccelerator: nil,
			wantTags:        nil,
			wantError:       true,
		},
		{
			name: "Successfully describe accelerator with no tags",
			arn:  testARN,
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Expect DescribeAcceleratorWithContext call
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &agatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator-no-tags"),
							IpAddressType:  agatypes.IpAddressTypeIpv4,
							Enabled:        aws.Bool(true),
							DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
							Status:         agatypes.AcceleratorStatusDeployed,
						},
					}, nil)

				// Expect ListTagsForResourceWithContext call with empty tags
				mockGAClient.EXPECT().
					ListTagsForResourceWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.ListTagsForResourceInput{
						ResourceArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.ListTagsForResourceOutput{
						Tags: []agatypes.Tag{},
					}, nil)
			},
			wantAccelerator: &agatypes.Accelerator{
				AcceleratorArn: aws.String(testARN),
				Name:           aws.String("test-accelerator-no-tags"),
				IpAddressType:  agatypes.IpAddressTypeIpv4,
				Enabled:        aws.Bool(true),
				DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
				Status:         agatypes.AcceleratorStatusDeployed,
			},
			wantTags:  map[string]string{},
			wantError: false,
		},
		{
			name: "Successfully describe accelerator with nil tag values",
			arn:  testARN,
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Expect DescribeAcceleratorWithContext call
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &agatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator"),
							IpAddressType:  agatypes.IpAddressTypeIpv4,
							Enabled:        aws.Bool(true),
							DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
							Status:         agatypes.AcceleratorStatusDeployed,
						},
					}, nil)

				// Expect ListTagsForResourceWithContext call with some nil tag values
				mockGAClient.EXPECT().
					ListTagsForResourceWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.ListTagsForResourceInput{
						ResourceArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.ListTagsForResourceOutput{
						Tags: []agatypes.Tag{
							{
								Key:   aws.String("aga.k8s.aws/resource"),
								Value: aws.String("test-accelerator"),
							},
							{
								Key:   aws.String("NilValue"),
								Value: nil,
							},
							{
								Key:   nil,
								Value: aws.String("NilKey"),
							},
						},
					}, nil)
			},
			wantAccelerator: &agatypes.Accelerator{
				AcceleratorArn: aws.String(testARN),
				Name:           aws.String("test-accelerator"),
				IpAddressType:  agatypes.IpAddressTypeIpv4,
				Enabled:        aws.Bool(true),
				DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
				Status:         agatypes.AcceleratorStatusDeployed,
			},
			wantTags: map[string]string{
				"aga.k8s.aws/resource": "test-accelerator",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockGAClient := services.NewMockGlobalAccelerator(ctrl)
			mockTrackingProvider := tracking.NewMockProvider(ctrl)
			mockTaggingManager := NewMockTaggingManager(ctrl)
			mockAccManager := NewMockAcceleratorManager(ctrl)
			logger := logr.New(&log.NullLogSink{})

			// Setup expectations
			if tt.setupExpectations != nil {
				tt.setupExpectations(mockGAClient)
			}

			// Create synthesizer
			synthesizer := &acceleratorSynthesizer{
				gaClient:           mockGAClient,
				trackingProvider:   mockTrackingProvider,
				taggingManager:     mockTaggingManager,
				acceleratorManager: mockAccManager,
				logger:             logger,
				stack:              nil, // Not used in this test
			}

			// Run the method being tested
			got, err := synthesizer.describeAcceleratorByARN(context.Background(), tt.arn)

			// Assert expectations
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantAccelerator, got.Accelerator)
				assert.Equal(t, tt.wantTags, got.Tags)
			}
		})
	}
}

func Test_acceleratorSynthesizer_handleCreateAccelerator(t *testing.T) {
	// Setup controller and mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create test stack
	stack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name              string
		resAccelerator    *agamodel.Accelerator
		setupExpectations func(mockGAClient *services.MockGlobalAccelerator, mockAccManager *MockAcceleratorManager)
		wantStatus        agamodel.AcceleratorStatus
		wantError         bool
	}{
		{
			name: "Successful accelerator creation",
			resAccelerator: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, "ga", "test-accelerator"),
				Spec: agamodel.AcceleratorSpec{
					Name:          "new-accelerator",
					IPAddressType: agamodel.IPAddressTypeIPV4,
					Enabled:       aws.Bool(true),
					Tags: map[string]string{
						"Environment": "test",
					},
				},
			},
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator, mockAccManager *MockAcceleratorManager) {
				mockAccManager.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, resAcc *agamodel.Accelerator) (agamodel.AcceleratorStatus, error) {
						// Verify that the resource accelerator is correctly passed to the Create method
						assert.Equal(t, "new-accelerator", resAcc.Spec.Name)
						assert.Equal(t, agamodel.IPAddressTypeIPV4, resAcc.Spec.IPAddressType)
						assert.True(t, *resAcc.Spec.Enabled)
						assert.Equal(t, "test", resAcc.Spec.Tags["Environment"])

						// Return the expected status
						return agamodel.AcceleratorStatus{
							AcceleratorARN: "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
							DNSName:        "a1234567890abcdef.awsglobalaccelerator.com",
							Status:         "DEPLOYED",
							IPSets: []agamodel.IPSet{
								{
									IpAddressFamily: "IPv4",
									IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
								},
							},
						}, nil
					})
			},
			wantStatus: agamodel.AcceleratorStatus{
				AcceleratorARN: "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
				DNSName:        "a1234567890abcdef.awsglobalaccelerator.com",
				Status:         "DEPLOYED",
				IPSets: []agamodel.IPSet{
					{
						IpAddressFamily: "IPv4",
						IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
					},
				},
			},
			wantError: false,
		},
		{
			name: "Creation error case",
			resAccelerator: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, "ga", "test-accelerator"),
				Spec: agamodel.AcceleratorSpec{
					Name:          "error-accelerator",
					IPAddressType: agamodel.IPAddressTypeIPV4,
					Enabled:       aws.Bool(true),
					Tags:          nil,
				},
			},
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator, mockAccManager *MockAcceleratorManager) {
				mockAccManager.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(agamodel.AcceleratorStatus{}, assert.AnError)
			},
			wantStatus: agamodel.AcceleratorStatus{},
			wantError:  true,
		},
		{
			name: "Create dual stack accelerator",
			resAccelerator: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, "ga", "test-accelerator"),
				Spec: agamodel.AcceleratorSpec{
					Name:          "dual-stack-accelerator",
					IPAddressType: agamodel.IPAddressTypeDualStack,
					Enabled:       aws.Bool(true),
					Tags:          nil,
				},
			},
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator, mockAccManager *MockAcceleratorManager) {
				mockAccManager.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, resAcc *agamodel.Accelerator) (agamodel.AcceleratorStatus, error) {
						// Verify that the IP address type is correctly passed to the Create method
						assert.Equal(t, agamodel.IPAddressTypeDualStack, resAcc.Spec.IPAddressType)

						// Return the expected status for a dual stack accelerator
						return agamodel.AcceleratorStatus{
							AcceleratorARN:   "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
							DNSName:          "a1234567890abcdef.awsglobalaccelerator.com",
							DualStackDNSName: "a1234567890abcdef.dualstack.awsglobalaccelerator.com",
							Status:           "IN_PROGRESS",
							IPSets: []agamodel.IPSet{
								{
									IpAddressFamily: "IPv4",
									IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
								},
								{
									IpAddressFamily: "IPv6",
									IpAddresses:     []string{"2001:db8::1", "2001:db8::2"},
								},
							},
						}, nil
					})
			},
			wantStatus: agamodel.AcceleratorStatus{
				AcceleratorARN:   "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
				DNSName:          "a1234567890abcdef.awsglobalaccelerator.com",
				DualStackDNSName: "a1234567890abcdef.dualstack.awsglobalaccelerator.com",
				Status:           "IN_PROGRESS",
				IPSets: []agamodel.IPSet{
					{
						IpAddressFamily: "IPv4",
						IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
					},
					{
						IpAddressFamily: "IPv6",
						IpAddresses:     []string{"2001:db8::1", "2001:db8::2"},
					},
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockGAClient := services.NewMockGlobalAccelerator(ctrl)
			mockTrackingProvider := tracking.NewMockProvider(ctrl)
			mockTaggingManager := NewMockTaggingManager(ctrl)
			mockAccManager := NewMockAcceleratorManager(ctrl)
			logger := logr.New(&log.NullLogSink{})

			// Setup expectations
			if tt.setupExpectations != nil {
				tt.setupExpectations(mockGAClient, mockAccManager)
			}

			// Create synthesizer
			synthesizer := &acceleratorSynthesizer{
				gaClient:           mockGAClient,
				trackingProvider:   mockTrackingProvider,
				taggingManager:     mockTaggingManager,
				acceleratorManager: mockAccManager,
				logger:             logger,
				stack:              stack,
			}

			// Run the method being tested
			err := synthesizer.handleCreateAccelerator(context.Background(), tt.resAccelerator)

			// Assert expectations
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Check that status is set correctly
				if assert.NotNil(t, tt.resAccelerator.Status) {
					assert.Equal(t, tt.wantStatus, *tt.resAccelerator.Status)
				}
			}
		})
	}
}

func Test_acceleratorSynthesizer_handleUpdateAccelerator(t *testing.T) {
	// Setup controller and mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create test stack
	stack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name              string
		resAccelerator    *agamodel.Accelerator
		sdkAccelerator    AcceleratorWithTags
		setupExpectations func(mockGAClient *services.MockGlobalAccelerator, mockAccManager *MockAcceleratorManager)
		wantStatus        agamodel.AcceleratorStatus
		wantError         bool
	}{
		{
			name: "Successful accelerator update",
			resAccelerator: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, "ga", "test-accelerator"),
				Spec: agamodel.AcceleratorSpec{
					Name:          "updated-accelerator-name",
					IPAddressType: agamodel.IPAddressTypeIPV4,
					Enabled:       aws.Bool(true),
					Tags:          nil,
				},
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &agatypes.Accelerator{
					AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
					Name:           aws.String("original-accelerator-name"),
					IpAddressType:  agatypes.IpAddressTypeIpv4,
					Enabled:        aws.Bool(false),
					DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
					Status:         agatypes.AcceleratorStatusDeployed,
				},
				Tags: map[string]string{
					"aga.k8s.aws/resource": "test-accelerator",
				},
			},
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator, mockAccManager *MockAcceleratorManager) {
				mockAccManager.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, resAcc *agamodel.Accelerator, sdkAcc AcceleratorWithTags) (agamodel.AcceleratorStatus, error) {
						// Verify that the resource accelerator is correctly passed to the Update method
						assert.Equal(t, "updated-accelerator-name", resAcc.Spec.Name)
						assert.Equal(t, agamodel.IPAddressTypeIPV4, resAcc.Spec.IPAddressType)
						assert.True(t, *resAcc.Spec.Enabled)

						// Return the expected status
						return agamodel.AcceleratorStatus{
							AcceleratorARN: *sdkAcc.Accelerator.AcceleratorArn,
							DNSName:        "a1234567890abcdef.awsglobalaccelerator.com",
							Status:         "DEPLOYED",
							IPSets: []agamodel.IPSet{
								{
									IpAddressFamily: "IPv4",
									IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
								},
							},
						}, nil
					})
			},
			wantStatus: agamodel.AcceleratorStatus{
				AcceleratorARN: "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
				DNSName:        "a1234567890abcdef.awsglobalaccelerator.com",
				Status:         "DEPLOYED",
				IPSets: []agamodel.IPSet{
					{
						IpAddressFamily: "IPv4",
						IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
					},
				},
			},
			wantError: false,
		},
		{
			name: "Update error case",
			resAccelerator: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, "ga", "test-accelerator"),
				Spec: agamodel.AcceleratorSpec{
					Name:          "updated-accelerator-name",
					IPAddressType: agamodel.IPAddressTypeIPV4,
					Enabled:       aws.Bool(true),
					Tags:          nil,
				},
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &agatypes.Accelerator{
					AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
					Name:           aws.String("original-accelerator-name"),
					IpAddressType:  agatypes.IpAddressTypeIpv4,
					Enabled:        aws.Bool(false),
				},
				Tags: map[string]string{
					"aga.k8s.aws/resource": "test-accelerator",
				},
			},
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator, mockAccManager *MockAcceleratorManager) {
				mockAccManager.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(agamodel.AcceleratorStatus{}, assert.AnError)
			},
			wantStatus: agamodel.AcceleratorStatus{},
			wantError:  true,
		},
		{
			name: "Update with IP address type change",
			resAccelerator: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, "ga", "test-accelerator"),
				Spec: agamodel.AcceleratorSpec{
					Name:          "test-accelerator",
					IPAddressType: agamodel.IPAddressTypeDualStack,
					Enabled:       aws.Bool(true),
					Tags:          nil,
				},
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &agatypes.Accelerator{
					AcceleratorArn:   aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
					Name:             aws.String("test-accelerator"),
					IpAddressType:    agatypes.IpAddressTypeIpv4,
					Enabled:          aws.Bool(true),
					DnsName:          aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
					DualStackDnsName: aws.String("a1234567890abcdef.dualstack.awsglobalaccelerator.com"),
					Status:           agatypes.AcceleratorStatusInProgress,
				},
				Tags: map[string]string{
					"aga.k8s.aws/resource": "test-accelerator",
				},
			},
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator, mockAccManager *MockAcceleratorManager) {
				mockAccManager.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, resAcc *agamodel.Accelerator, sdkAcc AcceleratorWithTags) (agamodel.AcceleratorStatus, error) {
						// Verify that the IP address type change is correctly passed to the Update method
						assert.Equal(t, agamodel.IPAddressTypeDualStack, resAcc.Spec.IPAddressType)
						assert.Equal(t, agatypes.IpAddressTypeIpv4, sdkAcc.Accelerator.IpAddressType)

						// Return the expected status for an in-progress update
						return agamodel.AcceleratorStatus{
							AcceleratorARN:   *sdkAcc.Accelerator.AcceleratorArn,
							DNSName:          *sdkAcc.Accelerator.DnsName,
							DualStackDNSName: *sdkAcc.Accelerator.DualStackDnsName,
							Status:           "IN_PROGRESS",
						}, nil
					})
			},
			wantStatus: agamodel.AcceleratorStatus{
				AcceleratorARN:   "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
				DNSName:          "a1234567890abcdef.awsglobalaccelerator.com",
				DualStackDNSName: "a1234567890abcdef.dualstack.awsglobalaccelerator.com",
				Status:           "IN_PROGRESS",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockGAClient := services.NewMockGlobalAccelerator(ctrl)
			mockTrackingProvider := tracking.NewMockProvider(ctrl)
			mockTaggingManager := NewMockTaggingManager(ctrl)
			mockAccManager := NewMockAcceleratorManager(ctrl)
			logger := logr.New(&log.NullLogSink{})

			// Setup expectations
			if tt.setupExpectations != nil {
				tt.setupExpectations(mockGAClient, mockAccManager)
			}

			// Create synthesizer
			synthesizer := &acceleratorSynthesizer{
				gaClient:           mockGAClient,
				trackingProvider:   mockTrackingProvider,
				taggingManager:     mockTaggingManager,
				acceleratorManager: mockAccManager,
				logger:             logger,
				stack:              stack,
			}

			// Run the method being tested
			err := synthesizer.handleUpdateAccelerator(context.Background(), tt.resAccelerator, tt.sdkAccelerator)

			// Assert expectations
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Check that status is set correctly
				if assert.NotNil(t, tt.resAccelerator.Status) {
					assert.Equal(t, tt.wantStatus, *tt.resAccelerator.Status)
				}
			}
		})
	}
}
