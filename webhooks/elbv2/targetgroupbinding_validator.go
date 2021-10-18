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
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
)

const apiPathValidateELBv2TargetGroupBinding = "/validate-elbv2-k8s-aws-v1beta1-targetgroupbinding"

// NewTargetGroupBindingValidator returns a mutator for TargetGroupBinding CRD.
func NewTargetGroupBindingValidator(k8sClient client.Client, elbv2Client services.ELBV2, logger logr.Logger) *targetGroupBindingValidator {
	return &targetGroupBindingValidator{
		k8sClient:   k8sClient,
		elbv2Client: elbv2Client,
		logger:      logger,
	}
}

var _ webhook.Validator = &targetGroupBindingValidator{}

type targetGroupBindingValidator struct {
	k8sClient   client.Client
	elbv2Client services.ELBV2
	logger      logr.Logger
}

func (v *targetGroupBindingValidator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &elbv2api.TargetGroupBinding{}, nil
}

func (v *targetGroupBindingValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	tgb := obj.(*elbv2api.TargetGroupBinding)
	if err := v.checkRequiredFields(tgb); err != nil {
		return err
	}
	if err := v.checkNodeSelector(tgb); err != nil {
		return err
	}
	if err := v.checkExistingTargetGroups(tgb); err != nil {
		return err
	}
	if err := v.checkTargetGroupIPAddressType(ctx, tgb); err != nil {
		return err
	}
	return nil
}

func (v *targetGroupBindingValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
	tgb := obj.(*elbv2api.TargetGroupBinding)
	oldTgb := oldObj.(*elbv2api.TargetGroupBinding)
	if err := v.checkRequiredFields(tgb); err != nil {
		return err
	}
	if err := v.checkImmutableFields(tgb, oldTgb); err != nil {
		return err
	}
	if err := v.checkNodeSelector(tgb); err != nil {
		return err
	}
	return nil
}

func (v *targetGroupBindingValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// checkRequiredFields will check required fields are not absent.
func (v *targetGroupBindingValidator) checkRequiredFields(tgb *elbv2api.TargetGroupBinding) error {
	var absentRequiredFields []string
	if tgb.Spec.TargetType == nil {
		absentRequiredFields = append(absentRequiredFields, "spec.targetType")
	}
	if len(absentRequiredFields) != 0 {
		return errors.Errorf("%s must specify these fields: %s", "TargetGroupBinding", strings.Join(absentRequiredFields, ","))
	}
	return nil
}

// checkImmutableFields will check immutable fields are not changed.
func (v *targetGroupBindingValidator) checkImmutableFields(tgb *elbv2api.TargetGroupBinding, oldTGB *elbv2api.TargetGroupBinding) error {
	var changedImmutableFields []string
	if tgb.Spec.TargetGroupARN != oldTGB.Spec.TargetGroupARN {
		changedImmutableFields = append(changedImmutableFields, "spec.targetGroupARN")
	}
	if (tgb.Spec.TargetType == nil) != (oldTGB.Spec.TargetType == nil) {
		changedImmutableFields = append(changedImmutableFields, "spec.targetType")
	}
	if tgb.Spec.TargetType != nil && oldTGB.Spec.TargetType != nil && (*tgb.Spec.TargetType) != (*oldTGB.Spec.TargetType) {
		changedImmutableFields = append(changedImmutableFields, "spec.targetType")
	}
	if (oldTGB.Spec.IPAddressType == nil && tgb.Spec.IPAddressType != nil && *tgb.Spec.IPAddressType != elbv2api.TargetGroupIPAddressTypeIPv4) ||
		(tgb.Spec.IPAddressType == nil && oldTGB.Spec.IPAddressType != nil) {
		changedImmutableFields = append(changedImmutableFields, "spec.ipAddressType")
	}
	if oldTGB.Spec.IPAddressType != nil && tgb.Spec.IPAddressType != nil && (*oldTGB.Spec.IPAddressType) != (*tgb.Spec.IPAddressType) {
		changedImmutableFields = append(changedImmutableFields, "spec.ipAddressType")
	}
	if len(changedImmutableFields) != 0 {
		return errors.Errorf("%s update may not change these fields: %s", "TargetGroupBinding", strings.Join(changedImmutableFields, ","))
	}
	return nil
}

// checkExistingTargetGroups will check for unique TargetGroup per TargetGroupBinding
func (v *targetGroupBindingValidator) checkExistingTargetGroups(tgb *elbv2api.TargetGroupBinding) error {
	ctx := context.Background()
	tgbList := elbv2api.TargetGroupBindingList{}
	if err := v.k8sClient.List(ctx, &tgbList); err != nil {
		return errors.Wrap(err, "failed to list TargetGroupBindings in the cluster")
	}
	for _, tgbObj := range tgbList.Items {
		if tgbObj.Spec.TargetGroupARN == tgb.Spec.TargetGroupARN {
			return errors.Errorf("TargetGroup %v is already bound to TargetGroupBinding %v", tgb.Spec.TargetGroupARN, k8s.NamespacedName(&tgbObj).String())
		}
	}
	return nil
}

//checkNodeSelector ensures that NodeSelector is only set when TargetType is ip
func (v *targetGroupBindingValidator) checkNodeSelector(tgb *elbv2api.TargetGroupBinding) error {
	if (*tgb.Spec.TargetType == elbv2api.TargetTypeIP) && (tgb.Spec.NodeSelector != nil) {
		return errors.Errorf("TargetGroupBinding cannot set NodeSelector when TargetType is ip")
	}
	return nil
}

// checkTargetGroupIPAddressType ensures IP address type matches with that on the AWS target group
func (v *targetGroupBindingValidator) checkTargetGroupIPAddressType(ctx context.Context, tgb *elbv2api.TargetGroupBinding) error {
	targetGroupIPAddressType, err := v.getTargetGroupIPAddressTypeFromAWS(ctx, tgb.Spec.TargetGroupARN)
	if err != nil {
		return errors.Wrap(err, "unable to get target group IP address type")
	}
	if (tgb.Spec.IPAddressType != nil && *tgb.Spec.IPAddressType != targetGroupIPAddressType) ||
		(tgb.Spec.IPAddressType == nil && targetGroupIPAddressType != elbv2api.TargetGroupIPAddressTypeIPv4) {
		return errors.Errorf("invalid IP address type %v for TargetGroup %v", targetGroupIPAddressType, tgb.Spec.TargetGroupARN)
	}
	return nil
}

// getTargetGroupIPAddressTypeFromAWS returns the target group IP address type of AWS target group
func (v *targetGroupBindingValidator) getTargetGroupIPAddressTypeFromAWS(ctx context.Context, tgARN string) (elbv2api.TargetGroupIPAddressType, error) {
	targetGroup, err := v.getTargetGroupFromAWS(ctx, tgARN)
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

// getTargetGroupFromAWS returns the AWS target group corresponding to the ARN
func (v *targetGroupBindingValidator) getTargetGroupFromAWS(ctx context.Context, tgARN string) (*elbv2sdk.TargetGroup, error) {
	req := &elbv2sdk.DescribeTargetGroupsInput{
		TargetGroupArns: awssdk.StringSlice([]string{tgARN}),
	}
	tgList, err := v.elbv2Client.DescribeTargetGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(tgList) != 1 {
		return nil, errors.Errorf("expecting a single targetGroup but got %v", len(tgList))
	}
	return tgList[0], nil
}

// +kubebuilder:webhook:path=/validate-elbv2-k8s-aws-v1beta1-targetgroupbinding,mutating=false,failurePolicy=fail,groups=elbv2.k8s.aws,resources=targetgroupbindings,verbs=create;update,versions=v1beta1,name=vtargetgroupbinding.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (v *targetGroupBindingValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateELBv2TargetGroupBinding, webhook.ValidatingWebhookForValidator(v))
}
