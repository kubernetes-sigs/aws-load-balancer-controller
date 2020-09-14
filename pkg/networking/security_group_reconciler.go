package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/services"
	ec2model "sigs.k8s.io/aws-alb-ingress-controller/pkg/model/ec2"
	"sync"
	"time"
)

const (
	defaultSGInfoCacheTTL = 10 * time.Minute
)

type SecurityGroupReconcileOptions struct {
	// PermissionSelector defines the selector to identify permissions that should be managed.
	// By default, it selects every permission.
	PermissionSelector labels.Selector
}

type SecurityGroupReconcileOption func(opts *SecurityGroupReconcileOptions)

// SecurityGroupReconciler manages securityGroup rules on securityGroup.
type SecurityGroupReconciler interface {
	ReconcileIngress(ctx context.Context, sgID string, permissions []ec2model.IPPermission, opts ...SecurityGroupReconcileOption) error
}

// NewDefaultSecurityGroupReconciler constructs new defaultSecurityGroupReconciler.
func NewDefaultSecurityGroupReconciler(ec2Client services.EC2, logger logr.Logger) *defaultSecurityGroupReconciler {
	return &defaultSecurityGroupReconciler{
		ec2Client:        ec2Client,
		logger:           logger,
		sgInfoCache:      cache.NewExpiring(),
		sgInfoCacheMutex: sync.RWMutex{},
		sgInfoCacheTTL:   defaultSGInfoCacheTTL,
	}
}

var _ SecurityGroupReconciler = &defaultSecurityGroupReconciler{}

// default implementation for SecurityGroupReconciler.
type defaultSecurityGroupReconciler struct {
	ec2Client services.EC2
	logger    logr.Logger

	sgInfoCache      *cache.Expiring
	sgInfoCacheMutex sync.RWMutex
	sgInfoCacheTTL   time.Duration
}

func (r *defaultSecurityGroupReconciler) ReconcileIngress(ctx context.Context, sgID string, permissions []ec2model.IPPermission, opts ...SecurityGroupReconcileOption) error {
	return nil
}

func (r *defaultSecurityGroupReconciler) fetchSGInfosFromAWS(ctx context.Context, sgIDs []string) (map[string]SecurityGroupInfo, error) {
	req := &ec2sdk.DescribeSecurityGroupsInput{
		GroupIds: awssdk.StringSlice(sgIDs),
	}
	sgs, err := r.ec2Client.DescribeSecurityGroupsAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	sgInfoByID := make(map[string]SecurityGroupInfo, len(sgs))
	for _, sg := range sgs {
		sgID := awssdk.StringValue(sg.GroupId)
		sgInfo := buildSecurityGroupInfo(sg)
		sgInfoByID[sgID] = sgInfo
	}
	return sgInfoByID, nil
}
