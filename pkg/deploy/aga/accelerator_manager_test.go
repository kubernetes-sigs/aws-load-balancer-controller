package aga

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	gatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultAcceleratorManager_buildSDKCreateAcceleratorInput(t *testing.T) {
	// Setup controller and mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup test resources
	mockGAService := &services.MockGlobalAccelerator{}
	mockTrackingProvider := tracking.NewMockProvider(ctrl)
	mockTaggingManager := NewMockTaggingManager(ctrl)
	logger := logr.New(&log.NullLogSink{})

	// Create a test stack
	stack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	// Create a mock Accelerator for testing
	createTestAccelerator := func(resName string, ipAddressType agamodel.IPAddressType, enabled *bool, tags map[string]string) *agamodel.Accelerator {
		// Create an Accelerator with fake CRD
		fakeCRD := &agaapi.GlobalAccelerator{}
		fakeCRD.UID = types.UID("test-uid-" + resName)

		acc := agamodel.NewAccelerator(stack, resName, agamodel.AcceleratorSpec{
			Name:          resName,
			IPAddressType: ipAddressType,
			Enabled:       enabled,
			Tags:          tags,
		}, fakeCRD)

		return acc
	}

	tests := []struct {
		name              string
		resAccelerator    *agamodel.Accelerator
		setupExpectations func()
		validateInput     func(*testing.T, *agamodel.Accelerator, *defaultAcceleratorManager)
	}{
		{
			name:           "Standard accelerator with minimal spec",
			resAccelerator: createTestAccelerator("test-accelerator", agamodel.IPAddressTypeIPV4, aws.Bool(true), nil),
			setupExpectations: func() {
				// Setup tracking provider expectations
				mockTrackingProvider.EXPECT().ResourceTags(gomock.Any(), gomock.Any(), gomock.Nil()).Return(map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
					"aga.k8s.aws/stack":     "test-namespace/test-name",
					"aga.k8s.aws/resource":  "test-accelerator",
				})

				// Setup tagging manager expectations
				expectedTags := map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
					"aga.k8s.aws/stack":     "test-namespace/test-name",
					"aga.k8s.aws/resource":  "test-accelerator",
				}
				mockTaggingManager.EXPECT().
					ConvertTagsToSDKTags(gomock.Eq(expectedTags)).
					Return([]gatypes.Tag{
						{
							Key:   aws.String("elbv2.k8s.aws/cluster"),
							Value: aws.String("test-cluster"),
						},
						{
							Key:   aws.String("aga.k8s.aws/stack"),
							Value: aws.String("test-namespace/test-name"),
						},
						{
							Key:   aws.String("aga.k8s.aws/resource"),
							Value: aws.String("test-accelerator"),
						},
					})
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, manager *defaultAcceleratorManager) {
				// Create input and validate fields
				input := manager.buildSDKCreateAcceleratorInput(context.Background(), resAccelerator)

				// Basic validations
				assert.Equal(t, "test-accelerator", *input.Name)
				assert.Equal(t, gatypes.IpAddressTypeIpv4, input.IpAddressType)
				assert.True(t, *input.Enabled)

				// Validate idempotency token is set properly
				assert.NotEmpty(t, *input.IdempotencyToken)

				// Validate tags are included
				expectedTagKeys := []string{"elbv2.k8s.aws/cluster", "aga.k8s.aws/stack", "aga.k8s.aws/resource"}
				for _, key := range expectedTagKeys {
					found := false
					for _, tag := range input.Tags {
						if *tag.Key == key {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected tag %s not found", key)
				}
			},
		},
		{
			name: "Accelerator with user tags",
			resAccelerator: createTestAccelerator("test-accelerator-with-tags", agamodel.IPAddressTypeIPV4, aws.Bool(true), map[string]string{
				"Environment": "test",
				"Application": "my-app",
			}),
			setupExpectations: func() {
				// Setup tracking provider expectations with user tags
				mockTrackingProvider.EXPECT().ResourceTags(gomock.Any(), gomock.Any(), gomock.Eq(map[string]string{
					"Environment": "test",
					"Application": "my-app",
				})).Return(map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
					"aga.k8s.aws/stack":     "test-namespace/test-name",
					"aga.k8s.aws/resource":  "test-accelerator",
					"Environment":           "test",
					"Application":           "my-app",
				})

				// Setup tagging manager expectations
				expectedTags := map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
					"aga.k8s.aws/stack":     "test-namespace/test-name",
					"aga.k8s.aws/resource":  "test-accelerator",
					"Environment":           "test",
					"Application":           "my-app",
				}
				mockTaggingManager.EXPECT().
					ConvertTagsToSDKTags(gomock.Eq(expectedTags)).
					Return([]gatypes.Tag{
						{
							Key:   aws.String("elbv2.k8s.aws/cluster"),
							Value: aws.String("test-cluster"),
						},
						{
							Key:   aws.String("aga.k8s.aws/stack"),
							Value: aws.String("test-namespace/test-name"),
						},
						{
							Key:   aws.String("aga.k8s.aws/resource"),
							Value: aws.String("test-accelerator"),
						},
						{
							Key:   aws.String("Environment"),
							Value: aws.String("test"),
						},
						{
							Key:   aws.String("Application"),
							Value: aws.String("my-app"),
						},
					})
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, manager *defaultAcceleratorManager) {
				// Create input and validate fields
				input := manager.buildSDKCreateAcceleratorInput(context.Background(), resAccelerator)

				// Basic validations
				assert.Equal(t, "test-accelerator-with-tags", *input.Name)
				assert.Equal(t, gatypes.IpAddressTypeIpv4, input.IpAddressType)
				assert.True(t, *input.Enabled)

				// Validate idempotency token is set properly
				assert.NotEmpty(t, *input.IdempotencyToken)

				// Validate tags are included (tracking tags + user tags)
				expectedTagKeys := []string{
					"elbv2.k8s.aws/cluster", "aga.k8s.aws/stack", "aga.k8s.aws/resource",
					"Environment", "Application",
				}

				for _, key := range expectedTagKeys {
					found := false
					for _, tag := range input.Tags {
						if *tag.Key == key {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected tag %s not found", key)
				}
			},
		},
		{
			name:           "Dual stack accelerator",
			resAccelerator: createTestAccelerator("test-dual-stack-accelerator", agamodel.IPAddressTypeDualStack, aws.Bool(true), nil),
			setupExpectations: func() {
				// Setup tracking provider expectations
				mockTrackingProvider.EXPECT().ResourceTags(gomock.Any(), gomock.Any(), gomock.Nil()).Return(map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
					"aga.k8s.aws/stack":     "test-namespace/test-name",
					"aga.k8s.aws/resource":  "test-accelerator",
				})

				// Setup tagging manager expectations
				expectedTags := map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
					"aga.k8s.aws/stack":     "test-namespace/test-name",
					"aga.k8s.aws/resource":  "test-accelerator",
				}
				mockTaggingManager.EXPECT().
					ConvertTagsToSDKTags(gomock.Eq(expectedTags)).
					Return([]gatypes.Tag{
						{
							Key:   aws.String("elbv2.k8s.aws/cluster"),
							Value: aws.String("test-cluster"),
						},
						{
							Key:   aws.String("aga.k8s.aws/stack"),
							Value: aws.String("test-namespace/test-name"),
						},
						{
							Key:   aws.String("aga.k8s.aws/resource"),
							Value: aws.String("test-accelerator"),
						},
					})
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, manager *defaultAcceleratorManager) {
				// Create input and validate fields
				input := manager.buildSDKCreateAcceleratorInput(context.Background(), resAccelerator)

				// Validate IP address type
				assert.Equal(t, gatypes.IpAddressTypeDualStack, input.IpAddressType)
			},
		},
		{
			name:           "Disabled accelerator",
			resAccelerator: createTestAccelerator("test-disabled-accelerator", agamodel.IPAddressTypeIPV4, aws.Bool(false), nil),
			setupExpectations: func() {
				// Setup tracking provider expectations
				mockTrackingProvider.EXPECT().ResourceTags(gomock.Any(), gomock.Any(), gomock.Nil()).Return(map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
					"aga.k8s.aws/stack":     "test-namespace/test-name",
					"aga.k8s.aws/resource":  "test-accelerator",
				})

				// Setup tagging manager expectations
				expectedTags := map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
					"aga.k8s.aws/stack":     "test-namespace/test-name",
					"aga.k8s.aws/resource":  "test-accelerator",
				}
				mockTaggingManager.EXPECT().
					ConvertTagsToSDKTags(gomock.Eq(expectedTags)).
					Return([]gatypes.Tag{
						{
							Key:   aws.String("elbv2.k8s.aws/cluster"),
							Value: aws.String("test-cluster"),
						},
						{
							Key:   aws.String("aga.k8s.aws/stack"),
							Value: aws.String("test-namespace/test-name"),
						},
						{
							Key:   aws.String("aga.k8s.aws/resource"),
							Value: aws.String("test-accelerator"),
						},
					})
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, manager *defaultAcceleratorManager) {
				// Create input and validate fields
				input := manager.buildSDKCreateAcceleratorInput(context.Background(), resAccelerator)

				// Validate enabled status is false
				assert.False(t, *input.Enabled)
			},
		},
		{
			name: "BYOIP accelerator with custom IP addresses",
			resAccelerator: func() *agamodel.Accelerator {
				fakeCRD := &agaapi.GlobalAccelerator{}
				fakeCRD.UID = types.UID("test-uid-byoip")
				acc := agamodel.NewAccelerator(stack, "test-byoip-accelerator", agamodel.AcceleratorSpec{
					Name:          "test-byoip-accelerator",
					IPAddressType: agamodel.IPAddressTypeIPV4,
					Enabled:       aws.Bool(true),
					IpAddresses:   []string{"192.0.2.1", "192.0.2.2"},
				}, fakeCRD)
				return acc
			}(),
			setupExpectations: func() {
				mockTrackingProvider.EXPECT().ResourceTags(gomock.Any(), gomock.Any(), gomock.Nil()).Return(map[string]string{
					"elbv2.k8s.aws/cluster": "test-cluster",
				})
				mockTaggingManager.EXPECT().ConvertTagsToSDKTags(gomock.Any()).Return([]gatypes.Tag{})
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, manager *defaultAcceleratorManager) {
				input := manager.buildSDKCreateAcceleratorInput(context.Background(), resAccelerator)
				assert.NotNil(t, input.IpAddresses)
				assert.Equal(t, []string{"192.0.2.1", "192.0.2.2"}, input.IpAddresses)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No need to reset gomock expectations as they're automatically reset

			// Setup expectations
			if tt.setupExpectations != nil {
				tt.setupExpectations()
			}

			// Create manager
			manager := &defaultAcceleratorManager{
				gaService:        mockGAService,
				trackingProvider: mockTrackingProvider,
				taggingManager:   mockTaggingManager,
				logger:           logger,
			}

			// No need to mock GetCRDUID as it's not used directly in this test

			// Run validation
			tt.validateInput(t, tt.resAccelerator, manager)

			// No need to verify gomock expectations as it's handled automatically when ctrl.Finish() is called
		})
	}
}

func Test_defaultAcceleratorManager_buildSDKUpdateAcceleratorInput(t *testing.T) {
	// Setup controller and mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup test resources
	mockGAService := &services.MockGlobalAccelerator{}
	mockTrackingProvider := tracking.NewMockProvider(ctrl)
	mockTaggingManager := NewMockTaggingManager(ctrl)
	logger := logr.New(&log.NullLogSink{})

	// Create a test stack
	stack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	// Create a mock Accelerator for testing
	createTestAccelerator := func(resName string, ipAddressType agamodel.IPAddressType, enabled *bool, tags map[string]string) *agamodel.Accelerator {
		// Create an Accelerator with fake CRD
		fakeCRD := &agaapi.GlobalAccelerator{}
		fakeCRD.UID = types.UID("test-uid-" + resName)

		acc := agamodel.NewAccelerator(stack, resName, agamodel.AcceleratorSpec{
			Name:          resName,
			IPAddressType: ipAddressType,
			Enabled:       enabled,
			Tags:          tags,
		}, fakeCRD)

		return acc
	}

	tests := []struct {
		name           string
		resAccelerator *agamodel.Accelerator
		sdkAccelerator AcceleratorWithTags
		validateInput  func(*testing.T, *agamodel.Accelerator, AcceleratorWithTags, *defaultAcceleratorManager)
	}{
		{
			name:           "Standard accelerator update",
			resAccelerator: createTestAccelerator("test-accelerator", agamodel.IPAddressTypeIPV4, aws.Bool(true), nil),
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
					Name:           aws.String("original-accelerator-name"),
					IpAddressType:  gatypes.IpAddressTypeIpv4,
					Enabled:        aws.Bool(true),
				},
				Tags: map[string]string{
					"aga.k8s.aws/resource": "test-accelerator",
				},
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags, manager *defaultAcceleratorManager) {
				// Create input and validate fields
				input := manager.buildSDKUpdateAcceleratorInput(context.Background(), resAccelerator, sdkAccelerator)

				// Basic validations
				assert.Equal(t, "test-accelerator", *input.Name)
				assert.Equal(t, gatypes.IpAddressTypeIpv4, input.IpAddressType)
				assert.True(t, *input.Enabled)
				assert.Equal(t, *sdkAccelerator.Accelerator.AcceleratorArn, *input.AcceleratorArn)
			},
		},
		{
			name:           "Change IP address type",
			resAccelerator: createTestAccelerator("test-accelerator-dual-stack", agamodel.IPAddressTypeDualStack, aws.Bool(true), nil),
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
					Name:           aws.String("test-accelerator"),
					IpAddressType:  gatypes.IpAddressTypeIpv4,
					Enabled:        aws.Bool(true),
				},
				Tags: map[string]string{
					"aga.k8s.aws/resource": "test-accelerator",
				},
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags, manager *defaultAcceleratorManager) {
				// Create input and validate fields
				input := manager.buildSDKUpdateAcceleratorInput(context.Background(), resAccelerator, sdkAccelerator)

				// Validate IP address type is changed to dual stack
				assert.Equal(t, gatypes.IpAddressTypeDualStack, input.IpAddressType)
			},
		},
		{
			name:           "Disable accelerator",
			resAccelerator: createTestAccelerator("test-disabled-accelerator", agamodel.IPAddressTypeIPV4, aws.Bool(false), nil),
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
					Name:           aws.String("test-accelerator"),
					IpAddressType:  gatypes.IpAddressTypeIpv4,
					Enabled:        aws.Bool(true),
				},
				Tags: map[string]string{
					"aga.k8s.aws/resource": "test-accelerator",
				},
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags, manager *defaultAcceleratorManager) {
				// Create input and validate fields
				input := manager.buildSDKUpdateAcceleratorInput(context.Background(), resAccelerator, sdkAccelerator)

				// Validate enabled status changed to false
				assert.False(t, *input.Enabled)
			},
		},
		{
			name: "BYOIP not supported in update",
			resAccelerator: func() *agamodel.Accelerator {
				fakeCRD := &agaapi.GlobalAccelerator{}
				fakeCRD.UID = types.UID("test-uid-byoip-update")
				acc := agamodel.NewAccelerator(stack, "test-byoip-update", agamodel.AcceleratorSpec{
					Name:          "test-byoip-update",
					IPAddressType: agamodel.IPAddressTypeIPV4,
					Enabled:       aws.Bool(true),
					IpAddresses:   []string{"192.0.2.1", "192.0.2.2"},
				}, fakeCRD)
				return acc
			}(),
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
					Name:           aws.String("test-byoip-update"),
					IpAddressType:  gatypes.IpAddressTypeIpv4,
					Enabled:        aws.Bool(true),
					IpSets: []gatypes.IpSet{
						{
							IpAddressFamily: gatypes.IpAddressFamilyIPv4,
							IpAddresses:     []string{"192.0.2.3", "192.0.2.4"},
						},
					},
				},
				Tags: map[string]string{
					"aga.k8s.aws/resource": "test-accelerator",
				},
			},
			validateInput: func(t *testing.T, resAccelerator *agamodel.Accelerator, sdkAccelerator AcceleratorWithTags, manager *defaultAcceleratorManager) {
				input := manager.buildSDKUpdateAcceleratorInput(context.Background(), resAccelerator, sdkAccelerator)
				assert.Nil(t, input.IpAddresses, "IP addresses should not be included in update input")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create manager
			manager := &defaultAcceleratorManager{
				gaService:        mockGAService,
				trackingProvider: mockTrackingProvider,
				taggingManager:   mockTaggingManager,
				logger:           logger,
			}

			// Run validation
			tt.validateInput(t, tt.resAccelerator, tt.sdkAccelerator, manager)
		})
	}
}

func Test_defaultAcceleratorManager_buildAcceleratorStatus(t *testing.T) {
	// Setup controller and mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup test resources
	mockGAService := &services.MockGlobalAccelerator{}
	mockTrackingProvider := tracking.NewMockProvider(ctrl)
	mockTaggingManager := NewMockTaggingManager(ctrl)
	logger := logr.New(&log.NullLogSink{})

	manager := &defaultAcceleratorManager{
		gaService:        mockGAService,
		trackingProvider: mockTrackingProvider,
		taggingManager:   mockTaggingManager,
		logger:           logger,
	}

	tests := []struct {
		name        string
		accelerator *gatypes.Accelerator
		want        agamodel.AcceleratorStatus
	}{
		{
			name: "Basic accelerator status",
			accelerator: &gatypes.Accelerator{
				AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
				Name:           aws.String("test-accelerator"),
				DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
				Status:         gatypes.AcceleratorStatusDeployed,
				IpSets: []gatypes.IpSet{
					{
						IpAddressFamily: gatypes.IpAddressFamilyIPv4,
						IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
					},
				},
			},
			want: agamodel.AcceleratorStatus{
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
		},
		{
			name: "Dual stack accelerator status",
			accelerator: &gatypes.Accelerator{
				AcceleratorArn:   aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
				Name:             aws.String("test-accelerator"),
				DnsName:          aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
				DualStackDnsName: aws.String("a1234567890abcdef.dualstack.awsglobalaccelerator.com"),
				Status:           gatypes.AcceleratorStatusDeployed,
				IpSets: []gatypes.IpSet{
					{
						IpAddressFamily: gatypes.IpAddressFamilyIPv4,
						IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
					},
					{
						IpAddressFamily: gatypes.IpAddressFamilyIPv6,
						IpAddresses:     []string{"2001:db8::1", "2001:db8::2"},
					},
				},
			},
			want: agamodel.AcceleratorStatus{
				AcceleratorARN:   "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
				DNSName:          "a1234567890abcdef.awsglobalaccelerator.com",
				DualStackDNSName: "a1234567890abcdef.dualstack.awsglobalaccelerator.com",
				Status:           "DEPLOYED",
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
		},
		{
			name: "In progress accelerator status",
			accelerator: &gatypes.Accelerator{
				AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"),
				Name:           aws.String("test-accelerator"),
				DnsName:        aws.String("a1234567890abcdef.awsglobalaccelerator.com"),
				Status:         gatypes.AcceleratorStatusInProgress,
				IpSets:         []gatypes.IpSet{},
			},
			want: agamodel.AcceleratorStatus{
				AcceleratorARN: "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
				DNSName:        "a1234567890abcdef.awsglobalaccelerator.com",
				Status:         "IN_PROGRESS",
				IPSets:         []agamodel.IPSet{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.buildAcceleratorStatus(tt.accelerator)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultAcceleratorManager_disableAccelerator(t *testing.T) {
	// Setup controller and mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Test ARN
	testARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"

	tests := []struct {
		name              string
		setupExpectations func(mockGAClient *services.MockGlobalAccelerator)
		expectedResult    bool
		expectedError     bool
	}{
		{
			name: "Accelerator not found (already deleted)",
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Mock DescribeAcceleratorWithContext to return AcceleratorNotFoundException
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(nil, &gatypes.AcceleratorNotFoundException{
						Message: aws.String("Accelerator not found"),
					})
			},
			expectedResult: true,  // true indicates accelerator is already deleted
			expectedError:  false, // no error should be returned
		},
		{
			name: "Accelerator already disabled",
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Mock DescribeAcceleratorWithContext to return an already disabled accelerator
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &gatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator"),
							Enabled:        aws.Bool(false), // Already disabled
						},
					}, nil)
			},
			expectedResult: false, // false indicates accelerator exists but no disable operation needed
			expectedError:  false, // no error should be returned
		},
		{
			name: "Accelerator enabled, successfully disabled",
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Mock DescribeAcceleratorWithContext to return an enabled accelerator
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &gatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator"),
							Enabled:        aws.Bool(true), // Enabled, needs disabling
						},
					}, nil)

				// Mock UpdateAcceleratorWithContext to successfully disable the accelerator
				mockGAClient.EXPECT().
					UpdateAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.UpdateAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
						Enabled:        aws.Bool(false),
					})).
					Return(&globalaccelerator.UpdateAcceleratorOutput{
						Accelerator: &gatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator"),
							Enabled:        aws.Bool(false), // Now disabled
						},
					}, nil)
			},
			expectedResult: false, // false indicates accelerator exists and was disabled
			expectedError:  false, // no error should be returned
		},
		{
			name: "Error when describing accelerator",
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Mock DescribeAcceleratorWithContext to return an error
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(nil, errors.New("unexpected error"))
			},
			expectedResult: false, // false in error case
			expectedError:  true,  // error should be returned
		},
		{
			name: "Error when updating/disabling accelerator",
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Mock DescribeAcceleratorWithContext to return an enabled accelerator
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &gatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator"),
							Enabled:        aws.Bool(true), // Enabled, needs disabling
						},
					}, nil)

				// Mock UpdateAcceleratorWithContext to fail
				mockGAClient.EXPECT().
					UpdateAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.UpdateAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
						Enabled:        aws.Bool(false),
					})).
					Return(nil, errors.New("failed to update accelerator"))
			},
			expectedResult: false, // false in error case
			expectedError:  true,  // error should be returned
		},
		{
			name: "Accelerator with nil enabled field",
			setupExpectations: func(mockGAClient *services.MockGlobalAccelerator) {
				// Mock DescribeAcceleratorWithContext to return an accelerator with nil enabled field
				mockGAClient.EXPECT().
					DescribeAcceleratorWithContext(gomock.Any(), gomock.Eq(&globalaccelerator.DescribeAcceleratorInput{
						AcceleratorArn: aws.String(testARN),
					})).
					Return(&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &gatypes.Accelerator{
							AcceleratorArn: aws.String(testARN),
							Name:           aws.String("test-accelerator"),
							Enabled:        nil, // nil field should be treated as disabled
						},
					}, nil)
			},
			expectedResult: false, // false indicates accelerator exists but no disable operation needed
			expectedError:  false, // no error should be returned
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockGAClient := services.NewMockGlobalAccelerator(ctrl)
			mockTrackingProvider := tracking.NewMockProvider(ctrl)
			mockTaggingManager := NewMockTaggingManager(ctrl)
			logger := logr.New(&log.NullLogSink{})

			// Setup expectations
			if tt.setupExpectations != nil {
				tt.setupExpectations(mockGAClient)
			}

			// Create manager
			manager := &defaultAcceleratorManager{
				gaService:        mockGAClient,
				trackingProvider: mockTrackingProvider,
				taggingManager:   mockTaggingManager,
				logger:           logger,
			}

			// Call the method being tested
			result, err := manager.disableAccelerator(context.Background(), testARN)

			// Assert results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func Test_defaultAcceleratorManager_isSDKAcceleratorSettingsDrifted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	stack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	tests := []struct {
		name           string
		resSpec        agamodel.AcceleratorSpec
		sdkAccelerator AcceleratorWithTags
		expectedDrift  bool
	}{
		{
			name: "No drift - all fields match",
			resSpec: agamodel.AcceleratorSpec{
				Name:          "test-accelerator",
				IPAddressType: agamodel.IPAddressTypeIPV4,
				Enabled:       aws.Bool(true),
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					Name:          aws.String("test-accelerator"),
					IpAddressType: gatypes.IpAddressTypeIpv4,
					Enabled:       aws.Bool(true),
				},
			},
			expectedDrift: false,
		},
		{
			name: "Name differs - drift detected",
			resSpec: agamodel.AcceleratorSpec{
				Name:          "new-name",
				IPAddressType: agamodel.IPAddressTypeIPV4,
				Enabled:       aws.Bool(true),
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					Name:          aws.String("old-name"),
					IpAddressType: gatypes.IpAddressTypeIpv4,
					Enabled:       aws.Bool(true),
				},
			},
			expectedDrift: true,
		},
		{
			name: "IP address type differs - drift detected",
			resSpec: agamodel.AcceleratorSpec{
				Name:          "test-accelerator",
				IPAddressType: agamodel.IPAddressTypeDualStack,
				Enabled:       aws.Bool(true),
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					Name:          aws.String("test-accelerator"),
					IpAddressType: gatypes.IpAddressTypeIpv4,
					Enabled:       aws.Bool(true),
				},
			},
			expectedDrift: true,
		},
		{
			name: "Enabled state differs - drift detected",
			resSpec: agamodel.AcceleratorSpec{
				Name:          "test-accelerator",
				IPAddressType: agamodel.IPAddressTypeIPV4,
				Enabled:       aws.Bool(false),
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					Name:          aws.String("test-accelerator"),
					IpAddressType: gatypes.IpAddressTypeIpv4,
					Enabled:       aws.Bool(true),
				},
			},
			expectedDrift: true,
		},
		{
			name: "IP addresses match - no drift",
			resSpec: agamodel.AcceleratorSpec{
				Name:          "test-accelerator",
				IPAddressType: agamodel.IPAddressTypeIPV4,
				Enabled:       aws.Bool(true),
				IpAddresses:   []string{"192.0.2.1", "192.0.2.2"},
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					Name:          aws.String("test-accelerator"),
					IpAddressType: gatypes.IpAddressTypeIpv4,
					Enabled:       aws.Bool(true),
					IpSets: []gatypes.IpSet{
						{IpAddresses: []string{"192.0.2.1", "192.0.2.2"}},
					},
				},
			},
			expectedDrift: false,
		},
		{
			name: "IP addresses differ - no drift but logs warning",
			resSpec: agamodel.AcceleratorSpec{
				Name:          "test-accelerator",
				IPAddressType: agamodel.IPAddressTypeIPV4,
				Enabled:       aws.Bool(true),
				IpAddresses:   []string{"192.0.2.1", "192.0.2.2"},
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					Name:          aws.String("test-accelerator"),
					IpAddressType: gatypes.IpAddressTypeIpv4,
					Enabled:       aws.Bool(true),
					IpSets: []gatypes.IpSet{
						{IpAddresses: []string{"192.0.2.3", "192.0.2.4"}},
					},
				},
			},
			expectedDrift: false,
		},
		{
			name: "Multiple fields differ - drift detected",
			resSpec: agamodel.AcceleratorSpec{
				Name:          "new-name",
				IPAddressType: agamodel.IPAddressTypeDualStack,
				Enabled:       aws.Bool(false),
			},
			sdkAccelerator: AcceleratorWithTags{
				Accelerator: &gatypes.Accelerator{
					Name:          aws.String("old-name"),
					IpAddressType: gatypes.IpAddressTypeIpv4,
					Enabled:       aws.Bool(true),
				},
			},
			expectedDrift: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockGAClient := services.NewMockGlobalAccelerator(ctrl)
			mockTrackingProvider := tracking.NewMockProvider(ctrl)
			mockTaggingManager := NewMockTaggingManager(ctrl)
			logger := logr.New(&log.NullLogSink{})

			manager := &defaultAcceleratorManager{
				gaService:        mockGAClient,
				trackingProvider: mockTrackingProvider,
				taggingManager:   mockTaggingManager,
				logger:           logger,
			}

			fakeCRD := &agaapi.GlobalAccelerator{}
			fakeCRD.UID = types.UID("test-uid")

			resAccelerator := agamodel.NewAccelerator(stack, "test-accelerator", tt.resSpec, fakeCRD)

			drifted := manager.isSDKAcceleratorSettingsDrifted(resAccelerator, tt.sdkAccelerator)
			assert.Equal(t, tt.expectedDrift, drifted)
		})
	}
}

func Test_defaultAcceleratorManager_areIPv4AddressesEqual(t *testing.T) {
	tests := []struct {
		name           string
		desiredIPs     []string
		actualIPSets   []agatypes.IpSet
		expectedResult bool
		description    string
	}{
		{
			name:           "Two BYOIPs match current - no drift",
			desiredIPs:     []string{"169.254.8.13", "169.254.8.14"},
			actualIPSets:   makeIPSets([]string{"169.254.8.13", "169.254.8.14"}),
			expectedResult: true,
			description:    "Both BYOIPs match exactly",
		},
		{
			name:           "One BYOIP matches current - drift",
			desiredIPs:     []string{"169.254.8.13, 169.254.9.13"},
			actualIPSets:   makeIPSets([]string{"169.254.8.13", "99.82.158.217"}),
			expectedResult: false,
			description:    "BYOIP present, Amazon auto-assigned the other",
		},
		{
			name:           "One BYOIP missing from current - drift detected",
			desiredIPs:     []string{"169.254.9.13"},
			actualIPSets:   makeIPSets([]string{"169.254.8.13", "99.82.158.217"}),
			expectedResult: false,
			description:    "Desired BYOIP not in current",
		},
		{
			name:           "Changing from one BYOIP to another - drift detected",
			desiredIPs:     []string{"169.254.8.16", "99.82.158.217"},
			actualIPSets:   makeIPSets([]string{"169.254.9.13", "99.82.158.217"}),
			expectedResult: false,
			description:    "Different BYOIP, count matches but IP doesn't",
		},
		{
			name:           "Two BYOIPs in different order - no drift",
			desiredIPs:     []string{"169.254.8.14", "169.254.8.13"},
			actualIPSets:   makeIPSets([]string{"169.254.8.13", "169.254.8.14"}),
			expectedResult: true,
			description:    "Order doesn't matter for exact match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultAcceleratorManager{
				logger: logr.Discard(),
			}
			result := m.areIPv4AddressesEqual(tt.desiredIPs, tt.actualIPSets)
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

func makeIPSets(ips []string) []agatypes.IpSet {
	if len(ips) == 0 {
		return []agatypes.IpSet{}
	}
	return []agatypes.IpSet{
		{
			IpAddressFamily: "IPv4",
			IpAddresses:     ips,
		},
	}
}
