package webhook

import (
	"context"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"testing"
)

func TestContextGetAdmissionRequestAndContextWithAdmissionRequest(t *testing.T) {
	type args struct {
		req *admission.Request
	}
	tests := []struct {
		name string
		args args
		want *admission.Request
	}{
		{
			name: "with request",
			args: args{
				req: &admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UID: "1",
					},
				},
			},
			want: &admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: "1",
				},
			},
		},
		{
			name: "without request",
			args: args{
				req: nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.args.req != nil {
				ctx = ContextWithAdmissionRequest(ctx, *tt.args.req)
			}
			got := ContextGetAdmissionRequest(ctx)
			assert.Equal(t, tt.want, got)
		})
	}
}
