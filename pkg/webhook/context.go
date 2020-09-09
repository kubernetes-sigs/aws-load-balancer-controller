package webhook

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type contextKey string

const (
	contextKeyAdmissionRequest contextKey = "admissionRequest"
)

func ContextGetAdmissionRequest(ctx context.Context) *admission.Request {
	if v := ctx.Value(contextKeyAdmissionRequest); v != nil {
		return v.(*admission.Request)
	}
	return nil
}

func ContextWithAdmissionRequest(ctx context.Context, req admission.Request) context.Context {
	return context.WithValue(ctx, contextKeyAdmissionRequest, &req)
}
