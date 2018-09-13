package sg

import "github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"

// InstanceAttachment represents the attachment of securityGroups to instance
type InstanceAttachment struct {
	GroupID string
	Targets tg.TargetGroups
}

type InstanceAttachementController interface {
	Reconcile(*InstanceAttachment) error
	Delete(*InstanceAttachment) error
}

func (attachment *InstanceAttachment) Reconcile() error {
	return nil
}
