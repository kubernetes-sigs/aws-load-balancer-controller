package tg

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	cloud aws.CloudAPI,
	store store.Storer,
	nameTagGen NameTagGenerator,
	tagsController tags.Controller,
	endpointResolver backend.EndpointResolver,
	client client.Client) GroupController {
	tgController := NewController(cloud, store, nameTagGen, tagsController, endpointResolver, client)
	return &defaultGroupController{
		cloud:        cloud,
		store:        store,
		nameTagGen:   nameTagGen,
		tgController: tgController,
	}
}

var _ GroupController = (*defaultGroupController)(nil)

type defaultGroupController struct {
	cloud      aws.CloudAPI
	store      store.Storer
	nameTagGen NameTagGenerator

	tgController Controller
}

func (controller *defaultGroupController) Reconcile(ctx context.Context, ingress *extensions.Ingress) (TargetGroupGroup, error) {
	tgByBackend := make(map[extensions.IngressBackend]TargetGroup)

	backends, err := ExtractTargetGroupBackends(ingress)
	if err != nil {
		return TargetGroupGroup{}, err
	}
	for _, backend := range backends {
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
	arns, err := controller.cloud.GetResourcesByFilters(tagFilters, aws.ResourceTypeEnumELBTargetGroup)
	if err != nil {
		return fmt.Errorf("failed to get targetGroups due to %v", err)
	}
	currentTgArns := sets.NewString(arns...)
	unusedTgArns := currentTgArns.Difference(usedTgArns)
	for arn := range unusedTgArns {
		albctx.GetLogger(ctx).Infof("deleting target group %v", arn)
		controller.tgController.StopReconcilingPodConditionStatus(arn)
		if err := controller.cloud.DeleteTargetGroupByArn(ctx, arn); err != nil {
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

func ExtractTargetGroupBackends(ingress *extensions.Ingress) ([]extensions.IngressBackend, error) {
	var rawIngBackends []extensions.IngressBackend
	if ingress.Spec.Backend != nil {
		rawIngBackends = append(rawIngBackends, *ingress.Spec.Backend)
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			rawIngBackends = append(rawIngBackends, path.Backend)
		}
	}

	var ingBackends []extensions.IngressBackend
	for _, ingBackend := range rawIngBackends {
		if action.Use(ingBackend.ServicePort.String()) {
			continue
		}
		ingBackends = append(ingBackends, ingBackend)
	}

	raw, err := action.NewParser().Parse(ingress)
	if err != nil {
		return nil, err
	}

	actions := raw.(*action.Config).Actions

	for _, action := range actions {
		if aws.StringValue(action.Type) != elbv2.ActionTypeEnumForward {
			continue
		}

		for _, tgt := range action.ForwardConfig.TargetGroups {
			if tgt.ServiceName != nil {
				ingBackends = append(ingBackends, extensions.IngressBackend{
					ServiceName: aws.StringValue(tgt.ServiceName),
					ServicePort: intstr.Parse(aws.StringValue(tgt.ServicePort)),
				})
			}
		}
	}

	return ingBackends, nil
}
