package networking

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_ingressClassValidator(t *testing.T) {
	tests := []struct {
		name               string
		ingClassParamsList []*elbv2api.IngressClassParams
		ingClass           *networking.IngressClass
		oldIngClass        *networking.IngressClass
		wantErr            string
	}{
		{
			name: "creation other controller",
			ingClass: &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "awesome-class",
				},
				Spec: networking.IngressClassSpec{
					Controller: "not-us",
					Parameters: &networking.IngressClassParametersReference{
						APIGroup: awssdk.String("not-us"),
						Kind:     "OtherParams",
						Name:     "awesome-class-params",
					},
				},
			},
		},
		{
			name: "creation no params",
			ingClass: &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "awesome-class",
				},
				Spec: networking.IngressClassSpec{
					Controller: "ingress.k8s.aws/alb",
				},
			},
		},
		{
			name: "creation with params",
			ingClass: &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "awesome-class",
				},
				Spec: networking.IngressClassSpec{
					Controller: "ingress.k8s.aws/alb",
				},
			},
		},
		{
			name: "creation with params",
			ingClass: &networking.IngressClass{
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
			ingClassParamsList: []*elbv2api.IngressClassParams{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class-params",
					},
				},
			},
		},
		{
			name: "creation missing apiGroup",
			ingClass: &networking.IngressClass{
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
			ingClassParamsList: []*elbv2api.IngressClassParams{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class-params",
					},
				},
			},
			wantErr: "spec.parameters.apiGroup: Required value: must be \"elbv2.k8s.aws\"",
		},
		{
			name: "creation wrong apiGroup",
			ingClass: &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "awesome-class",
				},
				Spec: networking.IngressClassSpec{
					Controller: "ingress.k8s.aws/alb",
					Parameters: &networking.IngressClassParametersReference{
						APIGroup: awssdk.String("other.k8s.aws"),
						Kind:     "IngressClassParams",
						Name:     "awesome-class-params",
					},
				},
			},
			ingClassParamsList: []*elbv2api.IngressClassParams{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class-params",
					},
				},
			},
			wantErr: "spec.parameters.apiGroup: Forbidden: must be \"elbv2.k8s.aws\"",
		},
		{
			name: "creation missing kind",
			ingClass: &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "awesome-class",
				},
				Spec: networking.IngressClassSpec{
					Controller: "ingress.k8s.aws/alb",
					Parameters: &networking.IngressClassParametersReference{
						APIGroup: awssdk.String("elbv2.k8s.aws"),
						Name:     "awesome-class-params",
					},
				},
			},
			ingClassParamsList: []*elbv2api.IngressClassParams{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class-params",
					},
				},
			},
			wantErr: "spec.parameters.kind: Required value: must be \"IngressClassParams\"",
		},
		{
			name: "creation wrong kind",
			ingClass: &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "awesome-class",
				},
				Spec: networking.IngressClassSpec{
					Controller: "ingress.k8s.aws/alb",
					Parameters: &networking.IngressClassParametersReference{
						APIGroup: awssdk.String("elbv2.k8s.aws"),
						Kind:     "OtherKind",
						Name:     "awesome-class-params",
					},
				},
			},
			ingClassParamsList: []*elbv2api.IngressClassParams{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awesome-class-params",
					},
				},
			},
			wantErr: "spec.parameters.kind: Forbidden: must be \"IngressClassParams\"",
		},
		{
			name: "creation params not found",
			ingClass: &networking.IngressClass{
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
			wantErr: "spec.parameters.name: Not found: \"awesome-class-params\"",
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
			for _, ingClassParams := range tt.ingClassParamsList {
				assert.NoError(t, k8sClient.Create(ctx, ingClassParams.DeepCopy()))
			}

			v := &ingressClassValidator{
				client: k8sClient,
			}
			var err error
			if tt.oldIngClass == nil {
				err = v.ValidateCreate(ctx, tt.ingClass)
			} else {
				err = v.ValidateUpdate(ctx, tt.ingClass, tt.oldIngClass)
			}
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
