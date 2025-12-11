package aga

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultStatusUpdater_UpdateStatusSuccess(t *testing.T) {
	// Setup test cases
	tests := []struct {
		name           string
		ga             *v1beta1.GlobalAccelerator
		accelerator    *agamodel.Accelerator
		wantRequeue    bool
		validateStatus func(t *testing.T, ga *v1beta1.GlobalAccelerator)
	}{
		{
			name: "Successfully update deployed accelerator status",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga",
					Namespace:  "default",
					Generation: 2,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: nil,
					Conditions:         []metav1.Condition{},
				},
			},
			accelerator: &agamodel.Accelerator{
				Status: &agamodel.AcceleratorStatus{
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
			wantRequeue: false,
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Check that status fields were updated correctly
				assert.NotNil(t, ga.Status.ObservedGeneration)
				assert.Equal(t, int64(2), *ga.Status.ObservedGeneration)
				assert.Equal(t, "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh", *ga.Status.AcceleratorARN)
				assert.Equal(t, "a1234567890abcdef.awsglobalaccelerator.com", *ga.Status.DNSName)
				assert.Equal(t, "DEPLOYED", *ga.Status.Status)

				// Check that the condition was added correctly
				assert.Len(t, ga.Status.Conditions, 1)
				condition := ga.Status.Conditions[0]
				assert.Equal(t, ConditionTypeReady, condition.Type)
				assert.Equal(t, metav1.ConditionTrue, condition.Status)
				assert.Equal(t, ReasonAcceleratorReady, condition.Reason)
			},
		},
		{
			name: "Successfully update in-progress accelerator status",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga-in-progress",
					Namespace:  "default",
					Generation: 2,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: nil,
					Conditions:         []metav1.Condition{},
				},
			},
			accelerator: &agamodel.Accelerator{
				Status: &agamodel.AcceleratorStatus{
					AcceleratorARN: "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh",
					DNSName:        "a1234567890abcdef.awsglobalaccelerator.com",
					Status:         "IN_PROGRESS", // Still provisioning
					IPSets: []agamodel.IPSet{
						{
							IpAddressFamily: "IPv4",
							IpAddresses:     []string{"192.0.2.250", "198.51.100.52"},
						},
					},
				},
			},
			wantRequeue: true, // Should requeue to check status again
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Check that status fields were updated correctly
				assert.NotNil(t, ga.Status.ObservedGeneration)
				assert.Equal(t, int64(2), *ga.Status.ObservedGeneration)
				assert.Equal(t, "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh", *ga.Status.AcceleratorARN)
				assert.Equal(t, "a1234567890abcdef.awsglobalaccelerator.com", *ga.Status.DNSName)
				assert.Equal(t, "IN_PROGRESS", *ga.Status.Status)

				// Check that the condition was added correctly - should be Unknown while provisioning
				assert.Len(t, ga.Status.Conditions, 1)
				condition := ga.Status.Conditions[0]
				assert.Equal(t, ConditionTypeReady, condition.Type)
				assert.Equal(t, metav1.ConditionUnknown, condition.Status)
				assert.Equal(t, ReasonAcceleratorProvisioning, condition.Reason)
			},
		},
		{
			name: "Update dual-stack accelerator status",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga-dual-stack",
					Namespace:  "default",
					Generation: 2,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: nil,
					Conditions:         []metav1.Condition{},
				},
			},
			accelerator: &agamodel.Accelerator{
				Status: &agamodel.AcceleratorStatus{
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
			wantRequeue: false,
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Check that dual-stack DNS name was updated correctly
				assert.NotNil(t, ga.Status.DualStackDnsName)
				assert.Equal(t, "a1234567890abcdef.dualstack.awsglobalaccelerator.com", *ga.Status.DualStackDnsName)

				// Check IP sets were copied correctly
				assert.Len(t, ga.Status.IPSets, 2)
				assert.Equal(t, "IPv4", *ga.Status.IPSets[0].IpAddressFamily)
				assert.Equal(t, "IPv6", *ga.Status.IPSets[1].IpAddressFamily)
			},
		},
		{
			name: "Skip update when already in sync",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga-in-sync",
					Namespace:  "default",
					Generation: 2,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: func() *int64 { i := int64(2); return &i }(),
					AcceleratorARN: func() *string {
						s := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd-abcd-1234-abcd-1234abcdefgh"
						return &s
					}(),
					DNSName: func() *string { s := "a1234567890abcdef.awsglobalaccelerator.com"; return &s }(),
					Status:  func() *string { s := "DEPLOYED"; return &s }(),
					Conditions: []metav1.Condition{
						{
							Type:               ConditionTypeReady,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.Now(),
							Reason:             ReasonAcceleratorReady,
							Message:            "GlobalAccelerator is ready and available",
						},
					},
					IPSets: []v1beta1.IPSet{
						{
							IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
							IpAddresses:     func() *[]string { s := []string{"192.0.2.250", "198.51.100.52"}; return &s }(),
						},
					},
				},
			},
			accelerator: &agamodel.Accelerator{
				Status: &agamodel.AcceleratorStatus{
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
			wantRequeue: false,
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Status should be unchanged
				assert.NotNil(t, ga.Status.ObservedGeneration)
				assert.Equal(t, int64(2), *ga.Status.ObservedGeneration)
			},
		},
		{
			name: "Handle nil accelerator status",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga-nil-status",
					Namespace:  "default",
					Generation: 2,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: nil,
					Conditions:         []metav1.Condition{},
				},
			},
			accelerator: &agamodel.Accelerator{
				Status: nil, // Nil status
			},
			wantRequeue: false,
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Status should remain unchanged
				assert.Nil(t, ga.Status.ObservedGeneration)
				assert.Empty(t, ga.Status.Conditions)
			},
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client and register the GlobalAccelerator CRD
			k8sClient := testutils.GenerateTestClient()

			// For the test cases that expect success, create the object in the API server first
			// Skip this for "Skip update when already in sync" and "Handle nil accelerator status" since they don't patch
			if tt.name != "Skip update when already in sync" && tt.name != "Handle nil accelerator status" {
				err := k8sClient.Create(context.Background(), tt.ga)
				if err != nil {
					t.Fatalf("Failed to create test object: %v", err)
				}
			}

			// Create status updater
			updater := &defaultStatusUpdater{
				k8sClient: k8sClient,
				logger:    logr.New(&log.NullLogSink{}),
			}

			// Call method being tested
			gotRequeue, err := updater.UpdateStatusSuccess(context.Background(), tt.ga, tt.accelerator)

			// Check error - we expect errors for tests without pre-created objects
			if tt.name == "Skip update when already in sync" || tt.name == "Handle nil accelerator status" {
				// These tests should pass without patching
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantRequeue, gotRequeue)

			// Validate the resulting status
			if tt.validateStatus != nil {
				tt.validateStatus(t, tt.ga)
			}
		})
	}
}

func Test_defaultStatusUpdater_UpdateStatusFailure(t *testing.T) {
	// Setup test cases
	tests := []struct {
		name           string
		ga             *v1beta1.GlobalAccelerator
		reason         string
		message        string
		validateStatus func(t *testing.T, ga *v1beta1.GlobalAccelerator)
	}{
		{
			name: "Update status with failure reason",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga-failure",
					Namespace:  "default",
					Generation: 3,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: nil,
					Conditions:         []metav1.Condition{},
				},
			},
			reason:  "ProvisioningFailed",
			message: "Reconciliation failed. See events and controller logs for details",
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Check that observed generation was updated
				assert.NotNil(t, ga.Status.ObservedGeneration)
				assert.Equal(t, int64(3), *ga.Status.ObservedGeneration)

				// Check that the failure condition was added correctly
				assert.Len(t, ga.Status.Conditions, 1)
				condition := ga.Status.Conditions[0]
				assert.Equal(t, ConditionTypeReady, condition.Type)
				assert.Equal(t, metav1.ConditionFalse, condition.Status)
				assert.Equal(t, "ProvisioningFailed", condition.Reason)
				assert.Equal(t, "Reconciliation failed. See events and controller logs for details", condition.Message)
			},
		},
		{
			name: "Update existing failure condition",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga-existing-failure",
					Namespace:  "default",
					Generation: 3,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: func() *int64 { i := int64(2); return &i }(),
					Conditions: []metav1.Condition{
						{
							Type:               ConditionTypeReady,
							Status:             metav1.ConditionFalse,
							LastTransitionTime: metav1.Now(),
							Reason:             "OldError",
							Message:            "Old error message",
						},
					},
				},
			},
			reason:  "NewError",
			message: "Reconciliation failed. See events and controller logs for details",
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Check that observed generation was updated
				assert.NotNil(t, ga.Status.ObservedGeneration)
				assert.Equal(t, int64(3), *ga.Status.ObservedGeneration)

				// Check that the failure condition was updated correctly
				assert.Len(t, ga.Status.Conditions, 1)
				condition := ga.Status.Conditions[0]
				assert.Equal(t, ConditionTypeReady, condition.Type)
				assert.Equal(t, metav1.ConditionFalse, condition.Status)
				assert.Equal(t, "NewError", condition.Reason)
				assert.Equal(t, "Reconciliation failed. See events and controller logs for details", condition.Message)
			},
		},
		{
			name: "Skip update when already in sync",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga-in-sync",
					Namespace:  "default",
					Generation: 3,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: func() *int64 { i := int64(3); return &i }(),
					Conditions: []metav1.Condition{
						{
							Type:               ConditionTypeReady,
							Status:             metav1.ConditionFalse,
							LastTransitionTime: metav1.Now(),
							Reason:             "SameError",
							Message:            "Reconciliation failed. See events and controller logs for details",
						},
					},
				},
			},
			reason:  "SameError",
			message: "Reconciliation failed. See events and controller logs for details",
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Status should be unchanged
				assert.NotNil(t, ga.Status.ObservedGeneration)
				assert.Equal(t, int64(3), *ga.Status.ObservedGeneration)
			},
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client using testutils
			k8sClient := testutils.GenerateTestClient()

			// For the test cases that expect success, create the object in the API server first
			// Skip this for "Skip update when already in sync" since it doesn't patch
			if tt.name != "Skip update when already in sync" {
				err := k8sClient.Create(context.Background(), tt.ga)
				if err != nil {
					t.Fatalf("Failed to create test object: %v", err)
				}
			}

			// Create status updater
			updater := &defaultStatusUpdater{
				k8sClient: k8sClient,
				logger:    logr.New(&log.NullLogSink{}),
			}

			// Call method being tested
			err := updater.UpdateStatusFailure(context.Background(), tt.ga, tt.reason, tt.message)

			// Check error - we expect errors for tests without pre-created objects
			if tt.name == "Skip update when already in sync" {
				// This test should pass without patching
				assert.NoError(t, err)
			}

			// Validate the resulting status
			if tt.validateStatus != nil {
				tt.validateStatus(t, tt.ga)
			}
		})
	}
}

func Test_defaultStatusUpdater_UpdateStatusDeletion(t *testing.T) {
	// Setup test cases
	tests := []struct {
		name           string
		ga             *v1beta1.GlobalAccelerator
		validateStatus func(t *testing.T, ga *v1beta1.GlobalAccelerator)
	}{
		{
			name: "Update status for deletion",
			ga: &v1beta1.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-ga-deleting",
					Namespace:  "default",
					Generation: 4,
				},
				Status: v1beta1.GlobalAcceleratorStatus{
					ObservedGeneration: func() *int64 { i := int64(3); return &i }(),
					Status:             func() *string { s := StatusDeployed; return &s }(),
					Conditions: []metav1.Condition{
						{
							Type:               ConditionTypeReady,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.Now(),
							Reason:             ReasonAcceleratorReady,
							Message:            "GlobalAccelerator is ready and available",
						},
					},
				},
			},
			validateStatus: func(t *testing.T, ga *v1beta1.GlobalAccelerator) {
				// Check that observed generation was updated
				assert.NotNil(t, ga.Status.ObservedGeneration)
				assert.Equal(t, int64(4), *ga.Status.ObservedGeneration)

				// Check that status was changed to "Deleting"
				assert.NotNil(t, ga.Status.Status)
				assert.Equal(t, StatusDeleting, *ga.Status.Status)

				// Check that conditions were added correctly
				assert.Len(t, ga.Status.Conditions, 2)

				// Find conditions by type
				var readyCondition, disablingCondition *metav1.Condition
				for i := range ga.Status.Conditions {
					if ga.Status.Conditions[i].Type == ConditionTypeReady {
						readyCondition = &ga.Status.Conditions[i]
					} else if ga.Status.Conditions[i].Type == ConditionTypeAcceleratorDisabling {
						disablingCondition = &ga.Status.Conditions[i]
					}
				}

				// Check Ready condition
				assert.NotNil(t, readyCondition)
				assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
				assert.Equal(t, ReasonAcceleratorDeleting, readyCondition.Reason)

				// Check AcceleratorDisabling condition
				assert.NotNil(t, disablingCondition)
				assert.Equal(t, metav1.ConditionTrue, disablingCondition.Status)
				assert.Equal(t, ReasonAcceleratorDisabling, disablingCondition.Reason)
			},
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client using testutils
			k8sClient := testutils.GenerateTestClient()

			// Create the object in the API server first
			err := k8sClient.Create(context.Background(), tt.ga)
			if err != nil {
				t.Fatalf("Failed to create test object: %v", err)
			}

			// Create status updater
			updater := &defaultStatusUpdater{
				k8sClient: k8sClient,
				logger:    logr.New(&log.NullLogSink{}),
			}

			// Call method being tested
			err = updater.UpdateStatusDeletion(context.Background(), tt.ga)

			// Validate the resulting status
			if tt.validateStatus != nil {
				tt.validateStatus(t, tt.ga)
			}
		})
	}
}

func Test_defaultStatusUpdater_updateCondition(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name           string
		conditions     *[]metav1.Condition
		newCondition   metav1.Condition
		wantChanged    bool
		wantConditions []metav1.Condition
	}{
		{
			name:       "Add condition to nil slice",
			conditions: nil,
			newCondition: metav1.Condition{
				Type:               "TestType",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "TestReason",
				Message:            "Test message",
			},
			wantChanged: true,
			wantConditions: []metav1.Condition{
				{
					Type:               "TestType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestReason",
					Message:            "Test message",
				},
			},
		},
		{
			name:       "Add condition to empty slice",
			conditions: &[]metav1.Condition{},
			newCondition: metav1.Condition{
				Type:               "TestType",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "TestReason",
				Message:            "Test message",
			},
			wantChanged: true,
			wantConditions: []metav1.Condition{
				{
					Type:               "TestType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "TestReason",
					Message:            "Test message",
				},
			},
		},
		{
			name: "Update existing condition",
			conditions: &[]metav1.Condition{
				{
					Type:               "TestType",
					Status:             metav1.ConditionFalse,
					LastTransitionTime: metav1.Now(),
					Reason:             "OldReason",
					Message:            "Old message",
				},
				{
					Type:               "OtherType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "OtherReason",
					Message:            "Other message",
				},
			},
			newCondition: metav1.Condition{
				Type:               "TestType",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "NewReason",
				Message:            "New message",
			},
			wantChanged: true,
			wantConditions: []metav1.Condition{
				{
					Type:               "TestType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "NewReason",
					Message:            "New message",
				},
				{
					Type:               "OtherType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "OtherReason",
					Message:            "Other message",
				},
			},
		},
		{
			name: "No change to existing condition",
			conditions: &[]metav1.Condition{
				{
					Type:               "TestType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "SameReason",
					Message:            "Same message",
				},
			},
			newCondition: metav1.Condition{
				Type:               "TestType",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "SameReason",
				Message:            "Same message",
			},
			wantChanged: false,
			wantConditions: []metav1.Condition{
				{
					Type:               "TestType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "SameReason",
					Message:            "Same message",
				},
			},
		},
		{
			name: "Add new condition type",
			conditions: &[]metav1.Condition{
				{
					Type:               "ExistingType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "ExistingReason",
					Message:            "Existing message",
				},
			},
			newCondition: metav1.Condition{
				Type:               "NewType",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "NewReason",
				Message:            "New message",
			},
			wantChanged: true,
			wantConditions: []metav1.Condition{
				{
					Type:               "ExistingType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "ExistingReason",
					Message:            "Existing message",
				},
				{
					Type:               "NewType",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "NewReason",
					Message:            "New message",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create status updater with testutils client
			updater := &defaultStatusUpdater{
				k8sClient: testutils.GenerateTestClient(),
				logger:    logr.New(&log.NullLogSink{}),
			}

			// Initialize conditions variable if it's nil to avoid nil pointer dereference
			var localConditions *[]metav1.Condition
			if tt.conditions == nil {
				localConditions = &[]metav1.Condition{}
			} else {
				localConditions = tt.conditions
			}

			// Call the method being tested
			gotChanged := updater.updateCondition(localConditions, tt.newCondition)

			// Check if changed flag matches expected
			assert.Equal(t, tt.wantChanged, gotChanged)

			// Check if conditions match expected
			assert.Equal(t, len(tt.wantConditions), len(*localConditions))

			// Check each condition in the slice
			for i, wantCondition := range tt.wantConditions {
				gotCondition := (*localConditions)[i]
				assert.Equal(t, wantCondition.Type, gotCondition.Type)
				assert.Equal(t, wantCondition.Status, gotCondition.Status)
				assert.Equal(t, wantCondition.Reason, gotCondition.Reason)
				assert.Equal(t, wantCondition.Message, gotCondition.Message)
			}
		})
	}
}

func Test_defaultStatusUpdater_areIPSetsEqual(t *testing.T) {
	tests := []struct {
		name     string
		existing []v1beta1.IPSet
		new      []v1beta1.IPSet
		want     bool
	}{
		{
			name: "Equal IP sets",
			existing: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.1", "198.51.100.1"}; return &s }(),
				},
			},
			new: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.1", "198.51.100.1"}; return &s }(),
				},
			},
			want: true,
		},
		{
			name: "Different IP addresses",
			existing: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.1", "198.51.100.1"}; return &s }(),
				},
			},
			new: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.2", "198.51.100.2"}; return &s }(),
				},
			},
			want: false,
		},
		{
			name: "Different IP address family",
			existing: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.1", "198.51.100.1"}; return &s }(),
				},
			},
			new: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv6"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"2001:db8::1", "2001:db8::2"}; return &s }(),
				},
			},
			want: false,
		},
		{
			name: "Different number of IP sets",
			existing: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.1", "198.51.100.1"}; return &s }(),
				},
			},
			new: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.1", "198.51.100.1"}; return &s }(),
				},
				{
					IpAddressFamily: func() *string { s := "IPv6"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"2001:db8::1", "2001:db8::2"}; return &s }(),
				},
			},
			want: false,
		},
		{
			name:     "Both empty",
			existing: []v1beta1.IPSet{},
			new:      []v1beta1.IPSet{},
			want:     true,
		},
		{
			name:     "Existing empty",
			existing: []v1beta1.IPSet{},
			new: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.1", "198.51.100.1"}; return &s }(),
				},
			},
			want: false,
		},
		{
			name: "New empty",
			existing: []v1beta1.IPSet{
				{
					IpAddressFamily: func() *string { s := "IPv4"; return &s }(),
					IpAddresses:     func() *[]string { s := []string{"192.0.2.1", "198.51.100.1"}; return &s }(),
				},
			},
			new:  []v1beta1.IPSet{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create status updater
			updater := &defaultStatusUpdater{
				k8sClient: testutils.GenerateTestClient(),
				logger:    logr.New(&log.NullLogSink{}),
			}

			// Call the method being tested
			got := updater.areIPSetsEqual(tt.existing, tt.new)

			// Check result
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultStatusUpdater_isAcceleratorDeployed(t *testing.T) {
	tests := []struct {
		name   string
		status agamodel.AcceleratorStatus
		want   bool
	}{
		{
			name: "Status deployed",
			status: agamodel.AcceleratorStatus{
				Status: StatusDeployed,
			},
			want: true,
		},
		{
			name: "Status in progress",
			status: agamodel.AcceleratorStatus{
				Status: StatusInProgress,
			},
			want: false,
		},
		{
			name: "Status empty",
			status: agamodel.AcceleratorStatus{
				Status: "",
			},
			want: false,
		},
		{
			name: "Status other",
			status: agamodel.AcceleratorStatus{
				Status: "OTHER",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create status updater
			updater := &defaultStatusUpdater{
				k8sClient: testutils.GenerateTestClient(),
				logger:    logr.New(&log.NullLogSink{}),
			}

			// Call the method being tested
			got := updater.isAcceleratorDeployed(tt.status)

			// Check result
			assert.Equal(t, tt.want, got)
		})
	}
}
