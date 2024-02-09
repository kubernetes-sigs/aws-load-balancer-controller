package webhook

import (
	"context"
	admissionv1 "k8s.io/api/admission/v1"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var validatingHandlerLog = ctrl.Log.WithName("validating_handler")
var _ admission.DecoderInjector = &validatingHandler{}
var _ admission.Handler = &validatingHandler{}

type validatingHandler struct {
	validator Validator
	decoder   *admission.Decoder
}

// InjectDecoder injects the decoder into a mutatingHandler.
func (h *validatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.decoder = d
	return nil
}

// Handle handles admission requests.
func (h *validatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	validatingHandlerLog.V(1).Info("validating webhook request", "request", req)
	var resp admission.Response
	switch req.Operation {
	case admissionv1.Create:
		resp = h.handleCreate(ctx, req)
	case admissionv1.Update:
		resp = h.handleUpdate(ctx, req)
	case admissionv1.Delete:
		resp = h.handleDelete(ctx, req)
	default:
		resp = admission.Allowed("")
	}
	validatingHandlerLog.V(1).Info("validating webhook response", "response", resp)
	return resp
}

func (h *validatingHandler) handleCreate(ctx context.Context, req admission.Request) admission.Response {
	prototype, err := h.validator.Prototype(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	obj := prototype.DeepCopyObject()
	if err := h.decoder.DecodeRaw(req.Object, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := h.validator.ValidateCreate(ContextWithAdmissionRequest(ctx, req), obj); err != nil {
		return admission.Denied(err.Error())
	}
	return admission.Allowed("")
}

func (h *validatingHandler) handleUpdate(ctx context.Context, req admission.Request) admission.Response {
	prototype, err := h.validator.Prototype(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	obj := prototype.DeepCopyObject()
	oldObj := prototype.DeepCopyObject()
	if err := h.decoder.DecodeRaw(req.Object, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := h.decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := h.validator.ValidateUpdate(ContextWithAdmissionRequest(ctx, req), obj, oldObj); err != nil {
		return admission.Denied(err.Error())
	}
	return admission.Allowed("")
}

func (h *validatingHandler) handleDelete(ctx context.Context, req admission.Request) admission.Response {
	prototype, err := h.validator.Prototype(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	obj := prototype.DeepCopyObject()
	if err := h.decoder.DecodeRaw(req.OldObject, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := h.validator.ValidateDelete(ContextWithAdmissionRequest(ctx, req), obj); err != nil {
		return admission.Denied(err.Error())
	}
	return admission.Allowed("")
}
