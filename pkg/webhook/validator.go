package webhook

import (
	"context"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Validator defines interface for a validation webHook
type Validator interface {
	// Prototype returns a prototype of Object for this admission request.
	Prototype(req admission.Request) (runtime.Object, error)

	// ValidateCreate handles Object creation and returns error if any.
	ValidateCreate(ctx context.Context, obj runtime.Object) error
	// ValidateUpdate handles Object update and returns error if any.
	ValidateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error
	// ValidateDelete handles Object deletion and returns error if any.
	ValidateDelete(ctx context.Context, obj runtime.Object) error
}

// ValidatingWebhookForValidator creates a new validating Webhook.
func ValidatingWebhookForValidator(validator Validator) *admission.Webhook {
	return &admission.Webhook{
		Handler: &validatingHandler{validator: validator},
	}
}
