package tg

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/targetgroup"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	realclient "sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func deepCopyPods(pods []*api.Pod) (ret []*api.Pod) {
	for _, pod := range pods {
		ret = append(ret, pod.DeepCopy())
	}
	return
}

const noConditionType = api.PodConditionType("")

func podsWithReadinessGateAndStatus(readinessGate, status api.PodConditionType, statusValue api.ConditionStatus) []*api.Pod {
	pod := &api.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "pod1",
		},
		Status: api.PodStatus{
			PodIP: "10.0.0.1",
		},
	}
	if readinessGate != noConditionType {
		pod.Spec = api.PodSpec{
			ReadinessGates: []api.PodReadinessGate{
				{
					ConditionType: readinessGate,
				},
			},
		}
	}
	if status != noConditionType {
		pod.Status.Conditions = []api.PodCondition{
			{
				Type:   status,
				Status: statusValue,
			},
		}
	}
	return []*api.Pod{pod}
}

func Test_SyncTargetsForReconciliation(t *testing.T) {
	tgArn := "arn:"
	serviceName := "name"
	servicePort := intstr.FromInt(123)
	backend := &extensions.IngressBackend{ServiceName: serviceName, ServicePort: servicePort}
	ingress := dummy.NewIngress()

	targets := &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance}
	desiredTargets := []*elbv2.TargetDescription{
		{
			Id: aws.String("10.0.0.1"),
		},
	}

	pods := podsWithReadinessGateAndStatus(podConditionTypeForIngressBackend(ingress, backend), noConditionType, api.ConditionUnknown)

	for _, tc := range []struct {
		Name            string
		Targets         *Targets
		DesiredTargets  []*elbv2.TargetDescription
		Pods            []*api.Pod
		ExistingTgWatch bool
		Cancelled       bool
	}{
		{
			Name:           "Unready target with matching pod readiness gate and without condition status gets synced for reconciliation; creates targetGroupWatch and starts go routine",
			DesiredTargets: desiredTargets,
			Pods:           pods,
		},
		{
			Name:            "When targetsToReconcile becomes empty, go routine will be cancelled",
			Pods:            []*api.Pod{},
			ExistingTgWatch: true,
			Cancelled:       true,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			endpointResolver := &mocks.EndpointResolver{}
			if len(tc.DesiredTargets) > 0 {
				endpointResolver.On("ReverseResolve", ingress, backend, tc.DesiredTargets).Return(tc.Pods, nil)
			}

			cloud := &mocks.CloudAPI{}
			store := &store.MockStorer{}
			healthyTresholdCount := int64(targetgroup.DefaultHealthyThresholdCount)
			healthCheckIntervalSeconds := int64(healthcheck.DefaultIntervalSeconds)
			store.On("GetIngressAnnotations", mock.Anything).Return(nil, nil)
			store.On("GetServiceAnnotations", mock.Anything, mock.Anything).Return(&annotations.Service{
				TargetGroup: &targetgroup.Config{
					HealthyThresholdCount: &healthyTresholdCount,
				},
				HealthCheck: &healthcheck.Config{
					IntervalSeconds: &healthCheckIntervalSeconds,
				},
			}, nil)
			client := testclient.NewFakeClient()

			controller := NewTargetHealthController(cloud, store, endpointResolver, client).(*targetHealthController)

			if tc.ExistingTgWatch {
				tgWatch, newCtx := newTargetGroupWatch(ctx, ingress, backend)
				ctx = newCtx
				controller.tgWatches[tgArn] = tgWatch
				go func(tgWatch *targetGroupWatch) {
					for {
						select {
						case <-tgWatch.interval:
						case <-tgWatch.targetsToReconcile:
						}
					}
				}(tgWatch)
			}

			err := controller.SyncTargetsForReconciliation(ctx, targets, tc.DesiredTargets)
			assert.NoError(t, err)

			cancelled := false
			select {
			case <-ctx.Done():
				cancelled = true
			default:
			}
			assert.Equal(t, tc.Cancelled, cancelled)

			cloud.AssertExpectations(t)
			endpointResolver.AssertExpectations(t)
		})
	}
}

func Test_RemovePodConditions(t *testing.T) {
	tgArn := "arn:"
	serviceName := "name"
	servicePort := intstr.FromInt(123)
	backend := &extensions.IngressBackend{ServiceName: serviceName, ServicePort: servicePort}
	ingress := dummy.NewIngress()

	desiredTargets := []*elbv2.TargetDescription{
		{
			Id: aws.String("10.0.0.1"),
		},
	}

	conditionType := podConditionTypeForIngressBackend(ingress, backend)

	podsWithoutCondition := podsWithReadinessGateAndStatus(conditionType, noConditionType, api.ConditionUnknown)
	podsWithCondition := podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionTrue)
	podsWithForeignCondition := podsWithReadinessGateAndStatus(
		conditionType,
		api.PodConditionType("target-health.alb.ingress.k8s.aws/ingress2_name_123"),
		api.ConditionTrue,
	)
	podsWithMultipleConditions := podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionTrue)
	podsWithMultipleConditions[0].Status.Conditions = append(podsWithMultipleConditions[0].Status.Conditions, podsWithForeignCondition[0].Status.Conditions[0])

	for _, tc := range []struct {
		Name           string
		Targets        *Targets
		RemovedTargets []*elbv2.TargetDescription
		PodsBefore     []*api.Pod
		PodsAfter      []*api.Pod
		ExpectedError  error
	}{
		{
			Name:           "Pod without condition is untouched",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			RemovedTargets: desiredTargets,
			PodsBefore:     podsWithoutCondition,
			PodsAfter:      podsWithoutCondition,
		},
		{
			Name:           "Pod with condition has the condition removed",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			RemovedTargets: desiredTargets,
			PodsBefore:     podsWithCondition,
			PodsAfter:      podsWithoutCondition,
		},
		{
			Name:           "Pod with our readiness gate but only foreign condition is untouched",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			RemovedTargets: desiredTargets,
			PodsBefore:     podsWithForeignCondition,
			PodsAfter:      podsWithForeignCondition,
		},
		{
			Name:           "Pod with our condition and a forgein condition has only our condition removed",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			RemovedTargets: desiredTargets,
			PodsBefore:     podsWithMultipleConditions,
			PodsAfter:      podsWithForeignCondition,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			endpointResolver := &mocks.EndpointResolver{}
			endpointResolver.On("ReverseResolve", ingress, backend, tc.RemovedTargets).Return(deepCopyPods(tc.PodsBefore), nil)

			store := &store.MockStorer{}
			cloud := &mocks.CloudAPI{}

			client := testclient.NewFakeClient()
			for _, actualPod := range tc.PodsBefore {
				assert.NoError(t, client.Create(ctx, actualPod))
			}

			controller := NewTargetHealthController(cloud, store, endpointResolver, client).(*targetHealthController)

			err := controller.RemovePodConditions(ctx, tc.Targets, tc.RemovedTargets)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			cloud.AssertExpectations(t)
			endpointResolver.AssertExpectations(t)

			for _, expectedPod := range tc.PodsAfter {
				var actualPod api.Pod
				key, err := realclient.ObjectKeyFromObject(expectedPod)
				assert.NoError(t, err)
				assert.NoError(t, client.Get(ctx, key, &actualPod))
				assert.Equal(t, len(expectedPod.Status.Conditions), len(actualPod.Status.Conditions))
			}
		})
	}
}

func Test_reconcilePodConditionsLoop(t *testing.T) {
	ctx := context.Background()
	endpointResolver := &mocks.EndpointResolver{}
	cloud := &mocks.CloudAPI{}
	store := &store.MockStorer{}
	store.On("GetIngressAnnotations", mock.Anything).Return(nil, fmt.Errorf("some error that should not propagate"))
	client := testclient.NewFakeClient()

	controller := NewTargetHealthController(cloud, store, endpointResolver, client).(*targetHealthController)

	tgWatch, ctx := newTargetGroupWatch(ctx, &extensions.Ingress{}, &extensions.IngressBackend{})
	go func() {
		tgWatch.cancel()
	}()

	controller.reconcilePodConditionsLoop(ctx, "arn", "something", tgWatch)
	// verifies that the loop breaks when the context is canceled, otherwise this will hang
}

type describeTargetHealthWithContextCall struct {
	input  *elbv2.DescribeTargetHealthInput
	output *elbv2.DescribeTargetHealthOutput
	err    error
}

func Test_reconcilePodConditions(t *testing.T) {
	conditionType := api.PodConditionType("target-health.alb.ingress.k8s.aws/ingress1_name_123")
	for _, tc := range []struct {
		name                                string
		tgARN                               string
		ingress                             *extensions.Ingress
		backend                             *extensions.IngressBackend
		targetsToReconcile                  []*elbv2.TargetDescription
		describeTargetHealthWithContextCall *describeTargetHealthWithContextCall
		podsBefore                          []*api.Pod
		podsAfter                           []*api.Pod
		expectedError                       error
		expectedNotReadyTargets             []*elbv2.TargetDescription
	}{
		{
			name:    "when all targets healthy",
			tgARN:   "tgArn1",
			ingress: &extensions.Ingress{},
			backend: &extensions.IngressBackend{},
			targetsToReconcile: []*elbv2.TargetDescription{
				{
					Id:   aws.String("ip1"),
					Port: aws.Int64(8080),
				},
				{
					Id:   aws.String("ip2"),
					Port: aws.Int64(8080),
				},
				{
					Id:   aws.String("ip3"),
					Port: aws.Int64(8080),
				},
			},
			describeTargetHealthWithContextCall: &describeTargetHealthWithContextCall{
				input: &elbv2.DescribeTargetHealthInput{
					TargetGroupArn: aws.String("tgArn1"),
					Targets: []*elbv2.TargetDescription{
						{
							Id:   aws.String("ip1"),
							Port: aws.Int64(8080),
						},
						{
							Id:   aws.String("ip2"),
							Port: aws.Int64(8080),
						},
						{
							Id:   aws.String("ip3"),
							Port: aws.Int64(8080),
						},
					},
				},
				output: &elbv2.DescribeTargetHealthOutput{
					TargetHealthDescriptions: []*elbv2.TargetHealthDescription{
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip1"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumHealthy),
							},
						},
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip2"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumHealthy),
							},
						},
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip3"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumHealthy),
							},
						},
					},
				},
			},
			podsBefore: []*api.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod1",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip1",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod2",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip2",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod3",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip3",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionFalse,
								Reason:  "oldReason",
								Message: "oldDescription",
							},
						},
					},
				},
			},
			podsAfter: []*api.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod1",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip1",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionTrue,
								Reason:  "reason",
								Message: "description",
							},
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod2",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip2",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionTrue,
								Reason:  "reason",
								Message: "description",
							},
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod3",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip3",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionTrue,
								Reason:  "reason",
								Message: "description",
							},
						},
					},
				},
			},
			expectedError:           nil,
			expectedNotReadyTargets: nil,
		},
		{
			name:    "when one targets unhealthy",
			tgARN:   "tgArn1",
			ingress: &extensions.Ingress{},
			backend: &extensions.IngressBackend{},
			targetsToReconcile: []*elbv2.TargetDescription{
				{
					Id:   aws.String("ip1"),
					Port: aws.Int64(8080),
				},
				{
					Id:   aws.String("ip2"),
					Port: aws.Int64(8080),
				},
				{
					Id:   aws.String("ip3"),
					Port: aws.Int64(8080),
				},
			},
			describeTargetHealthWithContextCall: &describeTargetHealthWithContextCall{
				input: &elbv2.DescribeTargetHealthInput{
					TargetGroupArn: aws.String("tgArn1"),
					Targets: []*elbv2.TargetDescription{
						{
							Id:   aws.String("ip1"),
							Port: aws.Int64(8080),
						},
						{
							Id:   aws.String("ip2"),
							Port: aws.Int64(8080),
						},
						{
							Id:   aws.String("ip3"),
							Port: aws.Int64(8080),
						},
					},
				},
				output: &elbv2.DescribeTargetHealthOutput{
					TargetHealthDescriptions: []*elbv2.TargetHealthDescription{
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip1"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumHealthy),
							},
						},
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip2"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumUnhealthy),
							},
						},
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip3"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumHealthy),
							},
						},
					},
				},
			},
			podsBefore: []*api.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod1",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip1",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod2",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip2",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod3",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip3",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionFalse,
								Reason:  "oldReason",
								Message: "oldDescription",
							},
						},
					},
				},
			},
			podsAfter: []*api.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod1",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip1",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionTrue,
								Reason:  "reason",
								Message: "description",
							},
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod2",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip2",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionFalse,
								Reason:  "reason",
								Message: "description",
							},
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod3",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip3",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionTrue,
								Reason:  "reason",
								Message: "description",
							},
						},
					},
				},
			},
			expectedError: nil,
			expectedNotReadyTargets: []*elbv2.TargetDescription{
				{
					Id:   aws.String("ip2"),
					Port: aws.Int64(8080),
				},
			},
		},
		{
			name:    "when one targets unable to resolve",
			tgARN:   "tgArn1",
			ingress: &extensions.Ingress{},
			backend: &extensions.IngressBackend{},
			targetsToReconcile: []*elbv2.TargetDescription{
				{
					Id:   aws.String("ip1"),
					Port: aws.Int64(8080),
				},
				{
					Id:   aws.String("ip2"),
					Port: aws.Int64(8080),
				},
				{
					Id:   aws.String("ip3"),
					Port: aws.Int64(8080),
				},
			},
			describeTargetHealthWithContextCall: &describeTargetHealthWithContextCall{
				input: &elbv2.DescribeTargetHealthInput{
					TargetGroupArn: aws.String("tgArn1"),
					Targets: []*elbv2.TargetDescription{
						{
							Id:   aws.String("ip1"),
							Port: aws.Int64(8080),
						},
						{
							Id:   aws.String("ip2"),
							Port: aws.Int64(8080),
						},
						{
							Id:   aws.String("ip3"),
							Port: aws.Int64(8080),
						},
					},
				},
				output: &elbv2.DescribeTargetHealthOutput{
					TargetHealthDescriptions: []*elbv2.TargetHealthDescription{
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip1"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumHealthy),
							},
						},
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip2"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumHealthy),
							},
						},
						{
							Target: &elbv2.TargetDescription{
								Id:   aws.String("ip3"),
								Port: aws.Int64(8080),
							},
							TargetHealth: &elbv2.TargetHealth{
								Reason:      aws.String("reason"),
								Description: aws.String("description"),
								State:       aws.String(elbv2.TargetHealthStateEnumHealthy),
							},
						},
					},
				},
			},
			podsBefore: []*api.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod1",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip1",
					},
				},
				nil,
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod3",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip3",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionFalse,
								Reason:  "oldReason",
								Message: "oldDescription",
							},
						},
					},
				},
			},
			podsAfter: []*api.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod1",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip1",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionTrue,
								Reason:  "reason",
								Message: "description",
							},
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
						Name:      "pod3",
					},
					Spec: api.PodSpec{
						ReadinessGates: []api.PodReadinessGate{
							{
								ConditionType: conditionType,
							},
						},
					},
					Status: api.PodStatus{
						PodIP: "ip3",
						Conditions: []api.PodCondition{
							{
								Type:    conditionType,
								Status:  api.ConditionTrue,
								Reason:  "reason",
								Message: "description",
							},
						},
					},
				},
			},
			expectedError:           nil,
			expectedNotReadyTargets: nil,
		},
	} {
		endpointResolver := &mocks.EndpointResolver{}
		store := &store.MockStorer{}
		cloud := &mocks.CloudAPI{}
		client := testclient.NewFakeClient()

		if tc.describeTargetHealthWithContextCall != nil {
			cloud.On("DescribeTargetHealthWithContext", mock.Anything, tc.describeTargetHealthWithContextCall.input).Return(tc.describeTargetHealthWithContextCall.output, tc.describeTargetHealthWithContextCall.err)
		}
		endpointResolver.On("ReverseResolve", tc.ingress, tc.backend, tc.targetsToReconcile).Maybe().Return(tc.podsBefore, nil)
		for _, actualPod := range tc.podsBefore {
			if actualPod != nil {
				assert.NoError(t, client.Create(context.Background(), actualPod))
			}
		}

		controller := NewTargetHealthController(cloud, store, endpointResolver, client).(*targetHealthController)
		notReadyTargets, err := controller.reconcilePodConditions(context.Background(), tc.tgARN, conditionType, tc.ingress, tc.backend, tc.targetsToReconcile)
		if tc.expectedError != nil {
			assert.EqualError(t, err, tc.expectedError.Error())
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedNotReadyTargets, notReadyTargets)
		}
		cloud.AssertExpectations(t)
		endpointResolver.AssertExpectations(t)

		for _, expectedPod := range tc.podsAfter {
			var actualPod api.Pod
			key, err := realclient.ObjectKeyFromObject(expectedPod)
			assert.NoError(t, err)
			assert.NoError(t, client.Get(context.Background(), key, &actualPod))

			for i := range actualPod.Status.Conditions {
				actualPod.Status.Conditions[i].LastProbeTime = v1.Time{}
				actualPod.Status.Conditions[i].LastTransitionTime = v1.Time{}
			}
			assert.Equal(t, *expectedPod, actualPod)
		}
	}
}

func Test_reconcilePodCondition(t *testing.T) {
	conditionType := api.PodConditionType("target-health.alb.ingress.k8s.aws/ingress1_name_123")

	for _, tc := range []struct {
		Name          string
		TargetHealth  string
		PodsBefore    []*api.Pod
		PodsAfter     []*api.Pod
		ExpectedError error
	}{
		{
			Name:         "Pod without condition and targetHealth = initial gets condition status = false",
			TargetHealth: elbv2.TargetHealthStateEnumInitial,
			PodsBefore:   podsWithReadinessGateAndStatus(conditionType, noConditionType, api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionFalse),
		},
		{
			Name:         "Pod without condition and targetHealth = unhealthy gets condition status = false",
			TargetHealth: elbv2.TargetHealthStateEnumUnhealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(conditionType, noConditionType, api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionFalse),
		},
		{
			Name:         "Pod without condition and targetHealth = draining gets condition status = false",
			TargetHealth: elbv2.TargetHealthStateEnumDraining,
			PodsBefore:   podsWithReadinessGateAndStatus(conditionType, noConditionType, api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionFalse),
		},
		{
			Name:         "Pod without condition and targetHealth = unavailable gets condition status = unknown",
			TargetHealth: elbv2.TargetHealthStateEnumUnavailable,
			PodsBefore:   podsWithReadinessGateAndStatus(conditionType, noConditionType, api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionUnknown),
		},
		{
			Name:         "Pod without condition and targetHealth = healthy gets condition status = true",
			TargetHealth: elbv2.TargetHealthStateEnumHealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(conditionType, noConditionType, api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionTrue),
		},
		{
			Name:         "Pod condition gets updated correctly false -> true",
			TargetHealth: elbv2.TargetHealthStateEnumHealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionFalse),
			PodsAfter:    podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionTrue),
		},
		{
			Name:         "Pod condition gets updated correctly true -> false",
			TargetHealth: elbv2.TargetHealthStateEnumUnhealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionTrue),
			PodsAfter:    podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionFalse),
		},
		{
			Name:         "Pod condition doesn't get updated if target health didn't change",
			TargetHealth: elbv2.TargetHealthStateEnumUnhealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionFalse),
			PodsAfter:    podsWithReadinessGateAndStatus(conditionType, conditionType, api.ConditionFalse),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			endpointResolver := &mocks.EndpointResolver{}

			cloud := &mocks.CloudAPI{}
			store := &store.MockStorer{}
			client := testclient.NewFakeClient()
			for _, actualPod := range tc.PodsBefore {
				assert.NoError(t, client.Create(ctx, actualPod))
			}

			controller := NewTargetHealthController(cloud, store, endpointResolver, client).(*targetHealthController)
			err := controller.reconcilePodCondition(ctx, conditionType, tc.PodsBefore[0].DeepCopy(), &elbv2.TargetHealth{State: aws.String(tc.TargetHealth)}, false)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			cloud.AssertExpectations(t)
			endpointResolver.AssertExpectations(t)

			for _, expectedPod := range tc.PodsAfter {
				var actualPod api.Pod
				key, err := realclient.ObjectKeyFromObject(expectedPod)
				assert.NoError(t, err)
				assert.NoError(t, client.Get(ctx, key, &actualPod))
				assert.Equal(t, *expectedPod, actualPod)
			}
		})
	}
}

func Test_filterTargetsNeedingReconciliation(t *testing.T) {
	tgArn := "arn:"
	serviceName := "name"
	servicePort := intstr.FromInt(123)
	backend := &extensions.IngressBackend{ServiceName: serviceName, ServicePort: servicePort}
	ingress := &extensions.Ingress{
		ObjectMeta: v1.ObjectMeta{Name: "ingress1"},
		Spec:       extensions.IngressSpec{Backend: backend},
	}
	ingress2 := &extensions.Ingress{
		ObjectMeta: v1.ObjectMeta{Name: "ingress2"},
		Spec:       extensions.IngressSpec{Backend: backend},
	}
	desiredTargets := []*elbv2.TargetDescription{{Id: aws.String("10.0.0.1")}}
	targets := &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance}
	conditionType := podConditionTypeForIngressBackend(ingress, backend)

	for _, tc := range []struct {
		Name                    string
		DesiredTargets          []*elbv2.TargetDescription
		ExpectedFilteredTargets []*elbv2.TargetDescription
		Pods                    []*api.Pod
		ExpectedError           error
	}{
		{
			Name:                    "Unready target without pod readiness gate gets ignored",
			DesiredTargets:          desiredTargets,
			Pods:                    podsWithReadinessGateAndStatus(noConditionType, noConditionType, api.ConditionUnknown),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
		{
			Name:                    "Unready target without matching pod readiness gate gets ignored",
			DesiredTargets:          desiredTargets,
			Pods:                    podsWithReadinessGateAndStatus(podConditionTypeForIngressBackend(ingress2, backend), noConditionType, api.ConditionUnknown),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
		{
			Name:                    "Unready target with matching pod readiness gate and without condition status gets picked",
			DesiredTargets:          desiredTargets,
			Pods:                    podsWithReadinessGateAndStatus(conditionType, noConditionType, api.ConditionUnknown),
			ExpectedFilteredTargets: desiredTargets,
		},
		{
			Name:           "Unready target with matching pod readiness gate and with condition status = false gets picked",
			DesiredTargets: desiredTargets,
			Pods: podsWithReadinessGateAndStatus(
				conditionType,
				conditionType,
				api.ConditionFalse,
			),
			ExpectedFilteredTargets: desiredTargets,
		},
		{
			Name:           "Ready target with matching pod readiness and condition status = true gate gets ignored",
			DesiredTargets: desiredTargets,
			Pods: podsWithReadinessGateAndStatus(
				conditionType,
				conditionType,
				api.ConditionTrue,
			),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			endpointResolver := &mocks.EndpointResolver{}
			if tc.ExpectedError == nil {
				endpointResolver.On("ReverseResolve", mock.Anything, backend, tc.DesiredTargets).Return(tc.Pods, nil)
			}

			store := &store.MockStorer{}
			cloud := &mocks.CloudAPI{}
			client := testclient.NewFakeClient()

			controller := NewTargetHealthController(cloud, store, endpointResolver, client).(*targetHealthController)
			filteredTargets, err := controller.filterTargetsNeedingReconciliation(conditionType, targets, tc.DesiredTargets)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			cloud.AssertExpectations(t)
			endpointResolver.AssertExpectations(t)

			assert.Equal(t, tc.ExpectedFilteredTargets, filteredTargets)
		})
	}
}
