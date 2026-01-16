package aga

import (
	"context"
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// Error messages
const (
	// UnsupportedResourceTypeMsg is the error message when an endpoint references an unsupported resource type
	UnsupportedResourceTypeMsg = "resource type %s is not supported for cross-namespace references"
)

// CrossNamespaceValidator validates cross-namespace references
type CrossNamespaceValidator interface {
	// ValidateCrossNamespaceReference checks if a resource in one namespace can be referenced from another
	ValidateCrossNamespaceReference(ctx context.Context, fromNamespace string,
		toGroup, toKind, toNamespace, toName string) error
}

// ReferenceGrantValidator implements CrossNamespaceValidator using Gateway API ReferenceGrants
type ReferenceGrantValidator struct {
	k8sClient client.Client
	logger    logr.Logger
}

// NewReferenceGrantValidator creates a new ReferenceGrantValidator
func NewReferenceGrantValidator(k8sClient client.Client, logger logr.Logger) *ReferenceGrantValidator {
	return &ReferenceGrantValidator{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ValidateCrossNamespaceReference checks if a ReferenceGrant allows the reference
func (v *ReferenceGrantValidator) ValidateCrossNamespaceReference(ctx context.Context,
	fromNamespace string,
	toGroup, toKind, toNamespace, toName string) error {

	// List ReferenceGrants in the target namespace
	refGrantList := &gwv1beta1.ReferenceGrantList{}
	if err := v.k8sClient.List(ctx, refGrantList, client.InNamespace(toNamespace)); err != nil {
		return fmt.Errorf("failed to check reference grants: %w", err)
	}

	// Check each grant
	for _, grant := range refGrantList.Items {
		// Check if any grant allows this reference
		if v.grantAllowsReference(grant, fromNamespace, toGroup, toKind, toName) {
			return nil // Reference is allowed
		}
	}

	// No matching grant found
	return fmt.Errorf("cross-namespace reference not allowed for reference %s/%s - no matching reference grant found",
		toNamespace, toName)
}

// grantAllowsReference checks if a specific ReferenceGrant allows the reference
func (v *ReferenceGrantValidator) grantAllowsReference(grant gwv1beta1.ReferenceGrant,
	fromNamespace string, toGroup, toKind, toName string) bool {

	// Check From section
	fromMatched := false
	for _, from := range grant.Spec.From {
		if string(from.Group) == shared_constants.GlobalAcceleratorResourcesGroup && string(from.Kind) == shared_constants.GlobalAcceleratorKind && string(from.Namespace) == fromNamespace {
			fromMatched = true
			break
		}
	}

	if !fromMatched {
		return false
	}

	// Check To section
	for _, to := range grant.Spec.To {
		if string(to.Group) == toGroup && string(to.Kind) == toKind {
			return true
		}
	}

	return false
}
