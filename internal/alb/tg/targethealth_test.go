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

func podsWithReadinessGateAndStatus(readinessGate, status string, statusValue api.ConditionStatus) []*api.Pod {
	pod := &api.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "pod1",
		},
		Status: api.PodStatus{
			PodIP: "10.0.0.1",
		},
	}
	if readinessGate != "" {
		pod.Spec = api.PodSpec{
			ReadinessGates: []api.PodReadinessGate{
				{
					ConditionType: api.PodConditionType(readinessGate),
				},
			},
		}
	}
	if status != "" {
		pod.Status.Conditions = []api.PodCondition{
			{
				Type:   api.PodConditionType(status),
				Status: statusValue,
			},
		}
	}
	return []*api.Pod{pod}
}

func Test_SyncTargetsForReconcilation(t *testing.T) {
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

	pods := podsWithReadinessGateAndStatus(fmt.Sprintf("target-health.%s/ingress1_name_123", parser.AnnotationsPrefix), "", api.ConditionUnknown)

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
			Name:                       "Unready target with matching pod readiness gate and without condition status gets synced for reconcilation; creates targetGroupWatch and starts go routine",
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

			err := controller.SyncTargetsForReconcilation(ctx, tc.Targets, tc.DesiredTargets)

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

	condition := fmt.Sprintf("target-health.%s/ingress1_name_123", parser.AnnotationsPrefix)

	podsWithoutCondition := podsWithReadinessGateAndStatus(condition, "", api.ConditionUnknown)
	podsWithCondition := podsWithReadinessGateAndStatus(
		condition,
		condition,
		api.ConditionTrue,
	)
	podsWithForeignCondition := podsWithReadinessGateAndStatus(
		condition,
		fmt.Sprintf("target-health.%s/ingress2_name_123", parser.AnnotationsPrefix),
		api.ConditionTrue,
	)
	podsWithMultipleConditions := podsWithReadinessGateAndStatus(
		condition,
		fmt.Sprintf("target-health.%s/ingress2_name_123", parser.AnnotationsPrefix),
		api.ConditionTrue,
	)
	podsWithMultipleConditions[0].Status.Conditions = append(podsWithMultipleConditions[0].Status.Conditions, api.PodCondition{
		Type: api.PodConditionType(condition),
	})

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
	condition := fmt.Sprintf("target-health.%s/ingress1_name_123", parser.AnnotationsPrefix)
	conditionType := api.PodConditionType(condition)

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
			PodsBefore:   podsWithReadinessGateAndStatus(condition, "", api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(condition, condition, api.ConditionFalse),
		},
		{
			Name:         "Pod without condition and targetHealth = unhealthy gets condition status = false",
			TargetHealth: elbv2.TargetHealthStateEnumUnhealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(condition, "", api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(condition, condition, api.ConditionFalse),
		},
		{
			Name:         "Pod without condition and targetHealth = draining gets condition status = false",
			TargetHealth: elbv2.TargetHealthStateEnumDraining,
			PodsBefore:   podsWithReadinessGateAndStatus(condition, "", api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(condition, condition, api.ConditionFalse),
		},
		{
			Name:         "Pod without condition and targetHealth = unavailable gets condition status = unknown",
			TargetHealth: elbv2.TargetHealthStateEnumUnavailable,
			PodsBefore:   podsWithReadinessGateAndStatus(condition, "", api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(condition, condition, api.ConditionUnknown),
		},
		{
			Name:         "Pod without condition and targetHealth = healthy gets condition status = true",
			TargetHealth: elbv2.TargetHealthStateEnumHealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(condition, "", api.ConditionUnknown),
			PodsAfter:    podsWithReadinessGateAndStatus(condition, condition, api.ConditionTrue),
		},
		{
			Name:         "Pod condition gets updated correctly false -> true",
			TargetHealth: elbv2.TargetHealthStateEnumHealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(condition, condition, api.ConditionFalse),
			PodsAfter:    podsWithReadinessGateAndStatus(condition, condition, api.ConditionTrue),
		},
		{
			Name:         "Pod condition gets updated correctly true -> false",
			TargetHealth: elbv2.TargetHealthStateEnumUnhealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(condition, condition, api.ConditionTrue),
			PodsAfter:    podsWithReadinessGateAndStatus(condition, condition, api.ConditionFalse),
		},
		{
			Name:         "Pod condition doesn't get updated if target health didn't change",
			TargetHealth: elbv2.TargetHealthStateEnumUnhealthy,
			PodsBefore:   podsWithReadinessGateAndStatus(condition, condition, api.ConditionFalse),
			PodsAfter:    podsWithReadinessGateAndStatus(condition, condition, api.ConditionFalse),
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

func Test_filterTargetsNeedingReconcilation(t *testing.T) {
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
				fmt.Sprintf("%s/target-health-reconcilation-strategy", parser.AnnotationsPrefix): "continuous",
			},
		},
		Spec: extensions.IngressSpec{Backend: backend},
	}
	desiredTargets := []*elbv2.TargetDescription{{Id: aws.String("10.0.0.1")}}

	condition := fmt.Sprintf("target-health.%s/ingress1_name_123", parser.AnnotationsPrefix)
	conditionType := api.PodConditionType(condition)

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
			Pods:                    podsWithReadinessGateAndStatus("", "", api.ConditionUnknown),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
		{
			Name:                    "Unready target without matching pod readiness gate gets ignored",
			Targets:                 &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets:          desiredTargets,
			Pods:                    podsWithReadinessGateAndStatus(fmt.Sprintf("target-health.%s/ingress2_name_123", parser.AnnotationsPrefix), "", api.ConditionUnknown),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
		{
			Name:                    "Unready target with matching pod readiness gate and without condition status gets picked",
			Targets:                 &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets:          desiredTargets,
			Pods:                    podsWithReadinessGateAndStatus(condition, "", api.ConditionUnknown),
			ExpectedFilteredTargets: desiredTargets,
		},
		{
			Name:           "Unready target with matching pod readiness gate and with condition status = false gets picked",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets: desiredTargets,
			Pods: podsWithReadinessGateAndStatus(
				condition,
				condition,
				api.ConditionFalse,
			),
			ExpectedFilteredTargets: desiredTargets,
		},
		{
			Name:           "Ready target with matching pod readiness and condition status = true gate gets ignored",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress1, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets: desiredTargets,
			Pods: podsWithReadinessGateAndStatus(
				condition,
				condition,
				api.ConditionTrue,
			),
			ExpectedFilteredTargets: []*elbv2.TargetDescription{},
		},
		{
			Name:           "Ready target with matching pod readiness and condition status = true gate gets picked with strategy = continuous",
			Targets:        &Targets{TgArn: tgArn, Ingress: ingress2, Backend: backend, TargetType: elbv2.TargetTypeEnumInstance},
			DesiredTargets: desiredTargets,
			Pods: podsWithReadinessGateAndStatus(
				condition,
				condition,
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
			filteredTargets, err := controller.filterTargetsNeedingReconcilation(conditionType, tc.Targets, tc.DesiredTargets)

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
