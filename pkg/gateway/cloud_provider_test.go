package gateway

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/certs"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// fakeCloud implements services.Cloud for testing without needing the full aws package.
type fakeCloud struct {
	region string
	vpcID  string
}

func (c *fakeCloud) Region() string                   { return c.region }
func (c *fakeCloud) VpcID() string                    { return c.vpcID }
func (c *fakeCloud) ELBV2() services.ELBV2            { return nil }
func (c *fakeCloud) EC2() services.EC2                { return nil }
func (c *fakeCloud) ACM() services.ACM                { return nil }
func (c *fakeCloud) WAFv2() services.WAFv2            { return nil }
func (c *fakeCloud) WAFRegional() services.WAFRegional { return nil }
func (c *fakeCloud) Shield() services.Shield          { return nil }
func (c *fakeCloud) RGT() services.RGT                { return nil }
func (c *fakeCloud) GlobalAccelerator() services.GlobalAccelerator { return nil }
func (c *fakeCloud) GetAssumedRoleELBV2(_ context.Context, _ string, _ string) (services.ELBV2, error) {
	return nil, errors.New("not supported")
}

var _ services.Cloud = &fakeCloud{}

// --- ReconcileContext getter tests ---

func TestReconcileContext_Getters(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cloud := &fakeCloud{region: "us-east-1", vpcID: "vpc-111"}
	subnetsResolver := networking.NewMockSubnetsResolver(ctrl)
	vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)

	rc := &ReconcileContext{
		Cloud:           cloud,
		SubnetsResolver: subnetsResolver,
		VPCInfoProvider: vpcInfoProvider,
	}

	assert.Equal(t, cloud, rc.GetCloud())
	assert.Equal(t, subnetsResolver, rc.GetSubnetsResolver())
	assert.Equal(t, vpcInfoProvider, rc.GetVPCInfoProvider())
	assert.Nil(t, rc.GetElbv2TaggingManager())
	assert.Nil(t, rc.GetBackendSGProvider())
	assert.Nil(t, rc.GetSecurityGroupResolver())
	assert.Nil(t, rc.GetCertDiscovery())
	assert.Nil(t, rc.GetTargetGroupARNMapper())
	assert.False(t, rc.IsCrossRegion())
}

func TestReconcileContext_NonDefaultRegionGetters(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cloud := &fakeCloud{region: "ap-northeast-1", vpcID: "vpc-222"}
	subnetsResolver := networking.NewMockSubnetsResolver(ctrl)
	vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)

	type fakeTaggingManager struct{ elbv2deploy.TaggingManager }
	type fakeBackendSGProvider struct{ networking.BackendSGProvider }
	type fakeSGResolver struct{ networking.SecurityGroupResolver }
	type fakeCertDiscovery struct{ certs.CertDiscovery }
	type fakeTGMapper struct{ shared_utils.TargetGroupARNMapper }

	taggingMgr := &fakeTaggingManager{}
	backendSG := &fakeBackendSGProvider{}
	sgResolver := &fakeSGResolver{}
	certDisc := &fakeCertDiscovery{}
	tgMapper := &fakeTGMapper{}

	rc := &ReconcileContext{
		Cloud:                 cloud,
		SubnetsResolver:       subnetsResolver,
		VPCInfoProvider:       vpcInfoProvider,
		Elbv2TaggingManager:   taggingMgr,
		BackendSGProvider:     backendSG,
		SecurityGroupResolver: sgResolver,
		CertDiscovery:         certDisc,
		TargetGroupARNMapper:  tgMapper,
	}

	assert.Equal(t, cloud, rc.GetCloud())
	assert.Equal(t, subnetsResolver, rc.GetSubnetsResolver())
	assert.Equal(t, vpcInfoProvider, rc.GetVPCInfoProvider())
	assert.Equal(t, taggingMgr, rc.GetElbv2TaggingManager())
	assert.Equal(t, backendSG, rc.GetBackendSGProvider())
	assert.Equal(t, sgResolver, rc.GetSecurityGroupResolver())
	assert.Equal(t, certDisc, rc.GetCertDiscovery())
	assert.Equal(t, tgMapper, rc.GetTargetGroupARNMapper())
	assert.False(t, rc.IsCrossRegion())
}

func TestReconcileContext_IsCrossRegion(t *testing.T) {
	defaultRC := &ReconcileContext{
		Cloud: &fakeCloud{region: "us-east-1"},
	}
	assert.False(t, defaultRC.IsCrossRegion())

	crossRegionRC := &ReconcileContext{
		Cloud:       &fakeCloud{region: "ap-northeast-1"},
		crossRegion: true,
	}
	assert.True(t, crossRegionRC.IsCrossRegion())
}

// --- GetReconcileContext default region tests ---

func TestGetReconcileContext_DefaultRegion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cloud := &fakeCloud{region: "us-east-1", vpcID: "vpc-default"}
	subnetsResolver := networking.NewMockSubnetsResolver(ctrl)
	vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)

	provider := &defaultCloudProvider{
		defaultCloud:           cloud,
		defaultSubnetsResolver: subnetsResolver,
		defaultVPCInfoProvider: vpcInfoProvider,
		logger:                 logr.New(&log.NullLogSink{}),
		cache:                  make(map[string]*ReconcileContext),
	}

	rc, err := provider.GetReconcileContext(context.Background(), "us-east-1", nil)
	assert.NoError(t, err)
	assert.Equal(t, cloud, rc.Cloud)
	assert.Equal(t, subnetsResolver, rc.SubnetsResolver)
	assert.Equal(t, vpcInfoProvider, rc.VPCInfoProvider)
	assert.Nil(t, rc.Elbv2TaggingManager)
	assert.Nil(t, rc.BackendSGProvider)
	assert.Nil(t, rc.SecurityGroupResolver)
	assert.Nil(t, rc.CertDiscovery)
	assert.Nil(t, rc.TargetGroupARNMapper)
	assert.False(t, rc.IsCrossRegion())
}

func TestGetReconcileContext_EmptyRegion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cloud := &fakeCloud{region: "us-east-1", vpcID: "vpc-default"}
	subnetsResolver := networking.NewMockSubnetsResolver(ctrl)
	vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)

	provider := &defaultCloudProvider{
		defaultCloud:           cloud,
		defaultSubnetsResolver: subnetsResolver,
		defaultVPCInfoProvider: vpcInfoProvider,
		logger:                 logr.New(&log.NullLogSink{}),
		cache:                  make(map[string]*ReconcileContext),
	}

	rc, err := provider.GetReconcileContext(context.Background(), "", nil)
	assert.NoError(t, err)
	assert.Equal(t, cloud, rc.Cloud)
	assert.False(t, rc.IsCrossRegion())
}

// --- resolveVPCForRegion tests ---

func TestResolveVPCForRegion_NilSpec(t *testing.T) {
	provider := &defaultCloudProvider{
		logger: logr.New(&log.NullLogSink{}),
	}

	_, err := provider.resolveVPCForRegion(context.Background(), "ap-northeast-1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "set vpcId, vpcSelector, or loadBalancerSubnets")
}

func TestResolveVPCForRegion_ExplicitVpcID(t *testing.T) {
	provider := &defaultCloudProvider{
		logger: logr.New(&log.NullLogSink{}),
	}

	spec := &elbv2gw.LoadBalancerConfigurationSpec{
		VpcID: awssdk.String("vpc-explicit"),
	}
	vpcID, err := provider.resolveVPCForRegion(context.Background(), "ap-northeast-1", spec)
	assert.NoError(t, err)
	assert.Equal(t, "vpc-explicit", vpcID)
}

func TestResolveVPCForRegion_EmptyVpcID_NoSelectorNoSubnets(t *testing.T) {
	provider := &defaultCloudProvider{
		logger: logr.New(&log.NullLogSink{}),
	}

	spec := &elbv2gw.LoadBalancerConfigurationSpec{
		VpcID: awssdk.String(""),
	}
	_, err := provider.resolveVPCForRegion(context.Background(), "ap-northeast-1", spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "set vpcId, vpcSelector, or loadBalancerSubnets")
}

func TestResolveVPCForRegion_EmptySpec(t *testing.T) {
	provider := &defaultCloudProvider{
		logger: logr.New(&log.NullLogSink{}),
	}

	spec := &elbv2gw.LoadBalancerConfigurationSpec{}
	_, err := provider.resolveVPCForRegion(context.Background(), "eu-west-1", spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "set vpcId, vpcSelector, or loadBalancerSubnets")
}

// --- resolveVPCFromSelector tests ---

func TestResolveVPCFromSelector_SingleMatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *ec2sdk.DescribeVpcsInput) ([]ec2types.Vpc, error) {
			assert.Len(t, input.Filters, 1)
			assert.Equal(t, "tag:env", *input.Filters[0].Name)
			assert.Equal(t, []string{"production"}, input.Filters[0].Values)
			return []ec2types.Vpc{
				{VpcId: awssdk.String("vpc-matched")},
			}, nil
		},
	)

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	vpcID, err := provider.resolveVPCFromSelector(context.Background(), ec2Client, map[string][]string{
		"env": {"production"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "vpc-matched", vpcID)
}

func TestResolveVPCFromSelector_NoMatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).Return([]ec2types.Vpc{}, nil)

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	_, err := provider.resolveVPCFromSelector(context.Background(), ec2Client, map[string][]string{
		"env": {"staging"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no VPC found matching vpcSelector")
}

func TestResolveVPCFromSelector_MultipleMatches(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).Return([]ec2types.Vpc{
		{VpcId: awssdk.String("vpc-1")},
		{VpcId: awssdk.String("vpc-2")},
	}, nil)

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	_, err := provider.resolveVPCFromSelector(context.Background(), ec2Client, map[string][]string{
		"env": {"production"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multiple VPCs (2) found")
}

func TestResolveVPCFromSelector_EmptySelector(t *testing.T) {
	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	_, err := provider.resolveVPCFromSelector(context.Background(), nil, map[string][]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one tag key with values")
}

func TestResolveVPCFromSelector_SelectorWithEmptyValues(t *testing.T) {
	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	_, err := provider.resolveVPCFromSelector(context.Background(), nil, map[string][]string{
		"env": {},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one tag key with values")
}

func TestResolveVPCFromSelector_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	_, err := provider.resolveVPCFromSelector(context.Background(), ec2Client, map[string][]string{
		"env": {"prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "throttled")
}

func TestResolveVPCFromSelector_MultipleFilters(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *ec2sdk.DescribeVpcsInput) ([]ec2types.Vpc, error) {
			assert.Len(t, input.Filters, 2)
			return []ec2types.Vpc{
				{VpcId: awssdk.String("vpc-multi-tag")},
			}, nil
		},
	)

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	vpcID, err := provider.resolveVPCFromSelector(context.Background(), ec2Client, map[string][]string{
		"env":     {"production"},
		"cluster": {"main"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "vpc-multi-tag", vpcID)
}

// --- resolveVPCFromFirstSubnet tests ---

func TestResolveVPCFromFirstSubnet_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *ec2sdk.DescribeSubnetsInput) ([]ec2types.Subnet, error) {
			assert.Equal(t, []string{"subnet-abc123"}, input.SubnetIds)
			return []ec2types.Subnet{
				{
					SubnetId: awssdk.String("subnet-abc123"),
					VpcId:    awssdk.String("vpc-from-subnet"),
				},
			}, nil
		},
	)

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	vpcID, err := provider.resolveVPCFromFirstSubnet(context.Background(), ec2Client, "subnet-abc123")
	assert.NoError(t, err)
	assert.Equal(t, "vpc-from-subnet", vpcID)
}

func TestResolveVPCFromFirstSubnet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), gomock.Any()).Return([]ec2types.Subnet{}, nil)

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	_, err := provider.resolveVPCFromFirstSubnet(context.Background(), ec2Client, "subnet-missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in target region")
}

func TestResolveVPCFromFirstSubnet_NilVpcID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), gomock.Any()).Return([]ec2types.Subnet{
		{SubnetId: awssdk.String("subnet-no-vpc"), VpcId: nil},
	}, nil)

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	_, err := provider.resolveVPCFromFirstSubnet(context.Background(), ec2Client, "subnet-no-vpc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no VpcId")
}

func TestResolveVPCFromFirstSubnet_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2Client := services.NewMockEC2(ctrl)
	ec2Client.EXPECT().DescribeSubnetsAsList(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	provider := &defaultCloudProvider{logger: logr.New(&log.NullLogSink{})}
	_, err := provider.resolveVPCFromFirstSubnet(context.Background(), ec2Client, "subnet-denied")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}
