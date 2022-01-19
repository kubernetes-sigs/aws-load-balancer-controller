package ingress

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"testing"
)

func Test_defaultClassLoader_Load(t *testing.T) {
	type env struct {
		nsList             []*corev1.Namespace
		ingClassList       []*networking.IngressClass
		ingClassParamsList []*elbv2api.IngressClassParams
	}
	type args struct {
		ing *networking.Ingress
	}
	tests := []struct {
		name    string
		env     env
		args    args
		want    ClassConfiguration
		wantErr error
	}{
		{
			name: "when IngressClassName unspecified",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
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
						IngressClassName: nil,
					},
				},
			},
			wantErr: errors.New("invalid ingress class: spec.ingressClassName is nil"),
		},
		{
			name: "when IngressClass not found",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: ingressclasses.networking.k8s.io \"awesome-class\" not found"),
		},
		{
			name: "when IngressClass found and belong to other controller",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
						},
					},
				},
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "some-other-controller",
							Parameters: &networking.IngressClassParametersReference{
								Kind: "IngressClassParams",
								Name: "awesome-class-config",
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			want: ClassConfiguration{
				IngClass: &networking.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
					},
					Spec: networking.IngressClassSpec{
						Controller: "some-other-controller",
						Parameters: &networking.IngressClassParametersReference{
							Kind: "IngressClassParams",
							Name: "awesome-class-config",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "when IngressClass is ALB - without IngressClassParams",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
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
							Parameters: nil,
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			want: ClassConfiguration{
				IngClass: &networking.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
					},
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
						Parameters: nil,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "when IngressClass is ALB - with invalid IngressClassParams - empty APIGroup",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
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
								Kind: "IngressClassParams",
								Name: "awesome-class-params",
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: IngressClass awesome-class references unknown parameters"),
		},
		{
			name: "when IngressClass is ALB - with invalid IngressClassParams - unknown APIGroup",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
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
								APIGroup: aws.String("some.other.group/v1"),
								Kind:     "IngressClassParams",
								Name:     "awesome-class-params",
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: IngressClass awesome-class references unknown parameters"),
		},
		{
			name: "when IngressClass is ALB - with invalid IngressClassParams - unknown Kind",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
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
								APIGroup: aws.String("elbv2.k8s.aws"),
								Kind:     "SomeOtherKind",
								Name:     "awesome-class-params",
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: IngressClass awesome-class references unknown parameters"),
		},
		{
			name: "when IngressClass is ALB - with invalid IngressClassParams - non-exists",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
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
								APIGroup: aws.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "awesome-class-params",
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: ingressclassparamses.elbv2.k8s.aws \"awesome-class-params\" not found"),
		},
		{
			name: "when IngressClass is ALB - with invalid IngressClassParams - namespaceSelector mismatch",
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
								APIGroup: aws.String("elbv2.k8s.aws"),
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			wantErr: errors.New("invalid ingress class: namespaceSelector of IngressClassParams awesome-class-params mismatch"),
		},
		{
			name: "when IngressClass is ALB - with valid IngressClassParams",
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
								APIGroup: aws.String("elbv2.k8s.aws"),
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
						IngressClassName: aws.String("awesome-class"),
					},
				},
			},
			want: ClassConfiguration{
				IngClass: &networking.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
					},
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
						Parameters: &networking.IngressClassParametersReference{
							APIGroup: aws.String("elbv2.k8s.aws"),
							Kind:     "IngressClassParams",
							Name:     "awesome-class-params",
						},
					},
				},
				IngClassParams: &elbv2api.IngressClassParams{
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

			l := &defaultClassLoader{
				client: k8sClient,
			}
			got, err := l.Load(ctx, tt.args.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opt := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
				}
				assert.True(t, cmp.Equal(tt.want, got, opt),
					"diff: %v", cmp.Diff(tt.want, got, opt))
			}
		})
	}
}

func Test_defaultClassLoader_validateIngressClassParamsNamespaceRestriction(t *testing.T) {
	type env struct {
		nsList []*corev1.Namespace
	}
	type args struct {
		admissionReq   *admission.Request
		ing            *networking.Ingress
		ingClassParams *elbv2api.IngressClassParams
	}
	tests := []struct {
		name    string
		env     env
		args    args
		wantErr error
	}{
		{
			name: "when ingressClassParams have empty namespaceSelector",
			env: env{
				nsList: []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "awesome-ns",
						},
					},
				},
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-ns",
						Name:      "awesome-ns",
					},
				},
				ingClassParams: &elbv2api.IngressClassParams{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
					},
					Spec: elbv2api.IngressClassParamsSpec{
						NamespaceSelector: nil,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "when ingressClassParams have nonempty namespaceSelector - matches Ingress's namespace [without admission request]",
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
			},
			args: args{
				admissionReq: nil,
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
				},
				ingClassParams: &elbv2api.IngressClassParams{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
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
			wantErr: nil,
		},
		{
			name: "when ingressClassParams have nonempty namespaceSelector - matches Ingress's namespace [with admission request]",
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
			},
			args: args{
				admissionReq: &admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Namespace: "awesome-ns",
					},
				},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "",
						Name:      "awesome-ing",
					},
				},
				ingClassParams: &elbv2api.IngressClassParams{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
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
			wantErr: nil,
		},
		{
			name: "when ingressClassParams have nonempty namespaceSelector - mismatches Ingress's namespace",
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
			},
			args: args{
				admissionReq: nil,
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "awesome-ns",
						Name:      "awesome-ing",
					},
				},
				ingClassParams: &elbv2api.IngressClassParams{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class",
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
			wantErr: errors.New("namespaceSelector of IngressClassParams awesome-class mismatch"),
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

			l := &defaultClassLoader{
				client: k8sClient,
			}
			if tt.args.admissionReq != nil {
				ctx = webhook.ContextWithAdmissionRequest(ctx, *tt.args.admissionReq)
			}
			err := l.validateIngressClassParamsNamespaceRestriction(ctx, tt.args.ing, tt.args.ingClassParams)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
