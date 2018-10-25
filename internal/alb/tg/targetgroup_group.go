package tg

import (
	"context"
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

// GroupController manages all target groups for one ingress.
type GroupController interface {
	// Reconcile ensures AWS an targetGroup exists for each backend in ingress.
	Reconcile(ctx context.Context, ingress *extensions.Ingress) (TargetGroupGroup, error)

	// GC will delete unused targetGroups matched by tag selector
	GC(ctx context.Context, tgGroup TargetGroupGroup) error

	// Delete will delete all targetGroups created for ingress
	Delete(ctx context.Context, ingressKey types.NamespacedName) error
}

// NewGroupController creates an GroupController
func NewGroupController(
	cloud aws.CloudAPI, rgt albrgt.ResourceGroupsTaggingAPIAPI,
	store store.Storer,
	nameTagGen NameTagGenerator,
	tagsController tags.Controller,
	endpointResolver backend.EndpointResolver) GroupController {
	tgController := NewController(cloud, store, nameTagGen, tagsController, endpointResolver)
	return &defaultGroupController{
		rgt:          rgt,
		cloud:        cloud,
		nameTagGen:   nameTagGen,
		tgController: tgController,
	}
}

var _ GroupController = (*defaultGroupController)(nil)

type defaultGroupController struct {
	rgt        albrgt.ResourceGroupsTaggingAPIAPI
	cloud      aws.CloudAPI
	nameTagGen NameTagGenerator

	tgController Controller
}

func (controller *defaultGroupController) Reconcile(ctx context.Context, ingress *extensions.Ingress) (TargetGroupGroup, error) {
	tgByBackend := make(map[extensions.IngressBackend]TargetGroup)
	var err error
	for _, backend := range controller.extractIngressBackends(ingress) {
		if action.Use(backend.ServicePort.String()) {
			continue
		}
		if _, ok := tgByBackend[backend]; ok {
			continue
		}
		if tgByBackend[backend], err = controller.tgController.Reconcile(ctx, ingress, backend); err != nil {
			return TargetGroupGroup{}, err
		}
	}
	selector := controller.nameTagGen.TagTGGroup(ingress.Namespace, ingress.Name)
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
	arns, err := controller.rgt.GetResourcesByFilters(tagFilters, albrgt.ResourceTypeEnumELBTargetGroup)
	if err != nil {
		return fmt.Errorf("failed to get targetGroups due to %v", err)
	}
	currentTgArns := sets.NewString(arns...)
	unusedTgArns := currentTgArns.Difference(usedTgArns)
	for arn := range unusedTgArns {
		if err := controller.cloud.DeleteTargetGroupByArn(arn); err != nil {
			return fmt.Errorf("failed to delete targetGroup due to %v", err)
		}
	}
	return nil
}

func (controller *defaultGroupController) Delete(ctx context.Context, ingressKey types.NamespacedName) error {
	selector := controller.nameTagGen.TagTGGroup(ingressKey.Namespace, ingressKey.Name)
	tgGroup := TargetGroupGroup{
		selector: selector,
	}
	return controller.GC(ctx, tgGroup)
}

// TODO, should be k8s utils :D
func (controller *defaultGroupController) extractIngressBackends(ingress *extensions.Ingress) []extensions.IngressBackend {
	var output []extensions.IngressBackend
	if ingress.Spec.Backend != nil {
		output = append(output, *ingress.Spec.Backend)
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			output = append(output, path.Backend)
		}
	}
	return output
}
