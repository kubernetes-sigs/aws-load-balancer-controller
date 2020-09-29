package networking

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	ec2equality "sigs.k8s.io/aws-load-balancer-controller/pkg/equality/ec2"
)

// configuration options for SecurityGroup Reconcile options.
type SecurityGroupReconcileOptions struct {
	// PermissionSelector defines the selector to identify permissions that should be managed.
	// Permissions that are not managed shouldn't be altered or deleted.
	// By default, it selects every permission.
	PermissionSelector labels.Selector

	// Whether only Authorize permissions.
	// By default, it grants and revoke permission.
	AuthorizeOnly bool
}

// Apply SecurityGroupReconcileOption options
func (opts *SecurityGroupReconcileOptions) ApplyOptions(options ...SecurityGroupReconcileOption) {
	for _, option := range options {
		option(opts)
	}
}

type SecurityGroupReconcileOption func(opts *SecurityGroupReconcileOptions)

// WithPermissionSelector is a option that sets the PermissionSelector.
func WithPermissionSelector(permissionSelector labels.Selector) SecurityGroupReconcileOption {
	return func(opts *SecurityGroupReconcileOptions) {
		opts.PermissionSelector = permissionSelector
	}
}

// WithAuthorizeOnly is a option that sets the AuthorizeOnly.
func WithAuthorizeOnly(authorizeOnly bool) SecurityGroupReconcileOption {
	return func(opts *SecurityGroupReconcileOptions) {
		opts.AuthorizeOnly = authorizeOnly
	}
}

// SecurityGroupReconciler manages securityGroup rules on securityGroup.
type SecurityGroupReconciler interface {
	// ReconcileIngress will reconcile Ingress permission on SecurityGroup to be desiredPermission.
	ReconcileIngress(ctx context.Context, sgID string, desiredPermissions []IPPermissionInfo, opts ...SecurityGroupReconcileOption) error
}

// NewDefaultSecurityGroupReconciler constructs new defaultSecurityGroupReconciler.
func NewDefaultSecurityGroupReconciler(sgManager SecurityGroupManager, logger logr.Logger) *defaultSecurityGroupReconciler {
	return &defaultSecurityGroupReconciler{
		sgManager: sgManager,
		logger:    logger,
	}
}

var _ SecurityGroupReconciler = &defaultSecurityGroupReconciler{}

// default implementation for SecurityGroupReconciler.
type defaultSecurityGroupReconciler struct {
	sgManager SecurityGroupManager
	logger    logr.Logger
}

func (r *defaultSecurityGroupReconciler) ReconcileIngress(ctx context.Context, sgID string, desiredPermissions []IPPermissionInfo, opts ...SecurityGroupReconcileOption) error {
	reconcileOpts := SecurityGroupReconcileOptions{
		PermissionSelector: labels.Everything(),
	}
	reconcileOpts.ApplyOptions(opts...)

	sgInfoByID, err := r.sgManager.FetchSGInfosByID(ctx, []string{sgID})
	if err != nil {
		return err
	}
	sgInfo := sgInfoByID[sgID]
	if err := r.reconcileIngressWithSGInfo(ctx, sgInfo, desiredPermissions, reconcileOpts); err != nil {
		if !r.shouldRetryWithoutCache(err) {
			return err
		}
		sgInfoByID, err := r.sgManager.FetchSGInfosByID(ctx, []string{sgID}, WithReloadIgnoringCache())
		if err != nil {
			return err
		}
		sgInfo := sgInfoByID[sgID]
		if err := r.reconcileIngressWithSGInfo(ctx, sgInfo, desiredPermissions, reconcileOpts); err != nil {
			return err
		}
	}
	return nil
}

func (r *defaultSecurityGroupReconciler) reconcileIngressWithSGInfo(ctx context.Context, sgInfo SecurityGroupInfo, desiredPermissions []IPPermissionInfo, reconcileOpts SecurityGroupReconcileOptions) error {
	extraPermissions := diffIPPermissionInfos(sgInfo.Ingress, desiredPermissions)
	permissionsToRevoke := make([]IPPermissionInfo, 0, len(extraPermissions))
	for _, permission := range extraPermissions {
		if reconcileOpts.PermissionSelector.Matches(labels.Set(permission.Labels)) {
			permissionsToRevoke = append(permissionsToRevoke, permission)
		}
	}
	permissionsToGrant := diffIPPermissionInfos(desiredPermissions, sgInfo.Ingress)
	if len(permissionsToRevoke) > 0 && !reconcileOpts.AuthorizeOnly {
		if err := r.sgManager.RevokeSGIngress(ctx, sgInfo.SecurityGroupID, permissionsToRevoke); err != nil {
			return err
		}
	}
	if len(permissionsToGrant) > 0 {
		if err := r.sgManager.AuthorizeSGIngress(ctx, sgInfo.SecurityGroupID, permissionsToGrant); err != nil {
			return err
		}
	}
	return nil
}

// shouldRetryWithoutCache tests whether we should retry SecurityGroup rules reconcile without cache.
func (r *defaultSecurityGroupReconciler) shouldRetryWithoutCache(err error) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == "InvalidPermission.Duplicate" || awsErr.Code() == "InvalidPermission.NotFound"
	}
	return false
}

// diffIPPermissionInfos calculates set_difference as source - target
func diffIPPermissionInfos(source []IPPermissionInfo, target []IPPermissionInfo) []IPPermissionInfo {
	opts := cmp.Options{
		ec2equality.CompareOptionForIPPermission(),
		cmpopts.IgnoreFields(IPPermissionInfo{}, "Labels"),
	}
	var diffs []IPPermissionInfo
	for _, sPermission := range source {
		containsInTarget := false
		for _, tPermission := range target {
			if cmp.Equal(sPermission, tPermission, opts) {
				containsInTarget = true
				break
			}
		}
		if !containsInTarget {
			diffs = append(diffs, sPermission)
		}
	}
	return diffs
}
