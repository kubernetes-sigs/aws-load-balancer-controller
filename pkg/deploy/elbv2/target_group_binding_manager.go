package elbv2

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	defaultWaitTGBDeletionPollInterval = 200 * time.Millisecond
	defaultWaitTGBDeletionTimeout      = 60 * time.Second
)

// TargetGroupBindingManager is responsible for create/update/delete TargetGroupBinding resources.
type TargetGroupBindingManager interface {
	Create(ctx context.Context, resTGB *elbv2model.TargetGroupBindingResource) (elbv2model.TargetGroupBindingResourceStatus, error)

	Update(ctx context.Context, resTGB *elbv2model.TargetGroupBindingResource, k8sTGB *elbv2api.TargetGroupBinding) (elbv2model.TargetGroupBindingResourceStatus, error)

	Delete(ctx context.Context, k8sTGB *elbv2api.TargetGroupBinding) error
}

// NewDefaultTargetGroupBindingManager constructs new defaultTargetGroupBindingManager
func NewDefaultTargetGroupBindingManager(k8sClient client.Client, trackingProvider tracking.Provider, logger logr.Logger) *defaultTargetGroupBindingManager {
	return &defaultTargetGroupBindingManager{
		k8sClient:        k8sClient,
		trackingProvider: trackingProvider,
		logger:           logger,

		waitTGBDeletionPollInterval: defaultWaitTGBDeletionPollInterval,
		waitTGBDeletionTimeout:      defaultWaitTGBDeletionTimeout,
	}
}

var _ TargetGroupBindingManager = &defaultTargetGroupBindingManager{}

type defaultTargetGroupBindingManager struct {
	k8sClient        client.Client
	trackingProvider tracking.Provider
	logger           logr.Logger

	waitTGBDeletionPollInterval time.Duration
	waitTGBDeletionTimeout      time.Duration
}

func (m *defaultTargetGroupBindingManager) Create(ctx context.Context, resTGB *elbv2model.TargetGroupBindingResource) (elbv2model.TargetGroupBindingResourceStatus, error) {
	tgARN, err := resTGB.Spec.Template.Spec.TargetGroupARN.Resolve(ctx)
	if err != nil {
		return elbv2model.TargetGroupBindingResourceStatus{}, err
	}
	stackLabels := m.trackingProvider.StackLabels(resTGB.Stack())
	k8sTGBSpec := elbv2api.TargetGroupBindingSpec{
		TargetGroupARN: tgARN,
		TargetType:     resTGB.Spec.Template.Spec.TargetType,
		ServiceRef:     resTGB.Spec.Template.Spec.ServiceRef,
	}
	if resTGB.Spec.Template.Spec.Networking != nil {
		k8sTGBNetworking, err := buildK8sTargetGroupBindingNetworking(ctx, *resTGB.Spec.Template.Spec.Networking)
		if err != nil {
			return elbv2model.TargetGroupBindingResourceStatus{}, err
		}
		k8sTGBSpec.Networking = &k8sTGBNetworking
	}

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
	tgARN, err := resTGB.Spec.Template.Spec.TargetGroupARN.Resolve(ctx)
	if err != nil {
		return elbv2model.TargetGroupBindingResourceStatus{}, err
	}
	k8sTGBSpec := elbv2api.TargetGroupBindingSpec{
		TargetGroupARN: tgARN,
		TargetType:     resTGB.Spec.Template.Spec.TargetType,
		ServiceRef:     resTGB.Spec.Template.Spec.ServiceRef,
	}
	if resTGB.Spec.Template.Spec.Networking != nil {
		k8sTGBNetworking, err := buildK8sTargetGroupBindingNetworking(ctx, *resTGB.Spec.Template.Spec.Networking)
		if err != nil {
			return elbv2model.TargetGroupBindingResourceStatus{}, err
		}
		k8sTGBSpec.Networking = &k8sTGBNetworking
	}

	oldK8sTGB := k8sTGB.DeepCopy()
	k8sTGB.Spec = k8sTGBSpec
	if err := m.k8sClient.Patch(ctx, k8sTGB, client.MergeFrom(oldK8sTGB)); err != nil {
		return elbv2model.TargetGroupBindingResourceStatus{}, err
	}
	return buildResTargetGroupBindingStatus(k8sTGB), nil
}

func (m *defaultTargetGroupBindingManager) Delete(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	m.logger.Info("deleting targetGroupBinding",
		"targetGroupBinding", k8s.NamespacedName(tgb))
	if err := m.k8sClient.Delete(ctx, tgb); err != nil {
		return err
	}
	if err := m.waitUntilTargetGroupBindingDeleted(ctx, tgb); err != nil {
		return errors.Wrap(err, "failed to wait targetGroupBinding deletion")
	}
	m.logger.Info("deleted targetGroupBinding",
		"targetGroupBinding", k8s.NamespacedName(tgb))
	return nil
}

func (m *defaultTargetGroupBindingManager) waitUntilTargetGroupBindingDeleted(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	ctx, cancel := context.WithTimeout(ctx, m.waitTGBDeletionTimeout)
	defer cancel()

	observedTGB := &elbv2api.TargetGroupBinding{}
	return wait.PollImmediateUntil(m.waitTGBDeletionPollInterval, func() (bool, error) {
		if err := m.k8sClient.Get(ctx, k8s.NamespacedName(tgb), observedTGB); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, ctx.Done())
}

func buildK8sTargetGroupBindingNetworking(ctx context.Context, resTGBNetworking elbv2model.TargetGroupBindingNetworking) (elbv2api.TargetGroupBindingNetworking, error) {
	k8sIngress := make([]elbv2api.NetworkingIngressRule, 0, len(resTGBNetworking.Ingress))
	for _, rule := range resTGBNetworking.Ingress {
		k8sPeers := make([]elbv2api.NetworkingPeer, 0, len(rule.From))
		for _, peer := range rule.From {
			peer, err := buildK8sNetworkingPeer(ctx, peer)
			if err != nil {
				return elbv2api.TargetGroupBindingNetworking{}, err
			}
			k8sPeers = append(k8sPeers, peer)
		}
		k8sIngress = append(k8sIngress, elbv2api.NetworkingIngressRule{
			From:  k8sPeers,
			Ports: rule.Ports,
		})
	}
	return elbv2api.TargetGroupBindingNetworking{
		Ingress: k8sIngress,
	}, nil
}

func buildK8sNetworkingPeer(ctx context.Context, resNetworkingPeer elbv2model.NetworkingPeer) (elbv2api.NetworkingPeer, error) {
	if resNetworkingPeer.IPBlock != nil {
		return elbv2api.NetworkingPeer{
			IPBlock: resNetworkingPeer.IPBlock,
		}, nil
	}
	if resNetworkingPeer.SecurityGroup != nil {
		groupID, err := resNetworkingPeer.SecurityGroup.GroupID.Resolve(ctx)
		if err != nil {
			return elbv2api.NetworkingPeer{}, err
		}
		return elbv2api.NetworkingPeer{
			SecurityGroup: &elbv2api.SecurityGroup{
				GroupID: groupID,
			},
		}, nil
	}
	return elbv2api.NetworkingPeer{}, errors.New("either ipBlock or securityGroup should be specified")
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
