package networking

import (
	"context"
	"errors"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultSecurityGroupReconciler_shouldRetryWithoutCache(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should retry without cache when got duplicated permission error",
			args: args{
				err: &smithy.GenericAPIError{Code: "InvalidPermission.Duplicate", Message: ""},
			},
			want: true,
		},
		{
			name: "should retry without cache when got not found permission error",
			args: args{
				err: &smithy.GenericAPIError{Code: "InvalidPermission.NotFound", Message: ""},
			},
			want: true,
		},
		{
			name: "should retry without cache when got too many rules error",
			args: args{
				err: &smithy.GenericAPIError{Code: "RulesPerSecurityGroupLimitExceeded", Message: ""},
			},
			want: true,
		},
		{
			name: "shouldn't retry when got some other error",
			args: args{
				err: &smithy.GenericAPIError{Code: "SomeOtherError", Message: ""},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &defaultSecurityGroupReconciler{}
			got := r.shouldRetryWithoutCache(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_diffIPPermissionInfos(t *testing.T) {
	type args struct {
		source []IPPermissionInfo
		target []IPPermissionInfo
	}
	tests := []struct {
		name string
		args args
		want []IPPermissionInfo
	}{
		{
			name: "source contains more than target",
			args: args{
				source: []IPPermissionInfo{
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.168.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.171.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.170.0.0/16"),
								},
							},
						},
					},
				},
				target: []IPPermissionInfo{
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.168.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.170.0.0/16"),
								},
							},
						},
					},
				},
			},
			want: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int32(80),
						ToPort:     awssdk.Int32(8080),
						IpRanges: []ec2types.IpRange{
							{
								CidrIp: awssdk.String("192.171.0.0/16"),
							},
						},
					},
				},
			},
		},
		{
			name: "source equals to target",
			args: args{
				source: []IPPermissionInfo{
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.168.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.170.0.0/16"),
								},
							},
						},
					},
				},
				target: []IPPermissionInfo{
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.168.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2types.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int32(80),
							ToPort:     awssdk.Int32(8080),
							IpRanges: []ec2types.IpRange{
								{
									CidrIp: awssdk.String("192.170.0.0/16"),
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "both source & target is nil",
			args: args{
				source: nil,
				target: nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffIPPermissionInfos(tt.args.source, tt.args.target)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReconcileSGIngress(t *testing.T) {
	sgId := "sgId"
	type fetchSGInfosByIDCall struct {
		req  []string
		resp map[string]SecurityGroupInfo
		err  error
	}

	tests := []struct {
		name           string
		inputSGRules   []IPPermissionInfo
		sgGetCall      fetchSGInfosByIDCall
		authorizeData  []IPPermissionInfo
		revokeData     []IPPermissionInfo
		revokeCalls    int
		authorizeCalls int
		authorizeError error
		revokeError    error
		expectErr      bool
	}{
		{
			name:         "no permissions in either ec2 or kube",
			inputSGRules: []IPPermissionInfo{},
			sgGetCall: fetchSGInfosByIDCall{
				req: []string{sgId},
				resp: map[string]SecurityGroupInfo{
					sgId: {
						SecurityGroupID: sgId,
					},
				},
			},
		},
		{
			name:         "sg get failure blocks revoke / authorize",
			inputSGRules: []IPPermissionInfo{},
			sgGetCall: fetchSGInfosByIDCall{
				req: []string{sgId},
				resp: map[string]SecurityGroupInfo{
					sgId: {
						SecurityGroupID: sgId,
					},
				},
				err: errors.New("bad thing"),
			},
			expectErr: true,
		},
		{
			name: "permission present in kube but not ec2 should lead to authorize call",
			inputSGRules: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(10),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			sgGetCall: fetchSGInfosByIDCall{
				req: []string{sgId},
				resp: map[string]SecurityGroupInfo{
					sgId: {
						SecurityGroupID: sgId,
					},
				},
			},
			authorizeData: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(10),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			authorizeCalls: 1,
		},
		{
			name:         "permission present in ec2 but not kube should lead to revoke call",
			inputSGRules: []IPPermissionInfo{},
			sgGetCall: fetchSGInfosByIDCall{
				req: []string{sgId},
				resp: map[string]SecurityGroupInfo{
					sgId: {
						SecurityGroupID: sgId,
						Ingress: []IPPermissionInfo{
							{
								Permission: ec2types.IpPermission{
									FromPort:   awssdk.Int32(10),
									ToPort:     awssdk.Int32(15),
									IpProtocol: awssdk.String("tcp"),
								},
							},
						},
					},
				},
			},
			revokeData: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(10),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			revokeCalls: 1,
		},
		{
			name: "revoke and authorize together",
			inputSGRules: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(12),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			sgGetCall: fetchSGInfosByIDCall{
				req: []string{sgId},
				resp: map[string]SecurityGroupInfo{
					sgId: {
						SecurityGroupID: sgId,
						Ingress: []IPPermissionInfo{
							{
								Permission: ec2types.IpPermission{
									FromPort:   awssdk.Int32(10),
									ToPort:     awssdk.Int32(15),
									IpProtocol: awssdk.String("tcp"),
								},
							},
						},
					},
				},
			},
			authorizeData: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(12),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			authorizeCalls: 1,
			revokeData: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(10),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			revokeCalls: 1,
		},
		{
			name: "authorize error should block revoke call",
			inputSGRules: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(12),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			sgGetCall: fetchSGInfosByIDCall{
				req: []string{sgId},
				resp: map[string]SecurityGroupInfo{
					sgId: {
						SecurityGroupID: sgId,
						Ingress: []IPPermissionInfo{
							{
								Permission: ec2types.IpPermission{
									FromPort:   awssdk.Int32(10),
									ToPort:     awssdk.Int32(15),
									IpProtocol: awssdk.String("tcp"),
								},
							},
						},
					},
				},
			},
			authorizeError: errors.New("authorize error"),
			expectErr:      true,
			authorizeData: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(12),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			authorizeCalls: 1,
		},
		{
			name: "revoke error should not block authorize call",
			inputSGRules: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(12),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			sgGetCall: fetchSGInfosByIDCall{
				req: []string{sgId},
				resp: map[string]SecurityGroupInfo{
					sgId: {
						SecurityGroupID: sgId,
						Ingress: []IPPermissionInfo{
							{
								Permission: ec2types.IpPermission{
									FromPort:   awssdk.Int32(10),
									ToPort:     awssdk.Int32(15),
									IpProtocol: awssdk.String("tcp"),
								},
							},
						},
					},
				},
			},
			revokeError: errors.New("revoke error"),
			expectErr:   true,
			authorizeData: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(12),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			authorizeCalls: 1,
			revokeData: []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(10),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			},
			revokeCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			sgManager := NewMockSecurityGroupManager(ctrl)
			reconciler := &defaultSecurityGroupReconciler{
				sgManager: sgManager,
				logger:    logr.New(&log.NullLogSink{}),
			}

			ctx := context.Background()
			sgManager.EXPECT().FetchSGInfosByID(gomock.Any(), tt.sgGetCall.req, gomock.Any()).Return(tt.sgGetCall.resp, tt.sgGetCall.err)

			sgManager.EXPECT().AuthorizeSGIngress(gomock.Eq(ctx), gomock.Eq(sgId), gomock.Eq(tt.authorizeData)).Return(tt.authorizeError).Times(tt.authorizeCalls)
			sgManager.EXPECT().RevokeSGIngress(gomock.Eq(ctx), gomock.Eq(sgId), gomock.Eq(tt.revokeData)).Return(tt.revokeError).Times(tt.revokeCalls)

			err := reconciler.ReconcileIngress(ctx, sgId, tt.inputSGRules)
			ctrl.Finish()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReconcileSGIngress_RehydrateCache(t *testing.T) {

	testCases := []struct {
		name                  string
		initialAuthorizeError error
		secondAuthorizeError  error
		revokeFirst           bool
	}{
		{
			name:                  "not found error leads to cache re-hydrate",
			initialAuthorizeError: &smithy.GenericAPIError{Code: "InvalidPermission.NotFound", Message: ""},
			revokeFirst:           false,
		},
		{
			name:                  "too many rules error leads to cache re-hydrate and inverse operations",
			initialAuthorizeError: &smithy.GenericAPIError{Code: "RulesPerSecurityGroupLimitExceeded", Message: ""},
			revokeFirst:           true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			sgId := "sgId"
			ctrl := gomock.NewController(t)

			sgManager := NewMockSecurityGroupManager(ctrl)
			reconciler := &defaultSecurityGroupReconciler{
				sgManager: sgManager,
				logger:    logr.New(&log.NullLogSink{}),
			}

			ctx := context.Background()

			sgManager.EXPECT().FetchSGInfosByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(map[string]SecurityGroupInfo{
				sgId: {
					SecurityGroupID: sgId,
					Ingress: []IPPermissionInfo{
						{
							Permission: ec2types.IpPermission{
								FromPort:   awssdk.Int32(10),
								ToPort:     awssdk.Int32(15),
								IpProtocol: awssdk.String("tcp"),
							},
						},
					},
				},
			}, nil)

			revokeData := []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(10),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			}

			inputSGRules := []IPPermissionInfo{
				{
					Permission: ec2types.IpPermission{
						FromPort:   awssdk.Int32(12),
						ToPort:     awssdk.Int32(15),
						IpProtocol: awssdk.String("tcp"),
					},
				},
			}

			if tt.revokeFirst {
				gomock.InOrder(
					sgManager.EXPECT().AuthorizeSGIngress(gomock.Eq(ctx), gomock.Eq(sgId), gomock.Eq(inputSGRules)).Times(1).Return(tt.initialAuthorizeError),
					sgManager.EXPECT().RevokeSGIngress(gomock.Eq(ctx), gomock.Eq(sgId), gomock.Eq(revokeData)).Return(nil).Times(1),
					sgManager.EXPECT().AuthorizeSGIngress(gomock.Eq(ctx), gomock.Eq(sgId), gomock.Eq(inputSGRules)).Times(1).Return(tt.secondAuthorizeError),
				)
			} else {
				gomock.InOrder(
					sgManager.EXPECT().AuthorizeSGIngress(gomock.Eq(ctx), gomock.Eq(sgId), gomock.Eq(inputSGRules)).Times(1).Return(tt.initialAuthorizeError),
					sgManager.EXPECT().AuthorizeSGIngress(gomock.Eq(ctx), gomock.Eq(sgId), gomock.Eq(inputSGRules)).Times(1).Return(tt.secondAuthorizeError),
					sgManager.EXPECT().RevokeSGIngress(gomock.Eq(ctx), gomock.Eq(sgId), gomock.Eq(revokeData)).Return(nil).Times(1),
				)
			}

			err := reconciler.ReconcileIngress(ctx, sgId, inputSGRules)
			ctrl.Finish()
			assert.NoError(t, err)
		})
	}
}
