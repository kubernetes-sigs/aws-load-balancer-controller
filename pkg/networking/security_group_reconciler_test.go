package networking

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

// fakeStatefulSGManager simulates the EC2-side SecurityGroup rule state for concurrency tests.
// It tracks how many manager calls run concurrently and whether a revoke ever left the
// SecurityGroup without any rules.
type fakeStatefulSGManager struct {
	sgID string

	mu    sync.Mutex
	perms map[string]IPPermissionInfo

	inFlight    int32
	maxInFlight int32
	wentEmpty   int32
}

// enter tracks call concurrency and sleeps briefly to widen any interleaving window.
func (f *fakeStatefulSGManager) enter() {
	cur := atomic.AddInt32(&f.inFlight, 1)
	for {
		max := atomic.LoadInt32(&f.maxInFlight)
		if cur <= max || atomic.CompareAndSwapInt32(&f.maxInFlight, max, cur) {
			break
		}
	}
	time.Sleep(time.Millisecond)
}

func (f *fakeStatefulSGManager) exit() {
	atomic.AddInt32(&f.inFlight, -1)
}

func (f *fakeStatefulSGManager) FetchSGInfosByID(ctx context.Context, sgIDs []string, opts ...FetchSGInfoOption) (map[string]SecurityGroupInfo, error) {
	f.enter()
	defer f.exit()
	f.mu.Lock()
	defer f.mu.Unlock()
	ingress := make([]IPPermissionInfo, 0, len(f.perms))
	for _, perm := range f.perms {
		ingress = append(ingress, perm)
	}
	return map[string]SecurityGroupInfo{
		f.sgID: {
			SecurityGroupID: f.sgID,
			Ingress:         ingress,
		},
	}, nil
}

func (f *fakeStatefulSGManager) FetchSGInfosByRequest(ctx context.Context, req *ec2sdk.DescribeSecurityGroupsInput) (map[string]SecurityGroupInfo, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeStatefulSGManager) AuthorizeSGIngress(ctx context.Context, sgID string, permissions []IPPermissionInfo) error {
	f.enter()
	defer f.exit()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, perm := range permissions {
		f.perms[perm.HashCode()] = perm
	}
	return nil
}

func (f *fakeStatefulSGManager) RevokeSGIngress(ctx context.Context, sgID string, permissions []IPPermissionInfo) error {
	f.enter()
	defer f.exit()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, perm := range permissions {
		delete(f.perms, perm.HashCode())
	}
	if len(f.perms) == 0 {
		atomic.StoreInt32(&f.wentEmpty, 1)
	}
	return nil
}

// TestReconcileSGIngress_ConcurrentReconcilesAreSerializedPerSG is a regression test for
// concurrent reconciles of the same SecurityGroup revoking rules without granting replacements.
// Endpoint churn on the TargetGroupBinding that defines the boundary of the aggregated
// port-range produces alternating desired permission sets reconciled concurrently; a reconcile
// diffing against a snapshot taken mid-way through another reconcile's authorize/revoke
// sequence can revoke the last remaining rule and leave the SecurityGroup empty.
func TestReconcileSGIngress_ConcurrentReconcilesAreSerializedPerSG(t *testing.T) {
	sgID := "sg-cluster"
	wideRule := NewGroupIDIPPermission("tcp", awssdk.Int32(3000), awssdk.Int32(8025), "sg-backend", nil)
	narrowRule := NewGroupIDIPPermission("tcp", awssdk.Int32(3000), awssdk.Int32(3008), "sg-backend", nil)

	f := &fakeStatefulSGManager{
		sgID:  sgID,
		perms: map[string]IPPermissionInfo{wideRule.HashCode(): wideRule},
	}
	reconciler := &defaultSecurityGroupReconciler{
		sgManager: f,
		logger:    logr.New(&log.NullLogSink{}),
	}

	const reconcileCount = 10
	var wg sync.WaitGroup
	errs := make([]error, reconcileCount)
	for i := 0; i < reconcileCount; i++ {
		desired := []IPPermissionInfo{wideRule}
		if i%2 == 1 {
			desired = []IPPermissionInfo{narrowRule}
		}
		wg.Add(1)
		go func(i int, desired []IPPermissionInfo) {
			defer wg.Done()
			errs[i] = reconciler.ReconcileIngress(context.Background(), sgID, desired)
		}(i, desired)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "reconcile %d", i)
	}
	assert.EqualValues(t, 1, atomic.LoadInt32(&f.maxInFlight),
		"concurrent reconciles of the same SecurityGroup must not interleave fetch/authorize/revoke calls")
	assert.EqualValues(t, 0, atomic.LoadInt32(&f.wentEmpty),
		"the managed permissions must never be fully revoked while a non-empty permission set is desired")
	assert.Len(t, f.perms, 1, "the final state must converge to exactly one of the desired permission sets")
}

// TestReconcileSGIngress_LocksAreScopedPerSG verifies reconciles of different SecurityGroups
// do not serialize against each other.
func TestReconcileSGIngress_LocksAreScopedPerSG(t *testing.T) {
	reconciler := &defaultSecurityGroupReconciler{}

	unlockA := reconciler.lockSG("sg-a")
	// must not block while sg-a is held; a shared lock would deadlock the test here.
	unlockB := reconciler.lockSG("sg-b")
	unlockB()

	// re-acquiring sg-a must block until it is released.
	acquired := make(chan struct{})
	go func() {
		unlock := reconciler.lockSG("sg-a")
		unlock()
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("acquiring the lock for a SecurityGroup that is already locked must block")
	case <-time.After(50 * time.Millisecond):
	}
	unlockA()
	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("the lock for a SecurityGroup must be acquirable after it is released")
	}
}
