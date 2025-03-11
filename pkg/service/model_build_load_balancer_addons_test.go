package service

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
)

func Test_defaultModelBuildTask_buildShieldProtection(t *testing.T) {
	type args struct {
		lbARN core.StringToken
	}
	tests := []struct {
		testName  string
		svc       *corev1.Service
		args      args
		want      *shieldmodel.Protection
		wantError bool
	}{
		{
			testName: "when shield-advanced-protection annotation is not specified",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want:      nil,
			wantError: false,
		},
		{
			testName: "when shield-advanced-protection annotation set to true",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-nlb-shield-advanced-protection": "true",
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &shieldmodel.Protection{
				Spec: shieldmodel.ProtectionSpec{
					Enabled:     true,
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantError: false,
		},
		{
			testName: "when shield-advanced-protection annotation set to false",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-nlb-shield-advanced-protection": "false",
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &shieldmodel.Protection{
				Spec: shieldmodel.ProtectionSpec{
					Enabled:     false,
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantError: false,
		},
		{
			testName: "when shield-advanced-protection annotation has non boolean value",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-nlb-shield-advanced-protection": "FalSe1",
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			task := &defaultModelBuildTask{
				service:          tt.svc,
				annotationParser: annotationParser,
				stack:            stack,
			}
			got, err := task.buildShieldProtection(context.Background(), tt.args.lbARN)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				opts := cmpopts.IgnoreTypes(core.ResourceMeta{})
				assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
			}
		})
	}
}
