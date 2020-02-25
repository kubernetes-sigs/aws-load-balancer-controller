package tg

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"
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

func (tgWatches targetGroupWatches) String() string {
	ret := "Target group watch for:\n"
	for arn, tgWatch := range tgWatches {
		ret += fmt.Sprintf("* %s: ingress %s/%s, backend service %s:%s; targets to reconcile: %s\n", arn, tgWatch.ingress.Namespace, tgWatch.ingress.Name, tgWatch.backend.ServiceName, tgWatch.backend.ServicePort.String(), tdsString(tgWatch.targetsToReconcile))
	}
	return ret
}

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

	desiredTargets := []*elbv2.TargetDescription{
		{
			Id: aws.String("10.0.0.1"),
		},
	}

	pods := podsWithReadinessGateAndStatus(podConditionTypeForIngressBackend(ingress, backend), noConditionType, api.ConditionUnknown)

	for _, tc := range []struct {
		Name                       string
		Targets                    *Targets
		DesiredTargets             []*elbv2.TargetDescription
		ExistingTagetsToReconcile  []*elbv2.TargetDescription
		Pods                       []*api.Pod
		ExpectedTargetsToReconcile []*elbv2.TargetDescription
		ExpectedError              error
		CancelCalled               bool
	}{
		{
			Name:                       "Unready target with matching pod readiness gate and without condition status gets synced for reconciliation; creates targetGroupWatch and starts go routine",
			Targets:                    &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets:             desiredTargets,
			Pods:                       pods,
			ExpectedTargetsToReconcile: desiredTargets,
		},
		{
			Name:                       "Existing targetsToReconcile will be removed when they are not desired anymore",
			Targets:                    &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets:             desiredTargets,
			Pods:                       pods,
			ExistingTagetsToReconcile:  []*elbv2.TargetDescription{{Id: aws.String("10.0.0.2")}},
			ExpectedTargetsToReconcile: desiredTargets,
		},
		{
			Name:                       "When targetsToReconcile becomes empty, go routine will be canceled",
			Targets:                    &Targets{TgArn: tgArn, Ingress: ingress, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets:             []*elbv2.TargetDescription{},
			Pods:                       []*api.Pod{},
			ExistingTagetsToReconcile:  []*elbv2.TargetDescription{{Id: aws.String("10.0.0.2")}},
			ExpectedTargetsToReconcile: []*elbv2.TargetDescription{},
			CancelCalled:               true,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			endpointResolver := &mocks.EndpointResolver{}
			if len(tc.DesiredTargets) > 0 {
				endpointResolver.On("ReverseResolve", ingress, backend, tc.DesiredTargets).Return(tc.Pods, nil)
			}

			cloud := &mocks.CloudAPI{}

			client := testclient.NewFakeClient()

			controller := NewTargetHealthController(cloud, endpointResolver, client).(*targetHealthController)

			cancelCalled := false
			if len(tc.ExistingTagetsToReconcile) > 0 {
				controller.tgWatches["arn:"] = &targetGroupWatch{
					targetsToReconcile: tc.ExistingTagetsToReconcile,
					cancel: func() {
						cancelCalled = true
					},
				}
			}

			err := controller.SyncTargetsForReconciliation(ctx, tc.Targets, tc.DesiredTargets)

			if len(tc.ExpectedTargetsToReconcile) > 0 {
				assert.Equal(t, 1, len(controller.tgWatches))
				tgWatch, ok := controller.tgWatches["arn:"]
				assert.True(t, ok)
				assert.Equal(t, tc.ExpectedTargetsToReconcile, tgWatch.targetsToReconcile)
			} else {
				assert.Equal(t, len(controller.tgWatches), 0)
			}

			assert.Equal(t, tc.CancelCalled, cancelCalled)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
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
		api.PodConditionType(fmt.Sprintf("target-health.%s/ingress2_name_123", parser.AnnotationsPrefix)),
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

			cloud := &mocks.CloudAPI{}

			client := testclient.NewFakeClient()
			for _, actualPod := range tc.PodsBefore {
				assert.NoError(t, client.Create(ctx, actualPod))
			}

			controller := NewTargetHealthController(cloud, endpointResolver, client).(*targetHealthController)

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
				assert.Equal(t, expectedPod, &actualPod)
			}
		})
	}
}

func Test_reconcilePodCondition(t *testing.T) {
	conditionType := api.PodConditionType(fmt.Sprintf("target-health.%s/ingress1_name_123", parser.AnnotationsPrefix))

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
			client := testclient.NewFakeClient()
			for _, actualPod := range tc.PodsBefore {
				assert.NoError(t, client.Create(ctx, actualPod))
			}

			controller := NewTargetHealthController(cloud, endpointResolver, client).(*targetHealthController)
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
	ingress1 := &extensions.Ingress{
		ObjectMeta: v1.ObjectMeta{Name: "ingress1"},
		Spec:       extensions.IngressSpec{Backend: backend},
	}
	ingress2 := &extensions.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name: "ingress2",
			Annotations: map[string]string{
				fmt.Sprintf("%s/target-health-reconciliation-strategy", parser.AnnotationsPrefix): targetHealthReconciliationStrategyContinuous,
			},
		},
		Spec: extensions.IngressSpec{Backend: backend},
	}
	desiredTargets := []*elbv2.TargetDescription{{Id: aws.String("10.0.0.1")}}

	conditionType := podConditionTypeForIngressBackend(ingress1, backend)

	for _, tc := range []struct {
		Name                    string
		Targets                 *Targets
		DesiredTargets          []*elbv2.TargetDescription
		ExpectedFilteredTargets []*elbv2.TargetDescription
		Pods                    []*api.Pod
		ExpectedError           error
	}{
		{
			Name:                    "Unready target without pod readiness gate gets ignored",
			Targets:                 &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets:          desiredTargets,
			Pods:                    podsWithReadinessGateAndStatus(noConditionType, noConditionType, api.ConditionUnknown),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
		{
			Name:                    "Unready target without matching pod readiness gate gets ignored",
			Targets:                 &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets:          desiredTargets,
			Pods:                    podsWithReadinessGateAndStatus(podConditionTypeForIngressBackend(ingress2, backend), noConditionType, api.ConditionUnknown),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
		{
			Name:                    "Unready target with matching pod readiness gate and without condition status gets picked",
			Targets:                 &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets:          desiredTargets,
			Pods:                    podsWithReadinessGateAndStatus(conditionType, noConditionType, api.ConditionUnknown),
			ExpectedFilteredTargets: desiredTargets,
		},
		{
			Name:           "Unready target with matching pod readiness gate and with condition status = false gets picked",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
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
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets: desiredTargets,
			Pods: podsWithReadinessGateAndStatus(
				conditionType,
				conditionType,
				api.ConditionTrue,
			),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
		{
			Name:           "Ready target with matching pod readiness and condition status = true gate gets picked with strategy = continuous",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress2, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets: desiredTargets,
			Pods: podsWithReadinessGateAndStatus(
				conditionType,
				conditionType,
				api.ConditionFalse,
			),
			ExpectedFilteredTargets: desiredTargets,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			endpointResolver := &mocks.EndpointResolver{}
			endpointResolver.On("ReverseResolve", mock.Anything, backend, tc.DesiredTargets).Return(tc.Pods, nil)

			cloud := &mocks.CloudAPI{}

			client := testclient.NewFakeClient()

			controller := NewTargetHealthController(cloud, endpointResolver, client).(*targetHealthController)
			filteredTargets, err := controller.filterTargetsNeedingReconciliation(conditionType, tc.Targets, tc.DesiredTargets)

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
