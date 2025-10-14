package ingress

import (
	"context"
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	wafv2sdk "github.com/aws/aws-sdk-go-v2/service/wafv2"
	wafv2types "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
	wafregionalmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafregional"
	wafv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafv2"
)

func Test_defaultModelBuildTask_buildWAFv2WebACLAssociation(t *testing.T) {
	type fields struct {
		ingGroup Group
	}
	type args struct {
		lbARN core.StringToken
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wafv2model.WebACLAssociation
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when all ingresses don't have wafv2-acl-arn set",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-1",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want:    nil,
			wantErr: assert.NoError,
		},
		{
			name: "when all ingresses have wafv2-acl-arn annotation set to wafv2-arn-1",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "wafv2-arn-1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "wafv2-arn-1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "wafv2-arn-1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when one of ingresses have wafv2-acl-arn annotation set to wafv2-arn-1",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "wafv2-arn-1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "wafv2-arn-1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when all ingresses have wafv2-acl-arn annotation set to none",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "none",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "none",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when one of ingresses have wafv2-acl-arn annotation set to none",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "none",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when ingresses have different value of wafv2-acl-arn annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "wafv2-arn-1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "none",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				assert.EqualError(t, err, "conflicting WAFv2 WebACL ARNs: [none wafv2-arn-1]", msgAndArgs...)
				return false
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				stack:            stack,
				annotationParser: annotationParser,
			}
			got, err := task.buildWAFv2WebACLAssociation(context.Background(), tt.args.lbARN)
			if !tt.wantErr(t, err, fmt.Sprintf("buildWAFv2WebACLAssociation(ctx, %v)", tt.args.lbARN)) {
				return
			}
			opts := cmpopts.IgnoreTypes(core.ResourceMeta{})
			assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
		})
	}
}

func Test_defaultModelBuildTask_buildWAFRegionalWebACLAssociation(t *testing.T) {
	type fields struct {
		ingGroup Group
	}
	type args struct {
		lbARN core.StringToken
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *wafregionalmodel.WebACLAssociation
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when all ingresses don't have waf-acl-id set",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-1",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want:    nil,
			wantErr: assert.NoError,
		},
		{
			name: "when all ingresses have waf-acl-id annotation set to web-acl-id-1",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/waf-acl-id": "web-acl-id-1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/waf-acl-id": "web-acl-id-1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafregionalmodel.WebACLAssociation{
				Spec: wafregionalmodel.WebACLAssociationSpec{
					WebACLID:    "web-acl-id-1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when one of ingresses have waf-acl-id annotation set to web-acl-id-1",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/waf-acl-id": "web-acl-id-1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafregionalmodel.WebACLAssociation{
				Spec: wafregionalmodel.WebACLAssociationSpec{
					WebACLID:    "web-acl-id-1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when all ingresses have waf-acl-id annotation set to none",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/waf-acl-id": "none",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/waf-acl-id": "none",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafregionalmodel.WebACLAssociation{
				Spec: wafregionalmodel.WebACLAssociationSpec{
					WebACLID:    "",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when one of ingresses have waf-acl-id annotation set to none",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/waf-acl-id": "none",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafregionalmodel.WebACLAssociation{
				Spec: wafregionalmodel.WebACLAssociationSpec{
					WebACLID:    "",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when ingresses have different value of waf-acl-id annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/waf-acl-id": "web-acl-id-1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/waf-acl-id": "none",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				assert.EqualError(t, err, "conflicting WAFClassic WebACL IDs: [none web-acl-id-1]", msgAndArgs...)
				return false
			},
		},
		{
			name: "when using deprecated web-acl-id annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/web-acl-id": "web-acl-id-1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want: &wafregionalmodel.WebACLAssociation{
				Spec: wafregionalmodel.WebACLAssociationSpec{
					WebACLID:    "web-acl-id-1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				stack:            stack,
				annotationParser: annotationParser,
			}
			got, err := task.buildWAFRegionalWebACLAssociation(context.Background(), tt.args.lbARN)
			if !tt.wantErr(t, err, fmt.Sprintf("buildWAFRegionalWebACLAssociation(ctx, %v)", tt.args.lbARN)) {
				return
			}
			opts := cmpopts.IgnoreTypes(core.ResourceMeta{})
			assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
		})
	}
}

func Test_defaultModelBuildTask_buildShieldProtection(t *testing.T) {
	type fields struct {
		ingGroup Group
	}
	type args struct {
		lbARN core.StringToken
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *shieldmodel.Protection
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when all ingresses don't have shield-advanced-protection set",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-1",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			want:    nil,
			wantErr: assert.NoError,
		},
		{
			name: "when all ingresses have shield-advanced-protection annotation set to true",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/shield-advanced-protection": "true",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/shield-advanced-protection": "true",
									},
								},
							},
						},
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
			wantErr: assert.NoError,
		},
		{
			name: "when one of ingresses have shield-advanced-protection annotation set to true",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/shield-advanced-protection": "true",
									},
								},
							},
						},
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
			wantErr: assert.NoError,
		},
		{
			name: "when all ingresses have shield-advanced-protection annotation set to false",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/shield-advanced-protection": "false",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/shield-advanced-protection": "false",
									},
								},
							},
						},
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
			wantErr: assert.NoError,
		},
		{
			name: "when one of ingresses have shield-advanced-protection annotation set to false",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/shield-advanced-protection": "false",
									},
								},
							},
						},
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
			wantErr: assert.NoError,
		},
		{
			name: "when ingresses have different value of shield-advanced-protection annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/shield-advanced-protection": "true",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/shield-advanced-protection": "false",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				assert.EqualError(t, err, "conflicting enable shield advanced protection", msgAndArgs...)
				return false
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				stack:            stack,
				annotationParser: annotationParser,
			}
			got, err := task.buildShieldProtection(context.Background(), tt.args.lbARN)
			if !tt.wantErr(t, err, fmt.Sprintf("buildShieldProtection(ctx, %v)", tt.args.lbARN)) {
				return
			}
			opts := cmpopts.IgnoreTypes(core.ResourceMeta{})
			assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
		})
	}
}

func Test_defaultModelBuildTask_buildWAFv2WebACLAssociationFromWAFv2Name(t *testing.T) {
	type getWebACLCall struct {
		req  *wafv2sdk.ListWebACLsInput
		resp *wafv2sdk.ListWebACLsOutput
		err  error
	}
	type fields struct {
		ingGroup Group
	}
	type args struct {
		lbARN          core.StringToken
		cache          map[string]string
		getWebACLCalls []getWebACLCall
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      *wafv2model.WebACLAssociation
		wantErr   assert.ErrorAssertionFunc
		wantCache map[string]string
	}{
		{
			name: "when one ingress has wafv2-acl-name annotation set to none",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-name": "none",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-1",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN:          core.LiteralStringToken("awesome-lb-arn"),
				cache:          map[string]string{},
				getWebACLCalls: []getWebACLCall{},
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr:   assert.NoError,
			wantCache: map[string]string{},
		},
		{
			name: "when one ingress has wafv2-acl-name annotation set",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-name": "web-acl-name1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-1",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
				cache: map[string]string{},
				getWebACLCalls: []getWebACLCall{
					{
						req: &wafv2sdk.ListWebACLsInput{
							Scope: wafv2types.ScopeRegional,
						},
						resp: &wafv2sdk.ListWebACLsOutput{
							WebACLs: []wafv2types.WebACLSummary{
								{
									Name: awssdk.String("web-acl-name1"),
									ARN:  awssdk.String("web-acl-arn1"),
								},
							},
						},
						err: nil,
					},
				},
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "web-acl-arn1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
			wantCache: map[string]string{
				"web-acl-name1": "web-acl-arn1",
			},
		},
		{
			name: "when ingresses have different value of wafv2-acl-name annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-name": "web-acl-name1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-name": "web-acl-name2",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN:          core.LiteralStringToken("awesome-lb-arn"),
				cache:          map[string]string{},
				getWebACLCalls: []getWebACLCall{},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				assert.EqualError(t, err, "conflicting WAFv2 WebACL names: [web-acl-name1 web-acl-name2]", msgAndArgs...)
				return false
			},
			wantCache: map[string]string{},
		},
		{
			name: "when one ingress has web-acl-name1 annotation and other ingress has web-acl-arn1 annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-name": "web-acl-name1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-arn": "web-acl-arn1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
				cache: map[string]string{},
				getWebACLCalls: []getWebACLCall{
					{
						req: &wafv2sdk.ListWebACLsInput{
							Scope: wafv2types.ScopeRegional,
						},
						resp: &wafv2sdk.ListWebACLsOutput{
							WebACLs: []wafv2types.WebACLSummary{
								{
									Name: awssdk.String("web-acl-name1"),
									ARN:  awssdk.String("web-acl-arn1"),
								},
							},
						},
						err: nil,
					},
				},
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "web-acl-arn1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
			wantCache: map[string]string{
				"web-acl-name1": "web-acl-arn1",
			},
		},
		{
			name: "when one ingress has wafv2AclName ingressClassParam set",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										WAFv2ACLName: "web-acl-name1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
				cache: map[string]string{},
				getWebACLCalls: []getWebACLCall{
					{
						req: &wafv2sdk.ListWebACLsInput{
							Scope: wafv2types.ScopeRegional,
						},
						resp: &wafv2sdk.ListWebACLsOutput{
							WebACLs: []wafv2types.WebACLSummary{
								{
									Name: awssdk.String("web-acl-name1"),
									ARN:  awssdk.String("web-acl-arn1"),
								},
							},
						},
						err: nil,
					},
				},
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "web-acl-arn1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
			wantCache: map[string]string{
				"web-acl-name1": "web-acl-arn1",
			},
		},
		{
			name: "when one ingress has both wafv2AclName ingressClassParam and web-acl-name1 annotation set",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-name": "web-acl-name1",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										WAFv2ACLName: "web-acl-name1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
				cache: map[string]string{},
				getWebACLCalls: []getWebACLCall{
					{
						req: &wafv2sdk.ListWebACLsInput{
							Scope: wafv2types.ScopeRegional,
						},
						resp: &wafv2sdk.ListWebACLsOutput{
							WebACLs: []wafv2types.WebACLSummary{
								{
									Name: awssdk.String("web-acl-name1"),
									ARN:  awssdk.String("web-acl-arn1"),
								},
							},
						},
						err: nil,
					},
				},
			},
			want: &wafv2model.WebACLAssociation{
				Spec: wafv2model.WebACLAssociationSpec{
					WebACLARN:   "web-acl-arn1",
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
			wantErr: assert.NoError,
			wantCache: map[string]string{
				"web-acl-name1": "web-acl-arn1",
			},
		},
		{
			name: "when one ingress has  wafv2AclName ingressClassParam set to web-acl-name1 and other ingress has web-acl-name2 annotation set",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "awesome-ing-0",
									Annotations: map[string]string{},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										WAFv2ACLName: "web-acl-name1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-name": "web-acl-name2",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN:          core.LiteralStringToken("awesome-lb-arn"),
				cache:          map[string]string{},
				getWebACLCalls: []getWebACLCall{},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				assert.EqualError(t, err, "conflicting WAFv2 WebACL names: [web-acl-name1 web-acl-name2]", msgAndArgs...)
				return false
			},
			wantCache: map[string]string{},
		},
		{
			name: "name not found",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "awesome-ing-0",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/wafv2-acl-name": "web-acl-name1",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										WAFv2ACLName: "web-acl-name1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				lbARN: core.LiteralStringToken("awesome-lb-arn"),
				cache: map[string]string{},
				getWebACLCalls: []getWebACLCall{
					{
						req: &wafv2sdk.ListWebACLsInput{
							Scope: wafv2types.ScopeRegional,
						},
						resp: &wafv2sdk.ListWebACLsOutput{
							WebACLs: []wafv2types.WebACLSummary{
								{
									Name: awssdk.String("some other name"),
									ARN:  awssdk.String("web-acl-arn1"),
								},
							},
						},
						err: nil,
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				assert.EqualError(t, err, "couldn't find WAFv2 WebACL with name: web-acl-name1", msgAndArgs...)
				return false
			},
			wantCache: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			wafv2Client := services.NewMockWAFv2(ctrl)
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")

			for _, call := range tt.args.getWebACLCalls {
				wafv2Client.EXPECT().
					ListWebACLsWithContext(gomock.Any(), call.req).
					Return(call.resp, call.err)
			}

			task := &defaultModelBuildTask{
				ingGroup:              tt.fields.ingGroup,
				stack:                 stack,
				annotationParser:      annotationParser,
				wafv2Client:           wafv2Client,
				webACLNameToArnMapper: newWebACLNameToArnMapper(wafv2Client, defaultWebACLNameToARNCacheTTL),
			}

			for webACLName, cachedArn := range tt.args.cache {
				task.webACLNameToArnMapper.cache.Set(webACLName, cachedArn, defaultWebACLNameToARNCacheTTL)
			}

			got, err := task.buildWAFv2WebACLAssociation(context.Background(), tt.args.lbARN)

			opts := cmpopts.IgnoreTypes(core.ResourceMeta{})
			assert.True(t, cmp.Equal(tt.want, got, opts), "diff", cmp.Diff(tt.want, got, opts))
			if !tt.wantErr(t, err, fmt.Sprintf("buildWAFRegionalWebACLAssociation(ctx, %v)", tt.args.lbARN)) {
				return
			}
			assert.Equal(t, len(tt.wantCache), task.webACLNameToArnMapper.cache.Len())

			for webACLName, expectedArn := range tt.wantCache {
				rawCacheItem, exists := task.webACLNameToArnMapper.cache.Get(webACLName)
				assert.True(t, exists)
				cachedArn := rawCacheItem.(string)
				assert.Equal(t, expectedArn, cachedArn)
			}
		})
	}
}
