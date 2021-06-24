package inject

import (
	"context"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"testing"
)

func Test_PodReadinessGate_Mutate(t *testing.T) {
	testNS1 := "name-space-1"
	testNS2 := "name-space-2"
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS1,
			Name:      "service-1",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "app-1",
				"svc": "svc1",
			},
		},
	}
	svc2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS1,
			Name:      "service-noselector",
		},
	}
	svc3 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS2,
			Name:      "service-1",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "app-1",
				"svc": "svc1",
			},
		},
	}

	targetTypeIP := elbv2api.TargetTypeIP
	targetTypeInstance := elbv2api.TargetTypeInstance
	tgb1 := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tgb-1-l6qw1",
			Namespace: testNS1,
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetType: &targetTypeIP,
			ServiceRef: elbv2api.ServiceReference{
				Name: svc1.Name,
			},
		},
	}
	tgb2 := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tgb-2-l6qw2",
			Namespace: testNS1,
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetType: &targetTypeIP,
			ServiceRef: elbv2api.ServiceReference{
				Name: svc1.Name,
			},
		},
	}
	tgb3 := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tgb-3-l6qw3",
			Namespace: testNS1,
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetType: &targetTypeIP,
			ServiceRef: elbv2api.ServiceReference{
				Name: "service-nonexistent",
			},
		},
	}
	tgb4 := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tgb-4-l6qw4",
			Namespace: testNS1,
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetType: &targetTypeIP,
			ServiceRef: elbv2api.ServiceReference{
				Name: svc2.Name,
			},
		},
	}
	tgb5 := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tgb-5-l6qw5",
			Namespace: testNS1,
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			TargetType: &targetTypeInstance,
			ServiceRef: elbv2api.ServiceReference{
				Name: svc1.Name,
			},
		},
	}

	tests := []struct {
		name      string
		namespace string
		services  []*corev1.Service
		tgbList   []*elbv2api.TargetGroupBinding
		pod       *corev1.Pod
		want      []corev1.PodReadinessGate
		config    Config
		wantError bool
	}{
		{
			name:      "matching tgb with ip targetType",
			namespace: testNS1,
			services:  []*corev1.Service{svc1},
			tgbList:   []*elbv2api.TargetGroupBinding{tgb1},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "app-1",
						"svc":    "svc1",
						"stable": "none",
					},
				},
			},
			want: []corev1.PodReadinessGate{
				{
					ConditionType: "target-health.elbv2.k8s.aws/tgb-1-l6qw1",
				},
			},
			config: Config{
				EnablePodReadinessGateInject: true,
			},
		},
		{
			name:      "matching tgb but with instance targetType",
			namespace: testNS1,
			services:  []*corev1.Service{svc1},
			tgbList:   []*elbv2api.TargetGroupBinding{tgb5},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "app-1",
						"svc":    "svc1",
						"stable": "none",
					},
				},
			},
			want: nil,
			config: Config{
				EnablePodReadinessGateInject: true,
			},
		},
		{
			name:      "multiple tgb with ip targetType",
			namespace: testNS1,
			services:  []*corev1.Service{svc1},
			tgbList:   []*elbv2api.TargetGroupBinding{tgb1, tgb2},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":    "app-1",
						"svc":    "svc1",
						"stable": "none",
					},
				},
			},
			want: []corev1.PodReadinessGate{
				{
					ConditionType: "target-health.elbv2.k8s.aws/tgb-1-l6qw1",
				},
				{
					ConditionType: "target-health.elbv2.k8s.aws/tgb-2-l6qw2",
				},
			},
			config: Config{
				EnablePodReadinessGateInject: true,
			},
		},
		{
			name:      "nonexistent service",
			namespace: testNS1,
			services:  []*corev1.Service{svc1, svc2, svc3},
			tgbList:   []*elbv2api.TargetGroupBinding{tgb3},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "app-nomatch",
					},
				},
			},
			want: []corev1.PodReadinessGate(nil),
			config: Config{
				EnablePodReadinessGateInject: true,
			},
		},
		{
			name:      "service without selector",
			namespace: testNS1,
			services:  []*corev1.Service{svc1, svc2, svc3},
			tgbList:   []*elbv2api.TargetGroupBinding{tgb4},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "app-nomatch",
					},
				},
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "leave-unmodified",
						},
					},
				},
			},
			want: []corev1.PodReadinessGate{
				{
					ConditionType: "leave-unmodified",
				},
			},
			config: Config{
				EnablePodReadinessGateInject: true,
			},
		},
		{
			name:      "pod label doesn't match",
			namespace: testNS1,
			services:  []*corev1.Service{svc1, svc2, svc3},
			tgbList:   []*elbv2api.TargetGroupBinding{tgb1, tgb2, tgb3, tgb4},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "app-nomatch",
					},
				},
			},
			want: []corev1.PodReadinessGate(nil),
			config: Config{
				EnablePodReadinessGateInject: true,
			},
		},
		{
			name:      "remove related old readiness gates",
			namespace: testNS1,
			services:  []*corev1.Service{svc1, svc2, svc3},
			tgbList:   []*elbv2api.TargetGroupBinding{tgb1, tgb2, tgb3, tgb4},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "app-1",
						"svc": "svc1",
						"new": "label",
					},
				},
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.alb.ingress.k8s.aws/ingress_svc_port",
						},
						{
							ConditionType: "target-health.alb.ingress.k8s.aws/load-balancer-any-tg-ready",
						},
						{
							ConditionType: "leave-intact",
						},
						{
							ConditionType: "target-health.elbv2.k8s.aws/tgb-x-non-exists",
						},
						{
							ConditionType: "target-health.elbv2.k8s.aws/tgb-1-l6qw1",
						},
					},
				},
			},
			want: []corev1.PodReadinessGate{
				{
					ConditionType: "leave-intact",
				},
				{
					ConditionType: "target-health.elbv2.k8s.aws/tgb-x-non-exists",
				},
				{
					ConditionType: "target-health.elbv2.k8s.aws/tgb-1-l6qw1",
				},
				{
					ConditionType: "target-health.elbv2.k8s.aws/tgb-2-l6qw2",
				},
			},
			config: Config{
				EnablePodReadinessGateInject: true,
			},
		},
		{
			name:      "inject disabled",
			namespace: testNS1,
			services:  []*corev1.Service{svc1, svc2, svc3},
			tgbList:   []*elbv2api.TargetGroupBinding{tgb1, tgb2, tgb3, tgb4},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "app-1",
						"svc": "svc1",
						"new": "label",
					},
				},
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.alb.ingress.k8s.aws/old_gate",
						},
						{
							ConditionType: "leave-intact",
						},
					},
				},
			},
			want: []corev1.PodReadinessGate{
				{
					ConditionType: "target-health.alb.ingress.k8s.aws/old_gate",
				},
				{
					ConditionType: "leave-intact",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, svc := range tt.services {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}
			for _, tgb := range tt.tgbList {
				assert.NoError(t, k8sClient.Create(ctx, tgb.DeepCopy()))
			}
			ctx = webhook.ContextWithAdmissionRequest(ctx, admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{Namespace: tt.namespace},
			})
			readinessGateInjector := NewPodReadinessGate(tt.config, k8sClient, &log.NullLogger{})
			err := readinessGateInjector.Mutate(ctx, tt.pod)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, tt.pod.Spec.ReadinessGates)
			}
		})
	}
}

func Test_PodReadinessGate_removeLegacyTargetHealthReadinessGates(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want *corev1.Pod
	}{
		{
			name: "no readiness gates",
			pod:  &corev1.Pod{},
			want: &corev1.Pod{},
		},
		{
			name: "unrelated readiness gates",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "unrelated",
						},
						{
							ConditionType: "test-gate",
						},
					},
				},
			},
			want: &corev1.Pod{
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "unrelated",
						},
						{
							ConditionType: "test-gate",
						},
					},
				},
			},
		},
		{
			name: "mix and match",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "unrelated",
						},
						{
							ConditionType: "target-health.alb.ingress.k8s.aws/old_gate",
						},
						{
							ConditionType: "target-health.elbv2.k8s.aws/tgb-0851b1f4d6",
						},
						{
							ConditionType: "survive",
						},
					},
				},
			},
			want: &corev1.Pod{
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "unrelated",
						},
						{
							ConditionType: "target-health.elbv2.k8s.aws/tgb-0851b1f4d6",
						},
						{
							ConditionType: "survive",
						},
					},
				},
			},
		},
		{
			name: "empty out",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					ReadinessGates: []corev1.PodReadinessGate{
						{
							ConditionType: "target-health.alb.ingress.k8s.aws/old_gate",
						},
					},
				},
			},
			want: &corev1.Pod{
				Spec: corev1.PodSpec{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readinessGateInjector := &PodReadinessGate{}
			readinessGateInjector.removeLegacyTargetHealthReadinessGates(context.Background(), tt.pod)
			assert.Equal(t, tt.want, tt.pod)
		})
	}
}
