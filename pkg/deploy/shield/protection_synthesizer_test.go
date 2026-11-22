package shield

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/shield"
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
	type resolveCall struct {
		resourceARN string
		arns        []string
		err         error
	}
	type fields struct {
		protectionSpecs       []shieldmodel.ProtectionSpec
		resolveCalls          []resolveCall
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
		{
			name: "when protection is desired on an NLB, protections are created per EIP allocation ARN",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     true,
						ResourceARN: core.LiteralStringToken("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123"),
					},
				},
				resolveCalls: []resolveCall{
					{
						resourceARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123",
						arns: []string{
							"arn:aws:ec2:us-west-2:123456789012:eip-allocation/eipalloc-1",
							"arn:aws:ec2:us-west-2:123456789012:eip-allocation/eipalloc-2",
						},
					},
				},
				getProtectionCalls: []getProtectionCall{
					{resourceARN: "arn:aws:ec2:us-west-2:123456789012:eip-allocation/eipalloc-1", protectionInfo: nil},
					{resourceARN: "arn:aws:ec2:us-west-2:123456789012:eip-allocation/eipalloc-2", protectionInfo: nil},
				},
				createProtectionCalls: []createProtectionCall{
					{resourceARN: "arn:aws:ec2:us-west-2:123456789012:eip-allocation/eipalloc-1", protectionName: "managed by aws-load-balancer-controller", protectionID: "protection-id-1"},
					{resourceARN: "arn:aws:ec2:us-west-2:123456789012:eip-allocation/eipalloc-2", protectionName: "managed by aws-load-balancer-controller", protectionID: "protection-id-2"},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when protection is enabled on an NLB without EIPs, reconciliation fails",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     true,
						ResourceARN: core.LiteralStringToken("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123"),
					},
				},
				resolveCalls: []resolveCall{
					{
						resourceARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123",
						arns:        nil,
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.EqualError(t, err, "NLB arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123 has no associated Elastic IP addresses; Shield Advanced protects an NLB via its EIP allocation ARNs, associate EIPs with the NLB (e.g. via the aws-load-balancer-eip-allocations annotation) before enabling protection", msgAndArgs...)
			},
		},
		{
			name: "when protection is disabled on an NLB without EIPs, reconciliation is a no-op",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     false,
						ResourceARN: core.LiteralStringToken("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123"),
					},
				},
				resolveCalls: []resolveCall{
					{
						resourceARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123",
						arns:        nil,
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "when resolving NLB EIP ARNs fails",
			fields: fields{
				protectionSpecs: []shieldmodel.ProtectionSpec{
					{
						Enabled:     true,
						ResourceARN: core.LiteralStringToken("arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123"),
					},
				},
				resolveCalls: []resolveCall{
					{
						resourceARN: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/net/my-nlb/abc123",
						err:         fmt.Errorf("some error"),
					},
				},
			},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
				return assert.EqualError(t, err, "failed to resolve shield protection resource ARNs for LoadBalancer: some error", msgAndArgs...)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			protectionManager := NewMockProtectionManager(ctrl)
			if len(tt.fields.resolveCalls) == 0 && len(tt.fields.protectionSpecs) > 0 {
				for _, spec := range tt.fields.protectionSpecs {
					arn, _ := spec.ResourceARN.Resolve(context.Background())
					tt.fields.resolveCalls = append(tt.fields.resolveCalls, resolveCall{resourceARN: arn, arns: []string{arn}})
				}
			}
			for _, call := range tt.fields.resolveCalls {
				protectionManager.EXPECT().ResolveProtectionResourceARNs(gomock.Any(), call.resourceARN).Return(call.arns, call.err)
			}
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
