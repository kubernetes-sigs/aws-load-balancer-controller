package webhook

import (
	"context"
	"encoding/json"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"net/http"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"testing"
)

func Test_mutatingHandler_InjectDecoder(t *testing.T) {
	h := mutatingHandler{
		decoder: nil,
	}
	decoder := &admission.Decoder{}
	h.InjectDecoder(decoder)

	assert.Equal(t, decoder, h.decoder)
}

func Test_mutatingHandler_Handle(t *testing.T) {
	schema := runtime.NewScheme()
	clientgoscheme.AddToScheme(schema)
	// k8sDecoder knows k8s objects
	decoder, _ := admission.NewDecoder(schema)
	patchTypeJSONPatch := admissionv1.PatchTypeJSONPatch

	initialPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
			Annotations: map[string]string{
				"some-key": "some-value",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "bar",
					Image: "bar:v1",
				},
			},
		},
		Status: corev1.PodStatus{},
	}
	initialPodRaw, err := json.Marshal(initialPod)
	assert.NoError(t, err)
	updatedPod := initialPod.DeepCopy()
	updatedPod.Spec.Containers[0].Image = "bar:v2"
	updatedPodRaw, err := json.Marshal(updatedPod)
	assert.NoError(t, err)

	type fields struct {
		mutatorPrototype    func(req admission.Request) (runtime.Object, error)
		mutatorMutateCreate func(ctx context.Context, obj runtime.Object) (runtime.Object, error)
		mutatorMutateUpdate func(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error)
		decoder             *admission.Decoder
	}
	type args struct {
		req admission.Request
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   admission.Response
	}{
		{
			name: "[create] approve request and mutates nothing",
			fields: fields{
				mutatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				mutatorMutateCreate: func(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
					return obj, nil
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: initialPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: []jsonpatch.JsonPatchOperation{},
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed:   true,
					PatchType: nil,
				},
			},
		},
		{
			name: "[create] approve request and mutates object annotations",
			fields: fields{
				mutatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				mutatorMutateCreate: func(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
					pod := obj.(*corev1.Pod)
					pod.Annotations = algorithm.MergeStringMap(
						pod.Annotations,
						map[string]string{"appmesh.k8s.aws/virtualNode": "awesome-node"},
					)
					return pod, nil
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: initialPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: []jsonpatch.JsonPatchOperation{
					{
						Operation: "add",
						Path:      "/metadata/annotations/appmesh.k8s.aws~1virtualNode",
						Value:     "awesome-node",
					},
				},
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed:   true,
					PatchType: &patchTypeJSONPatch,
				},
			},
		},
		{
			name: "[create] reject request",
			fields: fields{
				mutatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				mutatorMutateCreate: func(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
					return nil, errors.New("oops, some error happened")
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: initialPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: nil,
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Code:   http.StatusForbidden,
						Reason: "oops, some error happened",
					},
				},
			},
		},
		{
			name: "[create] unexpected object type - prototype returns error ",
			fields: fields{
				mutatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return nil, errors.New("oops, unexpected object type")
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: initialPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: nil,
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Code:    http.StatusBadRequest,
						Message: "oops, unexpected object type",
					},
				},
			},
		},
		{
			name: "[update] approve request and mutates nothing",
			fields: fields{
				mutatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				mutatorMutateUpdate: func(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
					return obj, nil
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Update,
						Object: runtime.RawExtension{
							Raw: updatedPodRaw,
						},
						OldObject: runtime.RawExtension{
							Raw: initialPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: []jsonpatch.JsonPatchOperation{},
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed:   true,
					PatchType: nil,
				},
			},
		},
		{
			name: "[update] approve request and mutates object annotations",
			fields: fields{
				mutatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				mutatorMutateUpdate: func(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
					pod := obj.(*corev1.Pod)
					pod.Annotations = algorithm.MergeStringMap(
						pod.Annotations,
						map[string]string{"appmesh.k8s.aws/virtualNode": "awesome-node"},
					)
					return pod, nil
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Update,
						Object: runtime.RawExtension{
							Raw: updatedPodRaw,
						},
						OldObject: runtime.RawExtension{
							Raw: initialPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: []jsonpatch.JsonPatchOperation{
					{
						Operation: "add",
						Path:      "/metadata/annotations/appmesh.k8s.aws~1virtualNode",
						Value:     "awesome-node",
					},
				},
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed:   true,
					PatchType: &patchTypeJSONPatch,
				},
			},
		},
		{
			name: "[update] reject request",
			fields: fields{
				mutatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				mutatorMutateUpdate: func(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
					return nil, errors.New("oops, some error happened")
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Update,
						Object: runtime.RawExtension{
							Raw: updatedPodRaw,
						},
						OldObject: runtime.RawExtension{
							Raw: initialPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: nil,
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Code:   http.StatusForbidden,
						Reason: "oops, some error happened",
					},
				},
			},
		},
		{
			name: "[update] unexpected object type - prototype returns error ",
			fields: fields{
				mutatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return nil, errors.New("oops, unexpected object type")
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Update,
						Object: runtime.RawExtension{
							Raw: updatedPodRaw,
						},
						OldObject: runtime.RawExtension{
							Raw: initialPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: nil,
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Code:    http.StatusBadRequest,
						Message: "oops, unexpected object type",
					},
				},
			},
		},
		{
			name: "[connect] methods other than create/update will pass through",
			fields: fields{
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Connect,
						Object: runtime.RawExtension{
							Raw: updatedPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				Patches: nil,
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: true,
					Result: &metav1.Status{
						Code: http.StatusOK,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mutator := NewMockMutator(ctrl)
			if tt.fields.mutatorPrototype != nil {
				mutator.EXPECT().Prototype(gomock.Any()).DoAndReturn(tt.fields.mutatorPrototype)
			}
			if tt.fields.mutatorMutateCreate != nil {
				mutator.EXPECT().MutateCreate(gomock.Any(), gomock.Any()).DoAndReturn(tt.fields.mutatorMutateCreate)
			}
			if tt.fields.mutatorMutateUpdate != nil {
				mutator.EXPECT().MutateUpdate(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(tt.fields.mutatorMutateUpdate)
			}

			h := &mutatingHandler{
				mutator: mutator,
				decoder: tt.fields.decoder,
			}
			got := h.Handle(ctx, tt.args.req)
			assert.Equal(t, tt.want, got)
		})
	}
}
