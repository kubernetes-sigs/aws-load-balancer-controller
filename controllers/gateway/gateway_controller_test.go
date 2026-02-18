package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	gateway_constants "sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/routeutils"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func Test_handleReconcileError(t *testing.T) {
	testCases := []struct {
		name                   string
		gw                     *gwv1.Gateway
		err                    error
		expectStatusUpdate     bool
		expectedAcceptedStatus metav1.ConditionStatus
		expectedMessage        string
	}{
		{
			name: "RequeueNeeded error should not update status",
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: "test-ns",
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.GatewayConditionAccepted),
						},
					},
				},
			},
			err:                ctrlerrors.NewRequeueNeeded("waiting for dependency"),
			expectStatusUpdate: false,
		},
		{
			name: "RequeueNeededAfter error should not update status",
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: "test-ns",
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.GatewayConditionAccepted),
						},
					},
				},
			},
			err:                ctrlerrors.NewRequeueNeededAfter("waiting for LB provisioning", 2*time.Minute),
			expectStatusUpdate: false,
		},
		{
			name: "regular error should update status with static message",
			gw: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: "test-ns",
				},
				Status: gwv1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.GatewayConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gwv1.GatewayConditionAccepted),
						},
					},
				},
			},
			err:                    errors.New("failed to create load balancer"),
			expectStatusUpdate:     true,
			expectedAcceptedStatus: metav1.ConditionFalse,
			expectedMessage:        gateway_constants.GatewayReconcileErrorMessage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testutils.GenerateTestClient()
			err := k8sClient.Create(context.Background(), tc.gw)
			assert.NoError(t, err)

			reconciler := &gatewayReconciler{
				k8sClient:               k8sClient,
				logger:                  logr.Discard(),
				eventRecorder:           record.NewFakeRecorder(10),
				gatewayConditionUpdater: prepareGatewayConditionUpdate,
			}

			// Get a fresh copy of the gateway for the reconciler to work with
			gwCopy := tc.gw.DeepCopy()
			reconciler.handleReconcileError(context.Background(), gwCopy, tc.err)

			// Fetch the gateway from the client to check if status was updated
			storedGw := &gwv1.Gateway{}
			err = k8sClient.Get(context.Background(), k8s.NamespacedName(tc.gw), storedGw)
			assert.NoError(t, err)

			if tc.expectStatusUpdate {
				// Find the Accepted condition
				var acceptedCondition *metav1.Condition
				for i := range storedGw.Status.Conditions {
					if storedGw.Status.Conditions[i].Type == string(gwv1.GatewayConditionAccepted) {
						acceptedCondition = &storedGw.Status.Conditions[i]
						break
					}
				}
				assert.NotNil(t, acceptedCondition, "Accepted condition should exist")
				assert.Equal(t, tc.expectedAcceptedStatus, acceptedCondition.Status)
				assert.Equal(t, tc.expectedMessage, acceptedCondition.Message)
				assert.Equal(t, string(gwv1.GatewayReasonInvalid), acceptedCondition.Reason)
			} else {
				// Status should remain unchanged (still True)
				var acceptedCondition *metav1.Condition
				for i := range storedGw.Status.Conditions {
					if storedGw.Status.Conditions[i].Type == string(gwv1.GatewayConditionAccepted) {
						acceptedCondition = &storedGw.Status.Conditions[i]
						break
					}
				}
				assert.NotNil(t, acceptedCondition, "Accepted condition should exist")
				assert.Equal(t, metav1.ConditionTrue, acceptedCondition.Status)
			}
		})
	}
}

func Test_updateGatewayStatusSuccess_normalizesDNSNameToLowercase(t *testing.T) {
	k8sClient := testutils.GenerateTestClient()
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
	}
	err := k8sClient.Create(context.Background(), gw)
	assert.NoError(t, err)

	reconciler := &gatewayReconciler{
		k8sClient:               k8sClient,
		logger:                  logr.Discard(),
		eventRecorder:           record.NewFakeRecorder(10),
		gatewayConditionUpdater: prepareGatewayConditionUpdate,
	}

	lbStatus := &elbv2model.LoadBalancerStatus{
		LoadBalancerARN: "arn:aws:elasticloadbalancing:region:account-id:loadbalancer/app/my-alb/123456789",
		DNSName:         "MyCamelBalancer-1234567890.EU-WEST-1.ELB.AMAZONAWS.COM",
		ProvisioningState: &elbv2types.LoadBalancerState{
			Code: elbv2types.LoadBalancerStateEnumActive,
		},
	}

	err = reconciler.updateGatewayStatusSuccess(context.Background(), lbStatus, gw, routeutils.LoaderResult{})
	assert.NoError(t, err)

	updatedGW := &gwv1.Gateway{}
	err = k8sClient.Get(context.Background(), k8s.NamespacedName(gw), updatedGW)
	assert.NoError(t, err)
	assert.Len(t, updatedGW.Status.Addresses, 1)
	assert.Equal(t, "mycamelbalancer-1234567890.eu-west-1.elb.amazonaws.com", updatedGW.Status.Addresses[0].Value)
	assert.NotNil(t, updatedGW.Status.Addresses[0].Type)
	assert.Equal(t, gwv1.HostnameAddressType, *updatedGW.Status.Addresses[0].Type)
}
