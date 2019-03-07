package tg

import (
	"context"
	"fmt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

// GroupController manages all target groups for one service.
type GroupController interface {
	// Reconcile ensures AWS an targetGroup exists for each backend in service.
	Reconcile(ctx context.Context, service *corev1.Service) (TargetGroupGroup, error)

	// GC will delete unused targetGroups matched by tag selector
	GC(ctx context.Context, tgGroup TargetGroupGroup) error

	// Delete will delete all targetGroups created for service
	Delete(ctx context.Context, serviceKey types.NamespacedName) error
}

// NewGroupController creates an GroupController
func NewGroupController(
	cloud aws.CloudAPI,
	store store.Storer,
	nameTagGen NameTagGenerator,
	tagsController tags.Controller,
	endpointResolver backend.EndpointResolver) GroupController {
	tgController := NewController(cloud, store, nameTagGen, tagsController, endpointResolver)
	return &defaultGroupController{
		cloud:        cloud,
		nameTagGen:   nameTagGen,
		tgController: tgController,
	}
}

var _ GroupController = (*defaultGroupController)(nil)

type defaultGroupController struct {
	cloud      aws.CloudAPI
	nameTagGen NameTagGenerator

	tgController Controller
}

func (controller *defaultGroupController) Reconcile(ctx context.Context, service *corev1.Service) (TargetGroupGroup, error) {
	tgByBackend := make(map[extensions.IngressBackend]TargetGroup)
	var err error
	for _, backend := range controller.extractServiceBackends(service) {
		if action.Use(backend.ServicePort.String()) {
			continue
		}
		if tgByBackend[backend], err = controller.tgController.Reconcile(ctx, service, backend); err != nil {
			return TargetGroupGroup{}, err
		}
	}
	selector := controller.nameTagGen.TagTGGroup(service.GetNamespace(), service.GetName())
	return TargetGroupGroup{
		TGByBackend: tgByBackend,
		selector:    selector,
	}, nil
}

func (controller *defaultGroupController) GC(ctx context.Context, tgGroup TargetGroupGroup) error {
	tagFilters := make(map[string][]string)
	for k, v := range tgGroup.selector {
		tagFilters[k] = []string{v}
	}

	usedTgArns := sets.NewString()
	for _, tg := range tgGroup.TGByBackend {
		usedTgArns.Insert(tg.Arn)
	}
	arns, err := controller.cloud.GetResourcesByFilters(tagFilters, aws.ResourceTypeEnumELBTargetGroup)
	if err != nil {
		return fmt.Errorf("failed to get targetGroups due to %v", err)
	}
	currentTgArns := sets.NewString(arns...)
	unusedTgArns := currentTgArns.Difference(usedTgArns)
	for arn := range unusedTgArns {
		albctx.GetLogger(ctx).Infof("deleting target group %v", arn)
		if err := controller.cloud.DeleteTargetGroupByArn(ctx, arn); err != nil {
			return fmt.Errorf("failed to delete targetGroup due to %v", err)
		}
	}
	return nil
}

func (controller *defaultGroupController) Delete(ctx context.Context, serviceKey types.NamespacedName) error {
	selector := controller.nameTagGen.TagTGGroup(serviceKey.Namespace, serviceKey.Name)
	tgGroup := TargetGroupGroup{
		selector: selector,
	}
	return controller.GC(ctx, tgGroup)
}

// TODO, should be k8s utils :D
func (controller *defaultGroupController) extractServiceBackends(service *corev1.Service) []extensions.IngressBackend {
	var output []extensions.IngressBackend
	backend := extensions.IngressBackend{
		ServiceName: service.GetName(),
	}
	for _, port := range service.Spec.Ports {
		if port.Name != "" {
			backend.ServicePort = intstr.FromString(port.Name)
		} else {
			backend.ServicePort = intstr.FromInt(int(port.Port))
		}
		output = append(output, backend)
	}
	return output
}
