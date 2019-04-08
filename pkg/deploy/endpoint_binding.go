package deploy

import (
	"context"
	"k8s.io/apimachinery/pkg/util/sets"
	"reflect"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewEndpointBindingActuator(ebRepo backend.EndpointBindingRepo, stack *build.LoadBalancingStack) Actuator {
	return &endpointBindingActuator{
		ebRepo: ebRepo,
		stack:  stack,
	}
}

type endpointBindingActuator struct {
	ebRepo backend.EndpointBindingRepo
	stack  *build.LoadBalancingStack
}

func (a *endpointBindingActuator) Initialize(ctx context.Context) error {
	existingEBList, err := a.ebRepo.List(context.Background(), client.MatchingField(backend.EndpointBindingRepoIndexStack, a.stack.ID))
	if err != nil {
		return err
	}
	existingEBMap := make(map[string]*api.EndpointBinding)
	for index := range existingEBList.Items {
		existingEBMap[existingEBList.Items[index].Name] = &existingEBList.Items[index]
	}

	inUseEBNames := sets.String{}
	for _, eb := range a.stack.EndpointBindings {
		inUseEBNames.Insert(eb.Name)
		tgArn, err := resolveTargetGroupReference(ctx, a.stack, eb.Spec.TargetGroup)
		if err != nil {
			return err
		}
		eb.Spec.TargetGroup.TargetGroupARN = tgArn
		eb.Labels = map[string]string{
			backend.EndpointBindingLabelKeyStack: a.stack.ID,
		}
		existingEB, ok := existingEBMap[eb.Name]
		if !ok {
			if err := a.ebRepo.Create(ctx, eb); err != nil {
				return err
			}
		} else if !reflect.DeepEqual(eb.Spec, existingEB.Spec) {
			if err := a.ebRepo.Update(ctx, eb); err != nil {
				return err
			}
		}
	}

	for name := range sets.StringKeySet(existingEBMap).Difference(inUseEBNames) {
		logging.FromContext(ctx).Info("deleting endpoint-binding", "name", name)
		if err := a.ebRepo.Delete(ctx, existingEBMap[name]); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("deleted endpoint-binding", "name", name)
	}
	return nil
}

func (a *endpointBindingActuator) Finalize(ctx context.Context) error {
	return nil
}
