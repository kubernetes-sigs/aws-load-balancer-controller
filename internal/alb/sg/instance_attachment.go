package sg

import (
	"context"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"k8s.io/apimachinery/pkg/types"
)

// InstanceAttachment manages SecurityGroups on worker nodes.
type InstanceAttachmentController interface {
	// Reconcile will setup SecurityGroup on worker nodes to allow inbound traffic from LoadBalancer(with lbSGID) to targets in tgGroup.
	Reconcile(ctx context.Context, ingKey types.NamespacedName, lbSGID string, tgGroup tg.TargetGroupGroup) error

	// Delete will cleanup resources setup in Reconcile.
	Delete(ctx context.Context, ingKey types.NamespacedName) error
}

func NewInstanceAttachmentController(sgController SecurityGroupController,
	targetENIsResolver TargetENIsResolver,
	nameTagGen NameTagGenerator,
	store store.Storer,
	cloud aws.CloudAPI) InstanceAttachmentController {

	controllerV1 := NewInstanceAttachmentControllerV1(sgController, targetENIsResolver, nameTagGen, store, cloud)
	controllerV2 := NewInstanceAttachmentControllerV2(sgController, targetENIsResolver, nameTagGen, store, cloud)
	return &migratableInstanceAttachmentController{
		activated:  controllerV2,
		deprecated: []InstanceAttachmentController{controllerV1},
	}
}

type migratableInstanceAttachmentController struct {
	// in-use InstanceAttachmentController version
	activated InstanceAttachmentController

	// deprecated InstanceAttachmentController versions
	deprecated []InstanceAttachmentController
}

func (c *migratableInstanceAttachmentController) Reconcile(ctx context.Context, ingKey types.NamespacedName, lbSGID string, tgGroup tg.TargetGroupGroup) error {
	for _, deprecatedC := range c.deprecated {
		if err := deprecatedC.Delete(ctx, ingKey); err != nil {
			return err
		}
	}
	return c.activated.Reconcile(ctx, ingKey, lbSGID, tgGroup)
}

func (c *migratableInstanceAttachmentController) Delete(ctx context.Context, ingKey types.NamespacedName) error {
	for _, deprecatedC := range c.deprecated {
		if err := deprecatedC.Delete(ctx, ingKey); err != nil {
			return err
		}
	}
	return c.activated.Delete(ctx, ingKey)
}
