package webhook

import (
	"context"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Mutator defines interface for a mutation webHook
type Mutator interface {
	// Prototype returns a prototype of Object for this admission request.
	Prototype(req admission.Request) (runtime.Object, error)

	// MutateCreate handles Object creation and returns the object after mutation and error if any.
	MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error)
	// MutateUpdate handles Object update and returns the object after mutation and error if any.
	MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error)
}

// MutatingWebhookForMutator creates a new mutating Webhook.
func MutatingWebhookForMutator(mutator Mutator) *admission.Webhook {
	return &admission.Webhook{
		Handler: &mutatingHandler{mutator: mutator},
	}
}
