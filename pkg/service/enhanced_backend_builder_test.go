package service

import (
	"context"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_defaultEnhancedBackendBuilder_Build(t *testing.T) {
	type env struct {
		svcs []*corev1.Service
	}
	type args struct {
		svc             *corev1.Service
		action          Action
		backendServices map[types.NamespacedName]*corev1.Service
	}
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "svc-1",
		},
	}
	portTCP := intstr.FromString("tcp")

	tests := []struct {
		name                string
		env                 env
		args                args
		want                EnhancedBackend
		wantBackendServices map[types.NamespacedName]*corev1.Service
		wantErr             error
	}{
		{
			name: "service backend",
			env:  env{svcs: []*corev1.Service{svc1}},
			args: args{
				svc: svc1,
				action: Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &portTCP,
							},
						},
					},
				},
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			for _, svc := range tt.env.svcs {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			b := &defaultEnhancedBackendBuilder{
				k8sClient:        k8sClient,
				annotationParser: annotationParser,
			}

			err := b.Build(context.Background(), tt.args.svc, tt.args.action, tt.args.backendServices)
			assert.NoError(t, err)
		})
	}
}

func Test_defaultEnhancedBackendBuilder_loadBackendServices(t *testing.T) {
	port80 := intstr.FromInt(80)
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "svc-1",
			Annotations: map[string]string{
				"version": "2",
			},
		},
	}
	svc2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "svc-2",
		},
	}

	type env struct {
		svcs []*corev1.Service
	}
	type args struct {
		action          *Action
		namespace       string
		backendServices map[types.NamespacedName]*corev1.Service
	}
	tests := []struct {
		name                string
		env                 env
		args                args
		wantAction          Action
		wantBackendServices map[types.NamespacedName]*corev1.Service
		wantErr             error
	}{
		{
			name: "forward to a single service",
			env: env{
				svcs: []*corev1.Service{svc1, svc2},
			},
			args: args{
				action: &Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace:       "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
			},
		},
		{
			name: "forward to multiple services",
			env: env{
				svcs: []*corev1.Service{svc1, svc2},
			},
			args: args{
				action: &Action{
					Type: ActionTypeForward,
					ForwardConfig: &ForwardActionConfig{
						TargetGroups: []TargetGroupTuple{
							{
								ServiceName: awssdk.String("svc-1"),
								ServicePort: &port80,
							},
							{
								ServiceName: awssdk.String("svc-2"),
								ServicePort: &port80,
							},
						},
					},
				},
				namespace:       "awesome-ns",
				backendServices: map[types.NamespacedName]*corev1.Service{},
			},
			wantBackendServices: map[types.NamespacedName]*corev1.Service{
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-1"}: svc1,
				types.NamespacedName{Namespace: "awesome-ns", Name: "svc-2"}: svc2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			for _, svc := range tt.env.svcs {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}

			b := &defaultEnhancedBackendBuilder{
				k8sClient: k8sClient,
			}
			err := b.loadBackendServices(ctx, tt.args.action, tt.args.namespace, tt.args.backendServices)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opt := equality.IgnoreFakeClientPopulatedFields()
				assert.True(t, cmp.Equal(tt.wantBackendServices, tt.args.backendServices, opt),
					"diff: %v", cmp.Diff(tt.wantBackendServices, tt.args.backendServices, opt))
			}
		})
	}
}
