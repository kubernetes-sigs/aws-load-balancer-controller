package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
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
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

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
				logger:                        &log.NullLogger{},
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
				logger:                        &log.NullLogger{},
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
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
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
				classLoader: ingress.NewDefaultClassLoader(k8sClient),
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
