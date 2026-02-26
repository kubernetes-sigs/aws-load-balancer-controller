package elbv2

import (
	"context"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sync"

	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const apiPathMutateELBv2TargetGroupBinding = "/mutate-elbv2-k8s-aws-v1beta1-targetgroupbinding"

type tgCacheObject struct {
	tg    *elbv2types.TargetGroup
	error error
}

// NewTargetGroupBindingMutator returns a mutator for TargetGroupBinding CRD.
// elbv2Provider is optional; when set, cross-region target group ARNs are resolved to a regional ELBV2 client.
func NewTargetGroupBindingMutator(elbv2Client services.ELBV2, defaultRegion string, elbv2Provider ELBV2ClientProvider, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector) *targetGroupBindingMutator {
	return &targetGroupBindingMutator{
		elbv2Client:      elbv2Client,
		defaultRegion:    defaultRegion,
		elbv2Provider:    elbv2Provider,
		logger:           logger,
		metricsCollector: metricsCollector,
	}
}

var _ webhook.Mutator = &targetGroupBindingMutator{}

type targetGroupBindingMutator struct {
	elbv2Client      services.ELBV2
	defaultRegion    string
	elbv2Provider    ELBV2ClientProvider
	logger           logr.Logger
	metricsCollector lbcmetrics.MetricCollector
}

func (m *targetGroupBindingMutator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &elbv2api.TargetGroupBinding{}, nil
}

func (m *targetGroupBindingMutator) MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {

	tgb := obj.(*elbv2api.TargetGroupBinding)

	if tgb.Spec.TargetGroupARN == "" && tgb.Spec.TargetGroupName == "" {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutateELBv2TargetGroupBinding, "checkTargetGroupArnOrName")
		return nil, errors.Errorf("must provide either TargetGroupARN or TargetGroupName")
	}
	if err := m.getArnFromNameIfNeeded(ctx, tgb); err != nil {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutateELBv2TargetGroupBinding, "getArnFromNameIfNeeded")
		return nil, err
	}

	elbv2ForTGB, err := resolveELBV2ForTGB(m.elbv2Client, m.defaultRegion, m.elbv2Provider, tgb.Spec.TargetGroupARN)
	if err != nil {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutateELBv2TargetGroupBinding, "resolveELBV2ForTGB")
		return nil, errors.Wrapf(err, "unable to create ELBV2 client for target group %s", tgb.Spec.TargetGroupARN)
	}

	targetGroupCache := sync.OnceValue(func() tgCacheObject {
		targetGroup, err := getTargetGroupFromAWS(ctx, elbv2ForTGB, tgb)
		return tgCacheObject{
			tg:    targetGroup,
			error: err,
		}
	})
	if err := m.defaultingTargetType(tgb, targetGroupCache); err != nil {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutateELBv2TargetGroupBinding, "defaultingTargetType")
		return nil, err
	}
	if err := m.defaultingIPAddressType(tgb, targetGroupCache); err != nil {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutateELBv2TargetGroupBinding, "defaultingIPAddressType")
		return nil, err
	}
	if err := m.defaultingVpcID(tgb, targetGroupCache); err != nil {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutateELBv2TargetGroupBinding, "defaultingVpcID")
		return nil, err
	}
	if err := m.defaultingTargetGroupProtocol(tgb, targetGroupCache); err != nil {
		m.metricsCollector.ObserveWebhookMutationError(apiPathMutateELBv2TargetGroupBinding, "defaultingTargetGroupProtocol")
		return nil, err
	}
	return tgb, nil
}

func (m *targetGroupBindingMutator) getArnFromNameIfNeeded(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	if tgb.Spec.TargetGroupARN == "" && tgb.Spec.TargetGroupName != "" {
		tgObj, err := getTargetGroupsByNameFromAWS(ctx, m.elbv2Client, tgb)
		if err != nil {
			return err
		}
		tgb.Spec.TargetGroupARN = *tgObj.TargetGroupArn
	}
	return nil
}

func (m *targetGroupBindingMutator) MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
	return obj, nil
}

func (m *targetGroupBindingMutator) defaultingTargetType(tgb *elbv2api.TargetGroupBinding, tgCache func() tgCacheObject) error {
	if tgb.Spec.TargetType != nil {
		return nil
	}
	sdkTargetType, err := m.obtainSDKTargetTypeFromAWS(tgCache)
	if err != nil {
		return errors.Wrap(err, "couldn't determine TargetType")
	}
	var targetType elbv2api.TargetType
	switch sdkTargetType {
	case string(elbv2types.TargetTypeEnumInstance):
		targetType = elbv2api.TargetTypeInstance
	case string(elbv2types.TargetTypeEnumIp):
		targetType = elbv2api.TargetTypeIP
	default:
		return errors.Errorf("unsupported TargetType: %v", sdkTargetType)
	}

	tgb.Spec.TargetType = &targetType
	return nil
}

func (m *targetGroupBindingMutator) defaultingIPAddressType(tgb *elbv2api.TargetGroupBinding, tgCache func() tgCacheObject) error {
	if tgb.Spec.IPAddressType != nil {
		return nil
	}
	targetGroupIPAddressType, err := m.getTargetGroupIPAddressTypeFromAWS(tgCache)
	if err != nil {
		return errors.Wrap(err, "unable to get target group IP address type")
	}
	tgb.Spec.IPAddressType = &targetGroupIPAddressType
	return nil
}

func (m *targetGroupBindingMutator) defaultingVpcID(tgb *elbv2api.TargetGroupBinding, tgCache func() tgCacheObject) error {
	if tgb.Spec.VpcID != "" {
		return nil
	}
	vpcId, err := m.getVpcIDFromAWS(tgCache)
	if err != nil {
		return errors.Wrap(err, "unable to get target group VpcID")
	}
	tgb.Spec.VpcID = vpcId
	return nil
}

func (m *targetGroupBindingMutator) defaultingTargetGroupProtocol(tgb *elbv2api.TargetGroupBinding, tgCache func() tgCacheObject) error {
	if tgb.Spec.TargetGroupProtocol != nil {
		return nil
	}
	sdkTgProtocol, err := obtainSDKTargetGroupProtocolFromAWS(tgCache)
	if err != nil {
		return errors.Wrap(err, "couldn't determine TargetGroup protocol")
	}
	tgb.Spec.TargetGroupProtocol = (*elbv2model.Protocol)(&sdkTgProtocol)
	return nil
}

func (m *targetGroupBindingMutator) obtainSDKTargetTypeFromAWS(tgCache func() tgCacheObject) (string, error) {
	obj := tgCache()
	targetGroup := obj.tg
	err := obj.error
	if err != nil {
		return "", err
	}
	return string(targetGroup.TargetType), nil
}

func obtainSDKTargetGroupProtocolFromAWS(tgCache func() tgCacheObject) (string, error) {
	obj := tgCache()
	targetGroup := obj.tg
	err := obj.error
	if err != nil {
		return "", err
	}
	return string(targetGroup.Protocol), nil
}

// getTargetGroupIPAddressTypeFromAWS returns the target group IP address type of AWS target group
func (m *targetGroupBindingMutator) getTargetGroupIPAddressTypeFromAWS(tgCache func() tgCacheObject) (elbv2api.TargetGroupIPAddressType, error) {
	obj := tgCache()
	targetGroup := obj.tg
	err := obj.error
	if err != nil {
		return "", err
	}
	var ipAddressType elbv2api.TargetGroupIPAddressType
	switch string(targetGroup.IpAddressType) {
	case string(elbv2types.TargetGroupIpAddressTypeEnumIpv6):
		ipAddressType = elbv2api.TargetGroupIPAddressTypeIPv6
	case string(elbv2types.TargetGroupIpAddressTypeEnumIpv4), "":
		ipAddressType = elbv2api.TargetGroupIPAddressTypeIPv4
	default:
		return "", errors.Errorf("unsupported IPAddressType: %v", string(targetGroup.IpAddressType))
	}
	return ipAddressType, nil
}

func (m *targetGroupBindingMutator) getVpcIDFromAWS(tgCache func() tgCacheObject) (string, error) {
	obj := tgCache()
	targetGroup := obj.tg
	err := obj.error
	if err != nil {
		return "", err
	}
	return awssdk.ToString(targetGroup.VpcId), nil
}

// +kubebuilder:webhook:path=/mutate-elbv2-k8s-aws-v1beta1-targetgroupbinding,mutating=true,failurePolicy=fail,groups=elbv2.k8s.aws,resources=targetgroupbindings,verbs=create;update,versions=v1beta1,name=mtargetgroupbinding.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (m *targetGroupBindingMutator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathMutateELBv2TargetGroupBinding, webhook.MutatingWebhookForMutator(m, mgr.GetScheme()))
}
