package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const apiPathMutateELBv2TargetGroupBinding = "/mutate-elbv2-k8s-aws-v1beta1-targetgroupbinding"

// NewTargetGroupBindingMutator returns a mutator for TargetGroupBinding CRD.
func NewTargetGroupBindingMutator(elbv2Client services.ELBV2, logger logr.Logger) *targetGroupBindingMutator {
	return &targetGroupBindingMutator{
		elbv2Client: elbv2Client,
		logger:      logger,
	}
}

var _ webhook.Mutator = &targetGroupBindingMutator{}

type targetGroupBindingMutator struct {
	elbv2Client services.ELBV2
	logger      logr.Logger
}

func (m *targetGroupBindingMutator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &elbv2api.TargetGroupBinding{}, nil
}

func (m *targetGroupBindingMutator) MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	tgb := obj.(*elbv2api.TargetGroupBinding)
	if err := m.defaultingTargetType(ctx, tgb); err != nil {
		return nil, err
	}
	if err := m.defaultingIPAddressType(ctx, tgb); err != nil {
		return nil, err
	}
	return tgb, nil
}

func (m *targetGroupBindingMutator) MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
	return obj, nil
}

func (m *targetGroupBindingMutator) defaultingTargetType(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	if tgb.Spec.TargetType != nil {
		return nil
	}
	tgARN := tgb.Spec.TargetGroupARN
	sdkTargetType, err := m.obtainSDKTargetTypeFromAWS(ctx, tgARN)
	if err != nil {
		return errors.Wrap(err, "couldn't determine TargetType")
	}
	var targetType elbv2api.TargetType
	switch sdkTargetType {
	case elbv2sdk.TargetTypeEnumInstance:
		targetType = elbv2api.TargetTypeInstance
	case elbv2sdk.TargetTypeEnumIp:
		targetType = elbv2api.TargetTypeIP
	default:
		return errors.Errorf("unsupported TargetType: %v", sdkTargetType)
	}

	tgb.Spec.TargetType = &targetType
	return nil
}

func (m *targetGroupBindingMutator) defaultingIPAddressType(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	if tgb.Spec.IPAddressType != nil {
		return nil
	}
	targetGroupIPAddressType, err := m.getTargetGroupIPAddressTypeFromAWS(ctx, tgb.Spec.TargetGroupARN)
	if err != nil {
		return errors.Wrap(err, "unable to get target group IP address type")
	}
	tgb.Spec.IPAddressType = &targetGroupIPAddressType
	return nil
}

func (m *targetGroupBindingMutator) obtainSDKTargetTypeFromAWS(ctx context.Context, tgARN string) (string, error) {
	targetGroup, err := m.getTargetGroupFromAWS(ctx, tgARN)
	if err != nil {
		return "", err
	}
	return awssdk.StringValue(targetGroup.TargetType), nil
}

// getTargetGroupIPAddressTypeFromAWS returns the target group IP address type of AWS target group
func (m *targetGroupBindingMutator) getTargetGroupIPAddressTypeFromAWS(ctx context.Context, tgARN string) (elbv2api.TargetGroupIPAddressType, error) {
	targetGroup, err := m.getTargetGroupFromAWS(ctx, tgARN)
	if err != nil {
		return "", err
	}
	var ipAddressType elbv2api.TargetGroupIPAddressType
	switch awssdk.StringValue(targetGroup.IpAddressType) {
	case elbv2sdk.TargetGroupIpAddressTypeEnumIpv6:
		ipAddressType = elbv2api.TargetGroupIPAddressTypeIPv6
	case elbv2sdk.TargetGroupIpAddressTypeEnumIpv4, "":
		ipAddressType = elbv2api.TargetGroupIPAddressTypeIPv4
	default:
		return "", errors.Errorf("unsupported IPAddressType: %v", awssdk.StringValue(targetGroup.IpAddressType))
	}
	return ipAddressType, nil
}

func (m *targetGroupBindingMutator) getTargetGroupFromAWS(ctx context.Context, tgARN string) (*elbv2sdk.TargetGroup, error) {
	req := &elbv2sdk.DescribeTargetGroupsInput{
		TargetGroupArns: awssdk.StringSlice([]string{tgARN}),
	}
	tgList, err := m.elbv2Client.DescribeTargetGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(tgList) != 1 {
		return nil, errors.Errorf("expecting a single targetGroup but got %v", len(tgList))
	}
	return tgList[0], nil
}

// +kubebuilder:webhook:path=/mutate-elbv2-k8s-aws-v1beta1-targetgroupbinding,mutating=true,failurePolicy=fail,groups=elbv2.k8s.aws,resources=targetgroupbindings,verbs=create;update,versions=v1beta1,name=mtargetgroupbinding.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (m *targetGroupBindingMutator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathMutateELBv2TargetGroupBinding, webhook.MutatingWebhookForMutator(m))
}
