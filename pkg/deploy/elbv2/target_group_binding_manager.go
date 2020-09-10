package elbv2

import (
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/tagging"
	elbv2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TargetGroupBindingManager is responsible for create/update/delete TargetGroupBinding resources.
type TargetGroupBindingManager interface {
	Create(ctx context.Context, resTGB *elbv2model.TargetGroupBindingResource) (elbv2model.TargetGroupBindingResourceStatus, error)

	Update(ctx context.Context, resTGB *elbv2model.TargetGroupBindingResource, k8sTGB *elbv2api.TargetGroupBinding) (elbv2model.TargetGroupBindingResourceStatus, error)

	Delete(ctx context.Context, k8sTGB *elbv2api.TargetGroupBinding) error
}

// NewDefaultTargetGroupBindingManager constructs new defaultTargetGroupBindingManager
func NewDefaultTargetGroupBindingManager(k8sClient client.Client, taggingProvider tagging.Provider, logger logr.Logger) *defaultTargetGroupBindingManager {
	return &defaultTargetGroupBindingManager{
		k8sClient:       k8sClient,
		taggingProvider: taggingProvider,
		logger:          logger,
	}
}

var _ TargetGroupBindingManager = &defaultTargetGroupBindingManager{}

type defaultTargetGroupBindingManager struct {
	k8sClient       client.Client
	taggingProvider tagging.Provider
	logger          logr.Logger
}

func (m *defaultTargetGroupBindingManager) Create(ctx context.Context, resTGB *elbv2model.TargetGroupBindingResource) (elbv2model.TargetGroupBindingResourceStatus, error) {
	tgARN, err := resTGB.Spec.TargetGroupARN.Resolve(ctx)
	if err != nil {
		return elbv2model.TargetGroupBindingResourceStatus{}, err
	}
	stackLabels := m.taggingProvider.StackTags(resTGB.Stack())
	k8sTGBSpec := resTGB.Spec.Template.Spec
	k8sTGBSpec.TargetGroupARN = tgARN
	k8sTGB := &elbv2api.TargetGroupBinding{
		ObjectMeta: v1.ObjectMeta{
			Namespace: resTGB.Spec.Template.Namespace,
			Name:      resTGB.Spec.Template.Name,
			Labels:    stackLabels,
		},
		Spec: k8sTGBSpec,
	}
	if err := m.k8sClient.Create(ctx, k8sTGB); err != nil {
		return elbv2model.TargetGroupBindingResourceStatus{}, err
	}
	return buildResTargetGroupBindingStatus(k8sTGB), nil
}

func (m *defaultTargetGroupBindingManager) Update(ctx context.Context, resTGB *elbv2model.TargetGroupBindingResource, k8sTGB *elbv2api.TargetGroupBinding) (elbv2model.TargetGroupBindingResourceStatus, error) {
	tgARN, err := resTGB.Spec.TargetGroupARN.Resolve(ctx)
	if err != nil {
		return elbv2model.TargetGroupBindingResourceStatus{}, err
	}
	k8sTGBSpec := resTGB.Spec.Template.Spec
	k8sTGBSpec.TargetGroupARN = tgARN
	oldK8sTGB := k8sTGB.DeepCopy()
	k8sTGB.Spec = k8sTGBSpec
	if err := m.k8sClient.Patch(ctx, k8sTGB, client.MergeFrom(oldK8sTGB)); err != nil {
		return elbv2model.TargetGroupBindingResourceStatus{}, err
	}
	return buildResTargetGroupBindingStatus(k8sTGB), nil
}

func (m *defaultTargetGroupBindingManager) Delete(ctx context.Context, k8sTGB *elbv2api.TargetGroupBinding) error {
	return m.k8sClient.Delete(ctx, k8sTGB)
}

func buildResTargetGroupBindingStatus(k8sTGB *elbv2api.TargetGroupBinding) elbv2model.TargetGroupBindingResourceStatus {
	return elbv2model.TargetGroupBindingResourceStatus{
		TargetGroupBindingRef: corev1.ObjectReference{
			Namespace: k8sTGB.Namespace,
			Name:      k8sTGB.Name,
			UID:       k8sTGB.UID,
		},
	}
}
