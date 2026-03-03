package elbv2

import (
	"context"
	"fmt"
	"regexp"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/types"

	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathValidateELBv2TargetGroupBinding = "/validate-elbv2-k8s-aws-v1beta1-targetgroupbinding"
	vpcIDValidationErr                     = "ValidationError: vpcID %v failed to satisfy constraint: VPC Id must begin with 'vpc-' followed by 8, 17 or 32 lowercase letters (a-f) or numbers."
	vpcIDNotMatchErr                       = "invalid VpcID %v doesnt match VpcID from TargetGroup %v"
	tgProtocolMismatch                     = "TargetGroup %v protocol differs (got %v, expected %v)"
	quicInstanceNotSupported               = "QUIC protocol is not supported for Instance target types."
)

var vpcIDPatternRegex = regexp.MustCompile("^(?:vpc-[0-9a-f]{8}|vpc-[0-9a-f]{17}|vpc-[0-9a-f]{32})$")

// NewTargetGroupBindingValidator returns a validator for TargetGroupBinding CRD.
// elbv2Provider is optional; when set, cross-region target group ARNs are resolved to a regional ELBV2 client.
func NewTargetGroupBindingValidator(k8sClient client.Client, elbv2Client services.ELBV2, defaultRegion string, elbv2Provider ELBV2ClientProvider, vpcID string, logger logr.Logger, metricsCollector lbcmetrics.MetricCollector) *targetGroupBindingValidator {
	return &targetGroupBindingValidator{
		k8sClient:        k8sClient,
		elbv2Client:      elbv2Client,
		defaultRegion:    defaultRegion,
		elbv2Provider:    elbv2Provider,
		logger:           logger,
		vpcID:            vpcID,
		metricsCollector: metricsCollector,
	}
}

var _ webhook.Validator = &targetGroupBindingValidator{}

type targetGroupBindingValidator struct {
	k8sClient        client.Client
	elbv2Client      services.ELBV2
	defaultRegion    string
	elbv2Provider    ELBV2ClientProvider
	logger           logr.Logger
	vpcID            string
	metricsCollector lbcmetrics.MetricCollector
}

func (v *targetGroupBindingValidator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &elbv2api.TargetGroupBinding{}, nil
}

func (v *targetGroupBindingValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	tgb := obj.(*elbv2api.TargetGroupBinding)

	elbv2ForTGB, err := resolveELBV2ForTGB(v.elbv2Client, v.defaultRegion, v.elbv2Provider, tgb.Spec.TargetGroupARN)
	if err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "resolveELBV2ForTGB")
		return errors.Wrapf(err, "unable to create ELBV2 client for target group %s", tgb.Spec.TargetGroupARN)
	}

	targetGroupCache := sync.OnceValue(func() tgCacheObject {
		targetGroup, err := getTargetGroupFromAWS(ctx, elbv2ForTGB, tgb)
		return tgCacheObject{
			tg:    targetGroup,
			error: err,
		}
	})

	if err := v.checkRequiredFields(ctx, tgb, elbv2ForTGB); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkRequiredFields")
		return err
	}
	if err := v.checkNodeSelector(tgb); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkNodeSelector")
		return err
	}
	if err := v.checkExistingTargetGroups(tgb); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkExistingTargetGroups")
		return err
	}
	if err := v.checkTargetGroupIPAddressType(tgb, targetGroupCache); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkTargetGroupIPAddressType")
		return err
	}
	if err := v.checkTargetGroupVpcID(tgb, targetGroupCache); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkTargetGroupVpcID")
		return err

	}
	if err := v.checkAssumeRoleConfig(tgb); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkAssumeRoleConfig")
		return err
	}

	if err := v.checkTargetGroupProtocol(tgb, targetGroupCache); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkTargetGroupProtocol")
		return err
	}

	return nil
}

func (v *targetGroupBindingValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
	tgb := obj.(*elbv2api.TargetGroupBinding)
	oldTgb := oldObj.(*elbv2api.TargetGroupBinding)

	elbv2ForTGB, err := resolveELBV2ForTGB(v.elbv2Client, v.defaultRegion, v.elbv2Provider, tgb.Spec.TargetGroupARN)
	if err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "resolveELBV2ForTGB")
		return errors.Wrapf(err, "unable to create ELBV2 client for target group %s", tgb.Spec.TargetGroupARN)
	}

	if err := v.checkRequiredFields(ctx, tgb, elbv2ForTGB); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkRequiredFields")
		return err
	}
	if err := v.checkImmutableFields(tgb, oldTgb); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkImmutableFields")
		return err
	}
	if err := v.checkNodeSelector(tgb); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkNodeSelector")
		return err
	}
	if err := v.checkAssumeRoleConfig(tgb); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkAssumeRoleConfig")
		return err
	}
	if err := v.checkExistingTargetGroups(tgb); err != nil {
		v.metricsCollector.ObserveWebhookValidationError(apiPathValidateELBv2TargetGroupBinding, "checkExistingTargetGroups")
		return err
	}
	return nil
}

func (v *targetGroupBindingValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// checkRequiredFields will check required fields are not absent.
// elbv2ForTGB is the ELBV2 client scoped to the correct region for this TGB.
func (v *targetGroupBindingValidator) checkRequiredFields(ctx context.Context, tgb *elbv2api.TargetGroupBinding, elbv2ForTGB services.ELBV2) error {
	var absentRequiredFields []string
	if tgb.Spec.TargetGroupARN == "" {
		if tgb.Spec.TargetGroupName == "" {
			absentRequiredFields = append(absentRequiredFields, "either TargetGroupARN or TargetGroupName")
		} else if tgb.Spec.TargetGroupName != "" {
			/*
				The purpose of this code is to guarantee that the either the ARN of the TargetGroup exists
				or it's possible to infer the ARN by the name of the TargetGroup (since it's unique).

				And even though the validator can't mutate, I added tgb.Spec.TargetGroupARN = *tgObj.TargetGroupArn
				to guarantee the object is in a consistent state though the rest of the process.

				The whole code of aws-load-balancer-controller was written assuming there is an ARN.
				By changing the object here I guarantee as early as possible that that assumption is true.
			*/

			tgObj, err := getTargetGroupsByNameFromAWS(ctx, elbv2ForTGB, tgb)
			if err != nil {
				return fmt.Errorf("searching TargetGroup with name %s: %w", tgb.Spec.TargetGroupName, err)
			}
			tgb.Spec.TargetGroupARN = *tgObj.TargetGroupArn
		}
	}
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
	if (tgb.Spec.VpcID != "" && oldTGB.Spec.VpcID != "" && (tgb.Spec.VpcID) != (oldTGB.Spec.VpcID)) ||
		(oldTGB.Spec.VpcID != "" && tgb.Spec.VpcID == "") ||
		(oldTGB.Spec.VpcID == "" && tgb.Spec.VpcID != "" && tgb.Spec.VpcID != v.vpcID) {
		changedImmutableFields = append(changedImmutableFields, "spec.vpcID")
	}
	if len(changedImmutableFields) != 0 {
		return errors.Errorf("%s update may not change these immutable fields: %s", "TargetGroupBinding", strings.Join(changedImmutableFields, ","))
	}
	return nil
}

// checkExistingTargetGroups will check for unique TargetGroup per TargetGroupBinding
func (v *targetGroupBindingValidator) checkExistingTargetGroups(updatedTgb *elbv2api.TargetGroupBinding) error {
	ctx := context.Background()
	tgbList := elbv2api.TargetGroupBindingList{}
	if err := v.k8sClient.List(ctx, &tgbList); err != nil {
		return errors.Wrap(err, "failed to list TargetGroupBindings in the cluster")
	}

	duplicateTGBs := make([]types.NamespacedName, 0)
	multiClusterSupported := updatedTgb.Spec.MultiClusterTargetGroup

	for _, tgbObj := range tgbList.Items {
		if tgbObj.UID != updatedTgb.UID && tgbObj.Spec.TargetGroupARN == updatedTgb.Spec.TargetGroupARN {
			if !tgbObj.Spec.MultiClusterTargetGroup {
				multiClusterSupported = false
			}
			duplicateTGBs = append(duplicateTGBs, k8s.NamespacedName(&tgbObj))
		}
	}

	if len(duplicateTGBs) != 0 && !multiClusterSupported {
		return errors.Errorf("TargetGroup %v is already bound to following TargetGroupBindings %v. Please enable MultiCluster mode on all TargetGroupBindings referencing %v or choose a different Target Group ARN.", updatedTgb.Spec.TargetGroupARN, duplicateTGBs, updatedTgb.Spec.TargetGroupARN)
	}

	return nil
}

// checkNodeSelector ensures that NodeSelector is only set when TargetType is ip
func (v *targetGroupBindingValidator) checkNodeSelector(tgb *elbv2api.TargetGroupBinding) error {
	if (*tgb.Spec.TargetType == elbv2api.TargetTypeIP) && (tgb.Spec.NodeSelector != nil) {
		return errors.Errorf("TargetGroupBinding cannot set NodeSelector when TargetType is ip")
	}
	return nil
}

// checkTargetGroupIPAddressType ensures IP address type matches with that on the AWS target group
func (v *targetGroupBindingValidator) checkTargetGroupIPAddressType(tgb *elbv2api.TargetGroupBinding, tgCache func() tgCacheObject) error {
	targetGroupIPAddressType, err := v.getTargetGroupIPAddressTypeFromAWS(tgCache)
	if err != nil {
		return errors.Wrap(err, "unable to get target group IP address type")
	}
	if (tgb.Spec.IPAddressType != nil && *tgb.Spec.IPAddressType != targetGroupIPAddressType) ||
		(tgb.Spec.IPAddressType == nil && targetGroupIPAddressType != elbv2api.TargetGroupIPAddressTypeIPv4) {
		return errors.Errorf("invalid IP address type %v for TargetGroup %v", targetGroupIPAddressType, tgb.Spec.TargetGroupARN)
	}
	return nil
}

// checkTargetGroupVpcID ensures VpcID matches with that on the AWS target group
func (v *targetGroupBindingValidator) checkTargetGroupVpcID(tgb *elbv2api.TargetGroupBinding, tgCache func() tgCacheObject) error {
	if tgb.Spec.VpcID == "" {
		return nil
	}
	if !vpcIDPatternRegex.MatchString(tgb.Spec.VpcID) {
		return errors.Errorf(vpcIDValidationErr, tgb.Spec.VpcID)
	}
	vpcID, err := v.getVpcIDFromAWS(tgCache)
	if err != nil {
		return errors.Wrap(err, "unable to get target group VpcID")
	}
	if vpcID != tgb.Spec.VpcID {
		return errors.Errorf(vpcIDNotMatchErr, tgb.Spec.VpcID, tgb.Spec.TargetGroupARN)
	}
	return nil
}

// getTargetGroupIPAddressTypeFromAWS returns the target group IP address type of AWS target group
func (v *targetGroupBindingValidator) getTargetGroupIPAddressTypeFromAWS(tgCache func() tgCacheObject) (elbv2api.TargetGroupIPAddressType, error) {

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

func (v *targetGroupBindingValidator) getVpcIDFromAWS(tgCache func() tgCacheObject) (string, error) {
	obj := tgCache()
	targetGroup := obj.tg
	err := obj.error
	if err != nil {
		return "", err
	}
	return awssdk.ToString(targetGroup.VpcId), nil
}

// checkAssumeRoleConfig various checks for using cross account target group bindings.
func (v *targetGroupBindingValidator) checkAssumeRoleConfig(tgb *elbv2api.TargetGroupBinding) error {
	if tgb.Spec.IamRoleArnToAssume == "" {
		return nil
	}

	if tgb.Spec.TargetType != nil && *tgb.Spec.TargetType == elbv2api.TargetTypeInstance {
		return errors.New("Unable to use instance target type while using assume role")
	}

	return nil
}

// checkTargetGroupVpcID ensures Target Group Protocol matches with that on the AWS target group
func (v *targetGroupBindingValidator) checkTargetGroupProtocol(tgb *elbv2api.TargetGroupBinding, tgCache func() tgCacheObject) error {
	// Backwards compatibility -- nil is acceptable for legacy tgb
	if tgb.Spec.TargetGroupProtocol == nil {
		return nil
	}
	tgProtocol, err := obtainSDKTargetGroupProtocolFromAWS(tgCache)
	if err != nil {
		return errors.Wrap(err, "unable to get target group VpcID")
	}
	if tgProtocol != string(*tgb.Spec.TargetGroupProtocol) {
		return errors.Errorf(tgProtocolMismatch, tgb.Spec.TargetGroupARN, *tgb.Spec.TargetGroupProtocol, tgProtocol)
	}

	if (tgProtocol == string(elbv2.ProtocolQUIC) || tgProtocol == string(elbv2.ProtocolTCP_QUIC)) && (tgb.Spec.TargetType != nil && *tgb.Spec.TargetType == elbv2api.TargetTypeInstance) {
		return errors.Errorf(quicInstanceNotSupported)
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-elbv2-k8s-aws-v1beta1-targetgroupbinding,mutating=false,failurePolicy=fail,groups=elbv2.k8s.aws,resources=targetgroupbindings,verbs=create;update,versions=v1beta1,name=vtargetgroupbinding.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (v *targetGroupBindingValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateELBv2TargetGroupBinding, webhook.ValidatingWebhookForValidator(v, mgr.GetScheme()))
}
