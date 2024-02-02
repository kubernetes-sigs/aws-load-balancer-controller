package elbv2

import (
	"context"
	"fmt"
	"net"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const apiPathValidateELBv2IngressClassParams = "/validate-elbv2-k8s-aws-v1beta1-ingressclassparams"

// NewIngressClassParamsValidator returns a validator for the IngressClassParams CRD.
func NewIngressClassParamsValidator() *ingressClassParamsValidator {
	return &ingressClassParamsValidator{}
}

var _ webhook.Validator = &ingressClassParamsValidator{}

type ingressClassParamsValidator struct {
}

func (v *ingressClassParamsValidator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &elbv2api.IngressClassParams{}, nil
}

func (v *ingressClassParamsValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	icp := obj.(*elbv2api.IngressClassParams)
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, v.checkInboundCIDRs(icp)...)
	allErrs = append(allErrs, v.checkSecurityGroupsSelectors(icp)...)
	allErrs = append(allErrs, v.checkSubnetSelectors(icp)...)

	return allErrs.ToAggregate()
}

func (v *ingressClassParamsValidator) ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
	icp := obj.(*elbv2api.IngressClassParams)
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, v.checkInboundCIDRs(icp)...)
	allErrs = append(allErrs, v.checkSecurityGroupsSelectors(icp)...)
	allErrs = append(allErrs, v.checkSubnetSelectors(icp)...)

	return allErrs.ToAggregate()
}

func (v *ingressClassParamsValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// checkInboundCIDRs will check for valid inboundCIDRs.
func (v *ingressClassParamsValidator) checkInboundCIDRs(icp *elbv2api.IngressClassParams) (allErrs field.ErrorList) {
	for idx, cidr := range icp.Spec.InboundCIDRs {
		fieldPath := field.NewPath("spec", "inboundCIDRs").Index(idx)
		allErrs = append(allErrs, validateCIDR(cidr, fieldPath)...)
	}

	return allErrs
}

// validateCIDR will check for a valid CIDR.
func validateCIDR(cidr string, fieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		detail := "Could not be parsed as a CIDR"
		if !strings.Contains(cidr, "/") {
			ip := net.ParseIP(cidr)
			if ip != nil {
				if ip.To4() != nil && !strings.Contains(cidr, ":") {
					detail += fmt.Sprintf(" (did you mean \"%s/32\")", cidr)
				} else {
					detail += fmt.Sprintf(" (did you mean \"%s/64\")", cidr)
				}
			}
		}
		allErrs = append(allErrs, field.Invalid(fieldPath, cidr, detail))
	} else if !ip.Equal(ipNet.IP) {
		maskSize, _ := ipNet.Mask.Size()
		detail := fmt.Sprintf("Network contains bits outside prefix (did you mean \"%s/%d\")", ipNet.IP, maskSize)
		allErrs = append(allErrs, field.Invalid(fieldPath, cidr, detail))
	}

	return allErrs
}

// checkSecurityGroupsSelectors will check for valid SubnetSelectors
func (v *ingressClassParamsValidator) checkSecurityGroupsSelectors(icp *elbv2api.IngressClassParams) (allErrs field.ErrorList) {
	if icp.Spec.SecurityGroups != nil {
		securityGroups := icp.Spec.SecurityGroups
		fieldPath := field.NewPath("spec", "securityGroups")
		if securityGroups.IDs == nil && !securityGroups.ManagedInbound && securityGroups.Tags == nil {
			allErrs = append(allErrs, field.Required(fieldPath, "must have `ids`, `managed`, or `tags`"))
			return allErrs
		}
		if securityGroups.IDs != nil {
			if len(icp.Spec.InboundCIDRs) > 0 {
				allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "inboundCIDRs"), "May not have both `inboundCIDRs` and `securityGroups.ids`"))
			}
			if securityGroups.ManagedInbound {
				allErrs = append(allErrs, field.Forbidden(fieldPath.Child("managedInbound"), "may not have both `ids` and `managedInbound` set"))
				return allErrs
			}
			if securityGroups.Tags != nil {
				allErrs = append(allErrs, field.Forbidden(fieldPath.Child("tags"), "may not have both `ids` and `tags` set"))
				return allErrs
			}
			fieldPath := fieldPath.Child("ids")
			seen := sets.New[elbv2api.SecurityGroupID]()
			for i, securityGroupID := range securityGroups.IDs {
				if seen.Has(securityGroupID) {
					allErrs = append(allErrs, field.Duplicate(fieldPath.Index(i), securityGroupID))
				}
				seen.Insert(securityGroupID)
			}
		} else if securityGroups.ManagedInbound {
			if securityGroups.Tags != nil {
				allErrs = append(allErrs, field.Forbidden(fieldPath.Child("tags"), "may not have both `managedInbound` and `tags` set"))
			}
		} else {
			fieldPath := fieldPath.Child("tags")
			if len(icp.Spec.InboundCIDRs) > 0 {
				allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "inboundCIDRs"), "May not have both `inboundCIDRs` and `securityGroups.tags`"))
			}
			if len(securityGroups.Tags) == 0 {
				allErrs = append(allErrs, field.Required(fieldPath, "must have at least one tag key"))
			}
			for tagKey, tagValues := range securityGroups.Tags {
				fieldPath := fieldPath.Key(tagKey)
				valueSeen := sets.New[string]()
				for i, value := range tagValues {
					if valueSeen.Has(value) {
						allErrs = append(allErrs, field.Duplicate(fieldPath.Index(i), value))
					}
					valueSeen.Insert(value)
				}
			}
		}
	}

	return allErrs
}

// checkSubnetSelectors will check for valid SubnetSelectors
func (v *ingressClassParamsValidator) checkSubnetSelectors(icp *elbv2api.IngressClassParams) (allErrs field.ErrorList) {
	if icp.Spec.Subnets != nil {
		subnets := icp.Spec.Subnets
		fieldPath := field.NewPath("spec", "subnets")
		if subnets.IDs == nil && subnets.Tags == nil {
			allErrs = append(allErrs, field.Required(fieldPath, "must have either `ids` or `tags`"))
			return allErrs
		}
		if subnets.IDs != nil {
			if subnets.Tags != nil {
				allErrs = append(allErrs, field.Forbidden(fieldPath.Child("tags"), "may not have both `ids` and `tags` set"))
				return allErrs
			}
			fieldPath := fieldPath.Child("ids")
			seen := sets.New[elbv2api.SubnetID]()
			for i, subnetID := range subnets.IDs {
				if seen.Has(subnetID) {
					allErrs = append(allErrs, field.Duplicate(fieldPath.Index(i), subnetID))
				}
				seen.Insert(subnetID)
			}
		} else {
			fieldPath := fieldPath.Child("tags")
			if len(subnets.Tags) == 0 {
				allErrs = append(allErrs, field.Required(fieldPath, "must have at least one tag key"))
			}
			for tagKey, tagValues := range subnets.Tags {
				fieldPath := fieldPath.Key(tagKey)
				valueSeen := sets.New[string]()
				for i, value := range tagValues {
					if valueSeen.Has(value) {
						allErrs = append(allErrs, field.Duplicate(fieldPath.Index(i), value))
					}
					valueSeen.Insert(value)
				}
			}
		}
	}

	return allErrs
}

// +kubebuilder:webhook:path=/validate-elbv2-k8s-aws-v1beta1-ingressclassparams,mutating=false,failurePolicy=fail,groups=elbv2.k8s.aws,resources=ingressclassparams,verbs=create;update,versions=v1beta1,name=vingressclassparams.elbv2.k8s.aws,sideEffects=None,webhookVersions=v1,admissionReviewVersions=v1beta1

func (v *ingressClassParamsValidator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathValidateELBv2IngressClassParams, webhook.ValidatingWebhookForValidator(v))
}
