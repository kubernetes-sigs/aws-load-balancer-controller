package shared_utils

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// ValidateCrossNamespaceReference checks if a ReferenceGrant allows the reference
func ValidateCrossNamespaceReference(ctx context.Context, k8sClient client.Client,
	fromNamespace, fromGroup, fromKind,
	toGroup, toKind, toNamespace, toName string) (bool, error) {

	// List ReferenceGrants in the target namespace
	refGrantList := &gwv1beta1.ReferenceGrantList{}
	if err := k8sClient.List(ctx, refGrantList, client.InNamespace(toNamespace)); err != nil {
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
			// If the CRD is not installed, handle it gracefully instead of returning an error
			return false, nil
		}
		return false, fmt.Errorf("failed to check reference grants: %w", err)
	}

	// Check each grant
	for _, grant := range refGrantList.Items {
		// Check if any grant allows this reference
		if grantAllowsReference(grant, fromNamespace, fromGroup, fromKind, toGroup, toKind, toName) {
			return true, nil // Reference is allowed
		}
	}

	// No matching grant found
	return false, nil
}

// grantAllowsReference checks if a specific ReferenceGrant allows the reference
func grantAllowsReference(grant gwv1beta1.ReferenceGrant,
	fromNamespace string, fromGroup, fromKind, toGroup, toKind, toName string) bool {

	// Check From section
	fromMatched := false
	for _, from := range grant.Spec.From {
		if string(from.Group) == fromGroup && string(from.Kind) == fromKind && string(from.Namespace) == fromNamespace {
			fromMatched = true
			break
		}
	}

	if !fromMatched {
		return false
	}

	// Check To section
	for _, to := range grant.Spec.To {
		if string(to.Group) != toGroup || string(to.Kind) != toKind {
			continue
		}

		// If the To section has a specific name, it must match
		// If no name is specified, it acts as a wildcard
		if to.Name != nil && string(*to.Name) != toName {
			continue
		}

		return true
	}

	return false
}
