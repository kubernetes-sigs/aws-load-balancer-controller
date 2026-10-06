package networking

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sync"
)

func newTestSecurityGroupManager(ec2Client services.EC2) *defaultSecurityGroupManager {
	return &defaultSecurityGroupManager{
		ec2Client:        ec2Client,
		logger:           logr.New(&log.NullLogSink{}),
		sgInfoCache:      cache.NewExpiring(),
		sgInfoCacheMutex: sync.RWMutex{},
		sgInfoCacheTTL:   defaultSGInfoCacheTTL,
	}
}

func Test_defaultSecurityGroupManager_AuthorizeSGIngress(t *testing.T) {
	fromPort := awssdk.Int32(80)
	toPort := awssdk.Int32(80)
	permissions := []IPPermissionInfo{
		NewCIDRIPPermission("tcp", fromPort, toPort, "192.168.0.0/16", nil),
	}

	tests := []struct {
		name         string
		authorizeErr error
		wantErr      bool
	}{
		{
			name:    "authorize succeeds",
			wantErr: false,
		},
		{
			name:         "authorize fails and the error is surfaced to the caller",
			authorizeErr: errors.New("InvalidGroup.NotFound: The security group 'sg-invalid' does not exist"),
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ec2Client := services.NewMockEC2(ctrl)
			ec2Client.EXPECT().AuthorizeSecurityGroupIngressWithContext(gomock.Any(), gomock.Any()).
				Return(&ec2sdk.AuthorizeSecurityGroupIngressOutput{}, tt.authorizeErr)

			m := newTestSecurityGroupManager(ec2Client)
			err := m.AuthorizeSGIngress(context.Background(), "sg-xxyy", permissions)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_defaultSecurityGroupManager_RevokeSGIngress(t *testing.T) {
	fromPort := awssdk.Int32(80)
	toPort := awssdk.Int32(80)
	permissions := []IPPermissionInfo{
		NewCIDRIPPermission("tcp", fromPort, toPort, "192.168.0.0/16", nil),
	}

	tests := []struct {
		name      string
		revokeErr error
		wantErr   bool
	}{
		{
			name:    "revoke succeeds",
			wantErr: false,
		},
		{
			name:      "revoke fails and the error is surfaced to the caller",
			revokeErr: errors.New("InvalidGroup.NotFound: The security group 'sg-invalid' does not exist"),
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ec2Client := services.NewMockEC2(ctrl)
			ec2Client.EXPECT().RevokeSecurityGroupIngressWithContext(gomock.Any(), gomock.Any()).
				Return(&ec2sdk.RevokeSecurityGroupIngressOutput{}, tt.revokeErr)

			m := newTestSecurityGroupManager(ec2Client)
			err := m.RevokeSGIngress(context.Background(), "sg-xxyy", permissions)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
