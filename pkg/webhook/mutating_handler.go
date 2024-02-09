package webhook

import (
	"context"
	"encoding/json"
	admissionv1 "k8s.io/api/admission/v1"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var mutatingHandlerLog = ctrl.Log.WithName("mutating_handler")
var _ admission.DecoderInjector = &mutatingHandler{}
var _ admission.Handler = &mutatingHandler{}

type mutatingHandler struct {
	mutator Mutator
	decoder *admission.Decoder
}

// InjectDecoder injects the decoder into a mutatingHandler.
func (h *mutatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.decoder = d
	return nil
}

// Handle handles admission requests.
func (h *mutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	mutatingHandlerLog.V(1).Info("mutating webhook request", "request", req)
	var resp admission.Response
	switch req.Operation {
	case admissionv1.Create:
		resp = h.handleCreate(ctx, req)
	case admissionv1.Update:
		resp = h.handleUpdate(ctx, req)
	default:
		resp = admission.Allowed("")
	}
	mutatingHandlerLog.V(1).Info("mutating webhook response", "response", resp)
	return resp
}

func (h *mutatingHandler) handleCreate(ctx context.Context, req admission.Request) admission.Response {
	prototype, err := h.mutator.Prototype(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	obj := prototype.DeepCopyObject()
	if err := h.decoder.DecodeRaw(req.Object, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	mutatedObj, err := h.mutator.MutateCreate(ContextWithAdmissionRequest(ctx, req), obj)
	if err != nil {
		return admission.Denied(err.Error())
	}
	mutatedObjPayload, err := json.Marshal(mutatedObj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, mutatedObjPayload)
}

func (h *mutatingHandler) handleUpdate(ctx context.Context, req admission.Request) admission.Response {
	prototype, err := h.mutator.Prototype(req)
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

	mutatedObj, err := h.mutator.MutateUpdate(ContextWithAdmissionRequest(ctx, req), obj, oldObj)
	if err != nil {
		return admission.Denied(err.Error())
	}
	mutatedObjPayload, err := json.Marshal(mutatedObj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, mutatedObjPayload)
}
