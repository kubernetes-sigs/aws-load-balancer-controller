package wafv2

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	wafv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/wafv2"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_webACLAssociationSynthesizer_Synthesize(t *testing.T) {
	type getAssociatedWebACLCall struct {
		resourceARN string
		webACLARN   string
		err         error
	}
	type associateWebACLCall struct {
		resourceARN string
		webACLARN   string
		err         error
	}
	type disassociateWebACLCall struct {
		resourceARN string
		err         error
	}
	type fields struct {
		webACLAssociationSpecs   []wafv2model.WebACLAssociationSpec
		getAssociatedWebACLCalls []getAssociatedWebACLCall
		associateWebACLCalls     []associateWebACLCall
		disassociateWebACLCall   []disassociateWebACLCall
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when there is no webACLAssociation resource",
			fields: fields{
				webACLAssociationSpecs: []wafv2model.WebACLAssociationSpec{},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when webACL is desired and it's already enabled with same webACL on LB",
			fields: fields{
				webACLAssociationSpecs: []wafv2model.WebACLAssociationSpec{
					{
						WebACLARN:   "web-acl-arn-1",
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getAssociatedWebACLCalls: []getAssociatedWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "web-acl-arn-1",
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when webACL is desired and it's already enabled with different webACL on LB",
			fields: fields{
				webACLAssociationSpecs: []wafv2model.WebACLAssociationSpec{
					{
						WebACLARN:   "web-acl-arn-1",
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getAssociatedWebACLCalls: []getAssociatedWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "web-acl-arn-2",
					},
				},
				associateWebACLCalls: []associateWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "web-acl-arn-1",
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when webACL is desired and it's not enabled on LB",
			fields: fields{
				webACLAssociationSpecs: []wafv2model.WebACLAssociationSpec{
					{
						WebACLARN:   "web-acl-arn-1",
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getAssociatedWebACLCalls: []getAssociatedWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "",
					},
				},
				associateWebACLCalls: []associateWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "web-acl-arn-1",
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when webACL is not desired but it's enabled on LB",
			fields: fields{
				webACLAssociationSpecs: []wafv2model.WebACLAssociationSpec{
					{
						WebACLARN:   "",
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getAssociatedWebACLCalls: []getAssociatedWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "web-acl-arn-1",
					},
				},
				disassociateWebACLCall: []disassociateWebACLCall{
					{
						resourceARN: "some-lb-arn",
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "failed to get webACL association on LB",
			fields: fields{
				webACLAssociationSpecs: []wafv2model.WebACLAssociationSpec{
					{
						WebACLARN:   "web-acl-arn-1",
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getAssociatedWebACLCalls: []getAssociatedWebACLCall{
					{
						resourceARN: "some-lb-arn",
						err:         fmt.Errorf("some error"),
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.EqualError(t, err, "failed to get WAFv2 webACL association on LoadBalancer: some error", msgAndArgs...)
			},
		},
		{
			name: "failed to create webACL association on LB",
			fields: fields{
				webACLAssociationSpecs: []wafv2model.WebACLAssociationSpec{
					{
						WebACLARN:   "web-acl-arn-1",
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getAssociatedWebACLCalls: []getAssociatedWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "",
					},
				},
				associateWebACLCalls: []associateWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "web-acl-arn-1",
						err:         fmt.Errorf("some error"),
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.EqualError(t, err, "failed to create WAFv2 webACL association on LoadBalancer: some error", msgAndArgs...)
			},
		},
		{
			name: "failed to delete webACL association on LB",
			fields: fields{
				webACLAssociationSpecs: []wafv2model.WebACLAssociationSpec{
					{
						WebACLARN:   "",
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getAssociatedWebACLCalls: []getAssociatedWebACLCall{
					{
						resourceARN: "some-lb-arn",
						webACLARN:   "web-acl-arn-1",
					},
				},
				disassociateWebACLCall: []disassociateWebACLCall{
					{
						resourceARN: "some-lb-arn",
						err:         fmt.Errorf("some error"),
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.EqualError(t, err, "failed to delete WAFv2 webACL association on LoadBalancer: some error", msgAndArgs...)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			associationManager := NewMockWebACLAssociationManager(ctrl)
			for _, call := range tt.fields.getAssociatedWebACLCalls {
				associationManager.EXPECT().GetAssociatedWebACL(gomock.Any(), call.resourceARN).Return(call.webACLARN, call.err)
			}
			for _, call := range tt.fields.associateWebACLCalls {
				associationManager.EXPECT().AssociateWebACL(gomock.Any(), call.resourceARN, call.webACLARN).Return(call.err)
			}
			for _, call := range tt.fields.disassociateWebACLCall {
				associationManager.EXPECT().DisassociateWebACL(gomock.Any(), call.resourceARN).Return(call.err)
			}

			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			for idx, spec := range tt.fields.webACLAssociationSpecs {
				wafv2model.NewWebACLAssociation(stack, fmt.Sprintf("%d", idx), spec)
			}
			s := &webACLAssociationSynthesizer{
				associationManager: associationManager,
				logger:             logr.New(&log.NullLogSink{}),
				stack:              stack,
			}
			tt.wantErr(t, s.Synthesize(context.Background()), "Synthesize")
		})
	}
}
