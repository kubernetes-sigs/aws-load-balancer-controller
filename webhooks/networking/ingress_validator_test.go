package networking

import (
	"context"
	"fmt"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_ingressValidator_checkIngressClass(t *testing.T) {
	type fields struct {
	}
	tests := []struct {
		name                   string
		configuredIngressClass string
		ing                    *networking.Ingress
		ingClassList           []*networking.IngressClass
		expected               bool
		expectedErr            string
	}{
		{
			name:                   "ingress with matching ingress.class annotation",
			configuredIngressClass: "alb",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "alb",
					},
				},
			},
			expected: false,
		},
		{
			name:                   "ingress with not-matching ingress.class annotation",
			configuredIngressClass: "alb",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			expected: true,
		},
		{
			name:                   "ingress without IngressClassName, handling",
			configuredIngressClass: "",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "awesome-ns",
					Name:      "awesome-ing",
				},
				Spec: networking.IngressSpec{
					IngressClassName: nil,
				},
			},
			expected: false,
		},
		{
			name:                   "ingress without IngressClassName, not handling",
			configuredIngressClass: "alb",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "awesome-ns",
					Name:      "awesome-ing",
				},
				Spec: networking.IngressSpec{
					IngressClassName: nil,
				},
			},
			expected: true,
		},
		{
			name: "ingress with IngressClassName that refers to non-existent IngressClass",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "awesome-ns",
					Name:      "awesome-ing",
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("awesome-class"),
				},
			},
			expectedErr: "invalid ingress class: ingressclasses.networking.k8s.io \"awesome-class\" not found",
		},
		{
			name: "ingress with IngressClassName that refers to IngressClass with non-matching controller",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "awesome-ns",
					Name:      "awesome-ing",
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("awesome-class"),
				},
			},
			ingClassList: []*networking.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
					},
					Spec: networking.IngressClassSpec{
						Controller: "other-controller",
					},
				},
			},
			expected: true,
		},
		{
			name: "ingress with IngressClassName that refers to IngressClass with no params",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "awesome-ns",
					Name:      "awesome-ing",
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("awesome-class"),
				},
			},
			ingClassList: []*networking.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
					},
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
					},
				},
			},
			expected: false,
		},
		{
			name: "ingress with IngressClassName that refers to IngressClass with missing IngressClassParams",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "awesome-ns",
					Name:      "awesome-ing",
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("awesome-class"),
				},
			},
			ingClassList: []*networking.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
					},
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
						Parameters: &networking.IngressClassParametersReference{
							APIGroup: awssdk.String("elbv2.k8s.aws"),
							Kind:     "IngressClassParams",
							Name:     "awesome-class-params",
						},
					},
				},
			},
			expected: false,
		},
		{
			name:                   "ingress without IngressClassName, default IngressClass",
			configuredIngressClass: "alb",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "awesome-ns",
					Name:      "awesome-ing",
				},
				Spec: networking.IngressSpec{
					IngressClassName: nil,
				},
			},
			ingClassList: []*networking.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
						Annotations: map[string]string{
							"ingressclass.kubernetes.io/is-default-class": "true",
						},
					},
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
					},
				},
			},
			expected: false,
		},
		{
			name:                   "ingress without IngressClassName, default IngressClass non-matching controller",
			configuredIngressClass: "",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "awesome-ns",
					Name:      "awesome-ing",
				},
				Spec: networking.IngressSpec{
					IngressClassName: nil,
				},
			},
			ingClassList: []*networking.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
						Annotations: map[string]string{
							"ingressclass.kubernetes.io/is-default-class": "true",
						},
					},
					Spec: networking.IngressClassSpec{
						Controller: "other-controller",
					},
				},
			},
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sSchema).
				Build()
			for _, ingClass := range tt.ingClassList {
				assert.NoError(t, k8sClient.Create(ctx, ingClass.DeepCopy()))
			}
			classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher(tt.configuredIngressClass)
			mockMetricsCollector := lbcmetrics.NewMockCollector()
			v := &ingressValidator{
				classLoader:                        ingress.NewDefaultClassLoader(k8sClient, false),
				classAnnotationMatcher:             classAnnotationMatcher,
				manageIngressesWithoutIngressClass: tt.configuredIngressClass == "",
				logger:                             logr.New(&log.NullLogSink{}),
				metricsCollector:                   mockMetricsCollector,
			}
			skip, err := v.checkIngressClass(ctx, tt.ing)
			if tt.expectedErr == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, skip)
			} else {
				assert.EqualError(t, err, tt.expectedErr)
			}
		})
	}
}
func Test_ingressValidator_checkIngressClassAnnotationUsage(t *testing.T) {
	type fields struct {
		disableIngressClassAnnotation bool
	}
	type args struct {
		ing    *networking.Ingress
		oldIng *networking.Ingress
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "ingress creation with matching ingress.class annotation - when new usage enabled",
			fields: fields{
				disableIngressClassAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress creation with matching ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantErr: errors.New("new usage of `kubernetes.io/ingress.class` annotation is forbidden"),
		},
		{
			name: "ingress creation with not-matching ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress creation with non ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching ingress.class annotation - when new usage enabled",
			fields: fields{
				disableIngressClassAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: errors.New("new usage of `kubernetes.io/ingress.class` annotation is forbidden"),
		},
		{
			name: "ingress updates with not-matching ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "envoy",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with non ingress.class annotation - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching ingress.class annotation unchanged - when new usage disabled",
			fields: fields{
				disableIngressClassAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher("alb")
			v := &ingressValidator{
				annotationParser:              annotationParser,
				classAnnotationMatcher:        classAnnotationMatcher,
				disableIngressClassAnnotation: tt.fields.disableIngressClassAnnotation,
				logger:                        logr.New(&log.NullLogSink{}),
			}
			err := v.checkIngressClassAnnotationUsage(tt.args.ing, tt.args.oldIng)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkGroupNameAnnotationUsage(t *testing.T) {
	type fields struct {
		disableIngressGroupAnnotation bool
	}
	type args struct {
		ing    *networking.Ingress
		oldIng *networking.Ingress
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "ingress creation with group.name annotation - when new usage enabled",
			fields: fields{
				disableIngressGroupAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress creation with group.name annotation - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantErr: errors.New("new usage of `alb.ingress.kubernetes.io/group.name` annotation is forbidden"),
		},
		{
			name: "ingress creation with non group.name annotation - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with group.name annotation - when new usage enabled",
			fields: fields{
				disableIngressGroupAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with group.name annotation - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: errors.New("new usage of `alb.ingress.kubernetes.io/group.name` annotation is forbidden"),
		},
		{
			name: "ingress updates with non group.name annotation - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching group.name annotation unchanged - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with matching group.name annotation changed - when new usage disabled",
			fields: fields{
				disableIngressGroupAnnotation: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group-2",
						},
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/group.name": "awesome-group-1",
						},
					},
				},
			},
			wantErr: errors.New("new usage of `alb.ingress.kubernetes.io/group.name` annotation is forbidden"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher("alb")
			v := &ingressValidator{
				annotationParser:              annotationParser,
				classAnnotationMatcher:        classAnnotationMatcher,
				disableIngressGroupAnnotation: tt.fields.disableIngressGroupAnnotation,
				logger:                        logr.New(&log.NullLogSink{}),
			}
			err := v.checkGroupNameAnnotationUsage(tt.args.ing, tt.args.oldIng)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkIngressClassUsage(t *testing.T) {
	type env struct {
		nsList             []*corev1.Namespace
		ingClassList       []*networking.IngressClass
		ingClassParamsList []*elbv2api.IngressClassParams
	}

	type args struct {
		ing    *networking.Ingress
		oldIng *networking.Ingress
	}
	tests := []struct {
		name    string
		env     env
		args    args
		wantErr error
	}{
		{
			name: "ingress creation without IngressClassName",
			env:  env{},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: nil,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress creation with IngressClassName that refers to non-existent IngressClass",
			env:  env{},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: ingressclasses.networking.k8s.io \"awesome-class\" not found"),
		},
		{
			name: "ingress creation with IngressClassName that refers to IngressClass without params",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
					},
				},
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress creation with IngressClassName that refers to IngressClass with IngressClassParams with mismatch namespaceSelector",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
							Labels: map[string]string{
								"team": "another-team",
							},
						},
					},
				},
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "awesome-class-params",
							},
						},
					},
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"team": "awesome-team",
								},
							},
						},
					},
				},
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: namespaceSelector of IngressClassParams awesome-class-params mismatch"),
		},
		{
			name: "ingress creation with IngressClassName that refers to IngressClass with IngressClassParams with matches namespaceSelector",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
							Labels: map[string]string{
								"team": "awesome-team",
							},
						},
					},
				},
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "awesome-class-params",
							},
						},
					},
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							NamespaceSelector: nil,
						},
					},
				},
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with removed IngressClassName",
			env:  env{},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: nil,
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress updates with changed IngressClassName that refers to non-existent IngressClass",
			env:  env{},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("old-awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: ingressclasses.networking.k8s.io \"awesome-class\" not found"),
		},
		{
			name: "ingress updates with added IngressClassName that refers to non-existent IngressClass",
			env:  env{},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: nil,
					},
				},
			},
			wantErr: errors.New("invalid ingress class: ingressclasses.networking.k8s.io \"awesome-class\" not found"),
		},
		{
			name: "ingress updates with unchanged IngressClassName that refers to non-existent IngressClass",
			env:  env{},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
				oldIng: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("awesome-class"),
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sSchema).
				Build()
			for _, ns := range tt.env.nsList {
				assert.NoError(t, k8sClient.Create(ctx, ns.DeepCopy()))
			}
			for _, ingClass := range tt.env.ingClassList {
				assert.NoError(t, k8sClient.Create(ctx, ingClass.DeepCopy()))
			}
			for _, ingClassParams := range tt.env.ingClassParamsList {
				assert.NoError(t, k8sClient.Create(ctx, ingClassParams.DeepCopy()))
			}

			v := &ingressValidator{
				classLoader: ingress.NewDefaultClassLoader(k8sClient, true),
			}
			err := v.checkIngressClassUsage(ctx, tt.args.ing, tt.args.oldIng)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkIngressAnnotationConditions(t *testing.T) {
	type fields struct {
		disableIngressGroupAnnotation bool
	}
	type args struct {
		ing *networking.Ingress
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "ingress has valid condition",
			fields: fields{
				disableIngressGroupAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/condition.svc-1": `[{"field":"query-string","queryStringConfig":{"values":[{"key":"paramA","value":"paramAValue"}]}}]`,
						},
					},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{
							{
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{
											{
												Path: "/ing-1-path",
												Backend: networking.IngressBackend{
													Service: &networking.IngressServiceBackend{
														Name: "svc-1",
														Port: networking.ServiceBackendPort{
															Name: "https",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress has invalid condition",
			fields: fields{
				disableIngressGroupAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/conditions.svc-1": `[{"field":"query-string","queryStringConfig":{"values":[{"key":"paramA","value":""}]}}]`,
						},
					},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{
							{
								IngressRuleValue: networking.IngressRuleValue{
									HTTP: &networking.HTTPIngressRuleValue{
										Paths: []networking.HTTPIngressPath{
											{
												Path: "/ing-1-path",
												Backend: networking.IngressBackend{
													Service: &networking.IngressServiceBackend{
														Name: "svc-1",
														Port: networking.ServiceBackendPort{
															Name: "https",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("ignoring Ingress ns-1/ing-1 since invalid alb.ingress.kubernetes.io/conditions.svc-1 annotation: invalid queryStringConfig: value cannot be empty"),
		},
		{
			name: "ingress rule without HTTP path",
			fields: fields{
				disableIngressGroupAnnotation: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/condition.svc-1": `[{"field":"query-string","queryStringConfig":{"values":[{"key":"paramA","value":"paramAValue"}]}}]`,
						},
					},
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{
							{
								Host: "host.example.com",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher("alb")
			v := &ingressValidator{
				annotationParser:              annotationParser,
				classAnnotationMatcher:        classAnnotationMatcher,
				disableIngressGroupAnnotation: tt.fields.disableIngressGroupAnnotation,
				logger:                        logr.Discard(),
			}
			err := v.checkIngressAnnotationConditions(tt.args.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkFrontendNlbTagsAnnotation(t *testing.T) {
	type args struct {
		ing *networking.Ingress
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "ingress without frontend NLB tags annotation",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ns-1",
						Name:        "ing-1",
						Annotations: map[string]string{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress with valid frontend NLB tags annotation",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress with empty frontend NLB tags annotation",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": "",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress with malformed frontend NLB tags annotation - missing equals",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment,Team=backend",
						},
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags format: tag 'Environment' must be in 'key=value' format"),
		},
		{
			name: "ingress with tag key exceeding maximum length",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": "VeryLongTagKeyThatExceedsTheMaximumAllowedLengthOfOneHundredTwentyEightCharactersWhichIsTheAWSLimitForTagKeysAndShouldCauseValidationError=value",
						},
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key exceeds maximum length of 128 characters"),
		},
		{
			name: "ingress with tag value exceeding maximum length",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=" + strings.Repeat("a", 257),
						},
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag value exceeds maximum length of 256 characters"),
		},
		{
			name: "ingress with AWS reserved tag key",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:cloudformation:stack-name=my-stack,Environment=production",
						},
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:cloudformation:stack-name' is reserved (aws:* pattern)"),
		},
		{
			name: "ingress with AWS reserved tag key - case insensitive",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": "AWS:CloudFormation:StackName=my-stack,Environment=production",
						},
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'AWS:CloudFormation:StackName' is reserved (aws:* pattern)"),
		},
		{
			name: "ingress with too many tags",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
								var tags []string
								for i := 1; i <= 51; i++ {
									tags = append(tags, fmt.Sprintf("tag%d=value%d", i, i))
								}
								return strings.Join(tags, ",")
							}(),
						},
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: number of tags (51) exceeds maximum allowed (50)"),
		},
		{
			name: "ingress with exactly 50 tags - should pass",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
								var tags []string
								for i := 1; i <= 50; i++ {
									tags = append(tags, fmt.Sprintf("tag%d=value%d", i, i))
								}
								return strings.Join(tags, ",")
							}(),
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress with tag key at maximum length - should pass",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": strings.Repeat("a", 128) + "=value",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress with tag value at maximum length - should pass",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "ing-1",
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=" + strings.Repeat("a", 256),
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			v := &ingressValidator{
				annotationParser: annotationParser,
				logger:           logr.Discard(),
			}
			err := v.checkFrontendNlbTagsAnnotation(tt.args.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkFrontendNlbTagsAnnotation_Comprehensive(t *testing.T) {
	tests := []struct {
		name    string
		ing     *networking.Ingress
		wantErr error
	}{
		{
			name: "valid frontend NLB tags annotation",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "valid frontend NLB tags with special characters",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=prod-test,Team=backend_team,Version=1.2.3",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "valid single tag",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ingress without frontend NLB tags annotation",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/scheme": "internet-facing",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "empty frontend NLB tags annotation",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "invalid format - missing equals sign",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment,Team=backend",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags format: tag 'Environment' must be in 'key=value' format"),
		},
		{
			name: "invalid format - empty key",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "=production,Team=backend",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags format: tag key cannot be empty"),
		},
		{
			name: "invalid format - empty value",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=,Team=backend",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags format: tag value cannot be empty"),
		},
		{
			name: "valid format - multiple equals signs in value",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=prod=test,Team=backend",
					},
				},
			},
			wantErr: nil, // Multiple equals signs in value are allowed
		},
		{
			name: "tag key exceeds character limit",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": strings.Repeat("a", 129) + "=value",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key exceeds maximum length of 128 characters"),
		},
		{
			name: "tag value exceeds character limit",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=" + strings.Repeat("a", 257),
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag value exceeds maximum length of 256 characters"),
		},
		{
			name: "too many tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							var tags []string
							for i := 0; i < 51; i++ {
								tags = append(tags, fmt.Sprintf("key%d=value%d", i, i))
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: number of tags (51) exceeds maximum allowed (50)"),
		},
		{
			name: "reserved AWS tag key",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:cloudformation:stack-name=mystack,Team=backend",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:cloudformation:stack-name' is reserved (aws:* pattern)"),
		},
		{
			name: "reserved AWS tag key with different case",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "AWS:Region=us-west-2,Team=backend",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'AWS:Region' is reserved (aws:* pattern)"),
		},
		{
			name: "duplicate tag keys",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend,Environment=staging",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: duplicate tag key 'Environment'"),
		},
		{
			name: "whitespace in tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": " Environment = production , Team = backend ",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "maximum allowed tags (50)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							var tags []string
							for i := 0; i < 50; i++ {
								tags = append(tags, fmt.Sprintf("key%d=value%d", i, i))
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "maximum key length (128 characters)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": strings.Repeat("a", 128) + "=value",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "maximum value length (256 characters)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=" + strings.Repeat("a", 256),
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			v := &ingressValidator{
				annotationParser: annotationParser,
				logger:           logr.New(&log.NullLogSink{}),
			}
			err := v.checkFrontendNlbTagsAnnotation(tt.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkFrontendNlbTagsAnnotation_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		ing     *networking.Ingress
		wantErr error
	}{
		{
			name: "malformed annotation - only commas",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": ",,,",
					},
				},
			},
			wantErr: nil, // Empty tags after splitting should be ignored
		},
		{
			name: "malformed annotation - trailing comma",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,",
					},
				},
			},
			wantErr: nil, // Trailing comma should be handled gracefully
		},
		{
			name: "malformed annotation - leading comma",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": ",Environment=production",
					},
				},
			},
			wantErr: nil, // Leading comma should be handled gracefully
		},
		{
			name: "special characters in key and value",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "app.kubernetes.io/name=my-app,app.kubernetes.io/version=1.0.0",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "unicode characters in tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=测试,Team=backend",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "numeric keys and values",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "123=456,789=012",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "case sensitivity test",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,environment=staging",
					},
				},
			},
			wantErr: nil, // Keys are case-sensitive, so this should be allowed
		},
		{
			name: "performance test - large tag set within limits",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							var tags []string
							// Create 45 tags with maximum allowed key and value lengths
							for i := 0; i < 45; i++ {
								key := fmt.Sprintf("key%d", i) + strings.Repeat("a", 124-len(fmt.Sprintf("key%d", i)))
								value := strings.Repeat("v", 256)
								tags = append(tags, key+"="+value)
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "external managed tag conflict simulation",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "kubernetes.io/cluster/my-cluster=owned,Environment=production",
					},
				},
			},
			wantErr: nil, // This test simulates external managed tags, but validation should pass at webhook level
		},
		{
			name: "reserved tag key variations",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:ec2:instance-id=i-1234567890abcdef0",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:ec2:instance-id' is reserved (aws:* pattern)"),
		},
		{
			name: "mixed valid and invalid tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,aws:region=us-west-2,Team=backend",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:region' is reserved (aws:* pattern)"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			v := &ingressValidator{
				annotationParser: annotationParser,
				logger:           logr.New(&log.NullLogSink{}),
			}
			err := v.checkFrontendNlbTagsAnnotation(tt.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkFrontendNlbTagsAnnotation_PerformanceAndLimits(t *testing.T) {
	tests := []struct {
		name    string
		ing     *networking.Ingress
		wantErr error
	}{
		{
			name: "performance test - parsing large valid annotation",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							var tags []string
							// Create 50 tags with reasonable key and value lengths
							for i := 0; i < 50; i++ {
								key := fmt.Sprintf("performance-test-key-%d", i)
								value := fmt.Sprintf("performance-test-value-%d", i)
								tags = append(tags, key+"="+value)
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "boundary test - exactly 50 tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							var tags []string
							for i := 0; i < 50; i++ {
								tags = append(tags, fmt.Sprintf("key%d=value%d", i, i))
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "boundary test - 51 tags (exceeds limit)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							var tags []string
							for i := 0; i < 51; i++ {
								tags = append(tags, fmt.Sprintf("key%d=value%d", i, i))
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: number of tags (51) exceeds maximum allowed (50)"),
		},
		{
			name: "boundary test - key exactly 128 characters",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": strings.Repeat("a", 128) + "=value",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "boundary test - key 129 characters (exceeds limit)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": strings.Repeat("a", 129) + "=value",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key exceeds maximum length of 128 characters"),
		},
		{
			name: "boundary test - value exactly 256 characters",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "key=" + strings.Repeat("v", 256),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "boundary test - value 257 characters (exceeds limit)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "key=" + strings.Repeat("v", 257),
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag value exceeds maximum length of 256 characters"),
		},
		{
			name: "stress test - complex annotation with mixed valid and invalid patterns",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "valid-key=valid-value,aws:invalid=reserved,another-valid=value",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:invalid' is reserved (aws:* pattern)"),
		},
		{
			name: "external managed tag simulation - kubernetes tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "kubernetes.io/cluster/test=owned,kubernetes.io/service-name=test-service",
					},
				},
			},
			wantErr: nil, // These should be allowed at webhook level
		},
		{
			name: "external managed tag simulation - elbv2 controller tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "elbv2.k8s.aws/cluster=test-cluster,ingress.k8s.aws/resource=LoadBalancer",
					},
				},
			},
			wantErr: nil, // These should be allowed at webhook level
		},
		{
			name: "reserved tag variations - different aws prefixes",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:ec2:instance-id=i-123",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:ec2:instance-id' is reserved (aws:* pattern)"),
		},
		{
			name: "case insensitive aws prefix check",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Aws:Ec2:InstanceId=i-123",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'Aws:Ec2:InstanceId' is reserved (aws:* pattern)"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			v := &ingressValidator{
				annotationParser: annotationParser,
				logger:           logr.New(&log.NullLogSink{}),
			}
			err := v.checkFrontendNlbTagsAnnotation(tt.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_ValidateCreate_FrontendNlbTags(t *testing.T) {
	tests := []struct {
		name    string
		ing     *networking.Ingress
		wantErr error
	}{
		{
			name: "create ingress with valid frontend NLB tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":                 "alb",
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "create ingress with invalid frontend NLB tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":                 "alb",
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:region=us-west-2",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:region' is reserved (aws:* pattern)"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sSchema).
				Build()

			classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher("alb")
			mockMetricsCollector := lbcmetrics.NewMockCollector()
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")

			v := &ingressValidator{
				annotationParser:                   annotationParser,
				classLoader:                        ingress.NewDefaultClassLoader(k8sClient, false),
				classAnnotationMatcher:             classAnnotationMatcher,
				manageIngressesWithoutIngressClass: false,
				logger:                             logr.New(&log.NullLogSink{}),
				metricsCollector:                   mockMetricsCollector,
			}

			err := v.ValidateCreate(ctx, tt.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_ValidateUpdate_FrontendNlbTags(t *testing.T) {
	tests := []struct {
		name    string
		ing     *networking.Ingress
		oldIng  *networking.Ingress
		wantErr error
	}{
		{
			name: "update ingress with valid frontend NLB tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":                 "alb",
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=staging,Team=backend",
					},
				},
			},
			oldIng: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":                 "alb",
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "update ingress with invalid frontend NLB tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":                 "alb",
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:cloudformation:stack-name=test",
					},
				},
			},
			oldIng: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":                 "alb",
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:cloudformation:stack-name' is reserved (aws:* pattern)"),
		},
		{
			name: "update ingress removing frontend NLB tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "alb",
					},
				},
			},
			oldIng: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":                 "alb",
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production",
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sSchema).
				Build()

			classAnnotationMatcher := ingress.NewDefaultClassAnnotationMatcher("alb")
			mockMetricsCollector := lbcmetrics.NewMockCollector()
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")

			v := &ingressValidator{
				annotationParser:                   annotationParser,
				classLoader:                        ingress.NewDefaultClassLoader(k8sClient, false),
				classAnnotationMatcher:             classAnnotationMatcher,
				manageIngressesWithoutIngressClass: false,
				logger:                             logr.New(&log.NullLogSink{}),
				metricsCollector:                   mockMetricsCollector,
			}

			err := v.ValidateUpdate(ctx, tt.ing, tt.oldIng)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ingressValidator_checkFrontendNlbTagsAnnotation_AdditionalEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		ing     *networking.Ingress
		wantErr error
	}{
		{
			name: "extremely long annotation value with valid tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							// Create a very long but valid annotation
							var tags []string
							for i := 0; i < 25; i++ {
								key := fmt.Sprintf("very-long-key-name-with-lots-of-characters-%d", i)
								value := fmt.Sprintf("very-long-value-with-lots-of-characters-and-details-%d", i)
								tags = append(tags, key+"="+value)
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with only whitespace",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "   \t\n   ",
					},
				},
			},
			wantErr: nil, // Should be treated as empty
		},
		{
			name: "annotation with mixed whitespace and valid tags",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "  Environment = production  ,  Team = backend  ",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with empty tag pairs mixed with valid ones",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,,,,Team=backend,,,",
					},
				},
			},
			wantErr: nil, // Empty pairs should be ignored
		},
		{
			name: "annotation with special characters in values",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=prod-test_env.v1,Team=backend@company.com,Version=1.2.3-beta+build.123",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with URL-like values",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Repository=https://github.com/company/repo,Documentation=https://docs.company.com/api",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with JSON-like values (without commas)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Config={\"env\":\"prod\"},Metadata={\"version\":\"1.0\"}",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with path-like keys",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "app.kubernetes.io/name=myapp,app.kubernetes.io/version=1.0.0,company.com/team=backend",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with boundary key length (exactly 128 chars)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": strings.Repeat("k", 128) + "=value",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with boundary value length (exactly 256 chars)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "key=" + strings.Repeat("v", 256),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with key exceeding limit by 1 char (129 chars)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": strings.Repeat("k", 129) + "=value",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key exceeds maximum length of 128 characters"),
		},
		{
			name: "annotation with value exceeding limit by 1 char (257 chars)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "key=" + strings.Repeat("v", 257),
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag value exceeds maximum length of 256 characters"),
		},
		{
			name: "annotation with exactly 50 tags (boundary test)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							var tags []string
							for i := 0; i < 50; i++ {
								tags = append(tags, fmt.Sprintf("tag%02d=value%02d", i, i))
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annotation with 51 tags (exceeds limit)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
							var tags []string
							for i := 0; i < 51; i++ {
								tags = append(tags, fmt.Sprintf("tag%02d=value%02d", i, i))
							}
							return strings.Join(tags, ",")
						}(),
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: number of tags (51) exceeds maximum allowed (50)"),
		},
		{
			name: "annotation with various AWS reserved key patterns",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:autoscaling:groupName=test",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'aws:autoscaling:groupName' is reserved (aws:* pattern)"),
		},
		{
			name: "annotation with mixed case AWS reserved key",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "AwS:CloudFormation:StackId=test",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: tag key 'AwS:CloudFormation:StackId' is reserved (aws:* pattern)"),
		},
		{
			name: "annotation with non-AWS reserved-looking key (should pass)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "myaws:custom:key=value,aws-like-key=value",
					},
				},
			},
			wantErr: nil, // These don't start with "aws:" so should be allowed
		},
		{
			name: "annotation with duplicate keys (case sensitive)",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=prod,environment=staging",
					},
				},
			},
			wantErr: nil, // Keys are case-sensitive, so this should be allowed
		},
		{
			name: "annotation with actual duplicate keys",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns-1",
					Name:      "ing-1",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=prod,Team=backend,Environment=staging",
					},
				},
			},
			wantErr: errors.New("invalid frontend NLB tags: duplicate tag key 'Environment'"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			v := &ingressValidator{
				annotationParser: annotationParser,
				logger:           logr.New(&log.NullLogSink{}),
			}
			err := v.checkFrontendNlbTagsAnnotation(tt.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
