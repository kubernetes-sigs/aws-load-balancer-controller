package webhook

import (
	"context"
	"encoding/json"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"testing"
)

func Test_validatingHandler_InjectDecoder(t *testing.T) {
	h := validatingHandler{
		decoder: nil,
	}
	decoder := &admission.Decoder{}
	h.InjectDecoder(decoder)

	assert.Equal(t, decoder, h.decoder)
}

func Test_validatingHandler_Handle(t *testing.T) {
	schema := runtime.NewScheme()
	clientgoscheme.AddToScheme(schema)
	// k8sDecoder knows k8s objects
	decoder, _ := admission.NewDecoder(schema)

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
		validatorPrototype      func(req admission.Request) (runtime.Object, error)
		validatorValidateCreate func(ctx context.Context, obj runtime.Object) error
		validatorValidateUpdate func(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error
		validatorValidateDelete func(ctx context.Context, obj runtime.Object) error
		decoder                 *admission.Decoder
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
			name: "[create] approve request",
			fields: fields{
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				validatorValidateCreate: func(ctx context.Context, obj runtime.Object) error {
					return nil
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
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: true,
					Result: &metav1.Status{
						Code: http.StatusOK,
					},
				},
			},
		},
		{
			name: "[create] reject request",
			fields: fields{
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				validatorValidateCreate: func(ctx context.Context, obj runtime.Object) error {
					return errors.New("oops, some error happened")
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
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
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
			name: "[update] approve request",
			fields: fields{
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				validatorValidateUpdate: func(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
					return nil
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
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: true,
					Result: &metav1.Status{
						Code: http.StatusOK,
					},
				},
			},
		},
		{
			name: "[update] reject request",
			fields: fields{
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				validatorValidateUpdate: func(ctx context.Context, obj runtime.Object, oldObj runtime.Object) error {
					return errors.New("oops, some error happened")
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
			name: "[update] unexpected object type - prototype returns error",
			fields: fields{
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
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
			name: "[delete] approve request",
			fields: fields{
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				validatorValidateDelete: func(ctx context.Context, obj runtime.Object) error {
					return nil
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Delete,
						OldObject: runtime.RawExtension{
							Raw: updatedPodRaw,
						},
					},
				},
			},
			want: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: true,
					Result: &metav1.Status{
						Code: http.StatusOK,
					},
				},
			},
		},
		{
			name: "[delete] reject request",
			fields: fields{
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return &corev1.Pod{}, nil
				},
				validatorValidateDelete: func(ctx context.Context, obj runtime.Object) error {
					return errors.New("oops, some error happened")
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Delete,
						OldObject: runtime.RawExtension{
							Raw: updatedPodRaw,
						},
					},
				},
			},
			want: admission.Response{
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
			name: "[delete]  unexpected object type - prototype returns error",
			fields: fields{
				validatorPrototype: func(req admission.Request) (runtime.Object, error) {
					return nil, errors.New("oops, unexpected object type")
				},
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Delete,
						OldObject: runtime.RawExtension{
							Raw: updatedPodRaw,
						},
					},
				},
			},
			want: admission.Response{
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
			name: "[connect] methods other than create/update/delete will pass through",
			fields: fields{
				decoder: decoder,
			},
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Connect,
						Object: runtime.RawExtension{
							Raw: initialPodRaw,
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
			validator := NewMockValidator(ctrl)
			if tt.fields.validatorPrototype != nil {
				validator.EXPECT().Prototype(gomock.Any()).DoAndReturn(tt.fields.validatorPrototype)
			}
			if tt.fields.validatorValidateCreate != nil {
				validator.EXPECT().ValidateCreate(gomock.Any(), gomock.Any()).DoAndReturn(tt.fields.validatorValidateCreate)
			}
			if tt.fields.validatorValidateUpdate != nil {
				validator.EXPECT().ValidateUpdate(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(tt.fields.validatorValidateUpdate)
			}
			if tt.fields.validatorValidateDelete != nil {
				validator.EXPECT().ValidateDelete(gomock.Any(), gomock.Any()).DoAndReturn(tt.fields.validatorValidateDelete)
			}

			h := &validatingHandler{
				validator: validator,
				decoder:   tt.fields.decoder,
			}
			got := h.Handle(ctx, tt.args.req)
			assert.Equal(t, tt.want, got)
		})
	}
}
