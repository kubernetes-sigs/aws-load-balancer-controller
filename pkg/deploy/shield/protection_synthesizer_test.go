package shield

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/shield"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_protectionSynthesizer_Synthesize(t *testing.T) {
	type getProtectionCall struct {
		resourceARN    string
		protectionInfo *ProtectionInfo
		err            error
	}
	type createProtectionCall struct {
		resourceARN    string
		protectionName string
		protectionID   string
		err            error
	}
	type deleteProtectionCall struct {
		resourceARN  string
		protectionID string
		err          error
	}
	type fields struct {
		protectionSpecs       []shieldmodel.ProtectionSpec
		getProtectionCalls    []getProtectionCall
		createProtectionCalls []createProtectionCall
		deleteProtectionCalls []deleteProtectionCall
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "when there is no protection resource",
			fields: fields{
				protectionSpecs: nil,
			},
			wantErr: assert.NoError,
		},
		{
			name: "when protection is desired and it's already enabled in LB",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     true,
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getProtectionCalls: []getProtectionCall{
					{
						resourceARN: "some-lb-arn",
						protectionInfo: &ProtectionInfo{
							Name: "managed by aws-load-balancer-controller",
							ID:   "some-protection-id",
						},
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when protection is desired and it's not enabled in LB",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     true,
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getProtectionCalls: []getProtectionCall{
					{
						resourceARN:    "some-lb-arn",
						protectionInfo: nil,
					},
				},
				createProtectionCalls: []createProtectionCall{
					{
						resourceARN:    "some-lb-arn",
						protectionName: "managed by aws-load-balancer-controller",
						protectionID:   "some-protection-id",
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when protection is not desired and it's already enabled in LB and managed by LBC",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     false,
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getProtectionCalls: []getProtectionCall{
					{
						resourceARN: "some-lb-arn",
						protectionInfo: &ProtectionInfo{
							Name: "managed by aws-load-balancer-controller",
							ID:   "some-protection-id",
						},
					},
				},
				deleteProtectionCalls: []deleteProtectionCall{
					{
						resourceARN:  "some-lb-arn",
						protectionID: "some-protection-id",
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when protection is not desired and it's already enabled in LB but not managed by LBC",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     false,
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getProtectionCalls: []getProtectionCall{
					{
						resourceARN: "some-lb-arn",
						protectionInfo: &ProtectionInfo{
							Name: "some other name",
							ID:   "some-protection-id",
						},
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when failed to get protection",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     true,
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getProtectionCalls: []getProtectionCall{
					{
						resourceARN: "some-lb-arn",
						err:         fmt.Errorf("some error"),
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.EqualError(t, err, "failed to get shield protection on LoadBalancer: some error", msgAndArgs...)
			},
		},
		{
			name: "when failed to create protection",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     true,
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getProtectionCalls: []getProtectionCall{
					{
						resourceARN:    "some-lb-arn",
						protectionInfo: nil,
					},
				},
				createProtectionCalls: []createProtectionCall{
					{
						resourceARN:    "some-lb-arn",
						protectionName: "managed by aws-load-balancer-controller",
						err:            fmt.Errorf("some error"),
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.EqualError(t, err, "failed to create shield protection on LoadBalancer: some error", msgAndArgs...)
			},
		},
		{
			name: "when failed to delete protection",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     false,
						ResourceARN: core.LiteralStringToken("some-lb-arn"),
					},
				},
				getProtectionCalls: []getProtectionCall{
					{
						resourceARN: "some-lb-arn",
						protectionInfo: &ProtectionInfo{
							Name: "managed by aws-load-balancer-controller",
							ID:   "some-protection-id",
						},
					},
				},
				deleteProtectionCalls: []deleteProtectionCall{
					{
						resourceARN:  "some-lb-arn",
						protectionID: "some-protection-id",
						err:          fmt.Errorf("some error"),
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.EqualError(t, err, "failed to delete shield protection on LoadBalancer: some error", msgAndArgs...)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			protectionManager := NewMockProtectionManager(ctrl)
			for _, call := range tt.fields.getProtectionCalls {
				protectionManager.EXPECT().GetProtection(gomock.Any(), call.resourceARN).Return(call.protectionInfo, call.err)
			}
			for _, call := range tt.fields.createProtectionCalls {
				protectionManager.EXPECT().CreateProtection(gomock.Any(), call.resourceARN, call.protectionName).Return(call.protectionID, call.err)
			}
			for _, call := range tt.fields.deleteProtectionCalls {
				protectionManager.EXPECT().DeleteProtection(gomock.Any(), call.resourceARN, call.protectionID).Return(call.err)
			}

			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			for idx, spec := range tt.fields.protectionSpecs {
				shieldmodel.NewProtection(stack, fmt.Sprintf("%d", idx), spec)
			}
			s := &protectionSynthesizer{
				protectionManager: protectionManager,
				logger:            logr.New(&log.NullLogSink{}),
				stack:             stack,
			}
			tt.wantErr(t, s.Synthesize(context.Background()), "Synthesize")
		})
	}
}
