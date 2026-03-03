package gateway

import (
	"context"
	"fmt"
	"sync"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	awspkg "sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/certs"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	awsmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileContext holds the Cloud and region-specific resolvers for a gateway reconcile.
// For the default region it wraps the default cloud and resolvers with optional fields nil.
// For non-default regions all fields are populated with region-scoped implementations.
type ReconcileContext struct {
	Cloud               services.Cloud
	SubnetsResolver     networking.SubnetsResolver
	VPCInfoProvider     networking.VPCInfoProvider
	Elbv2TaggingManager elbv2deploy.TaggingManager
	BackendSGProvider   networking.BackendSGProvider
	SecurityGroupResolver networking.SecurityGroupResolver
	CertDiscovery         certs.CertDiscovery
	TargetGroupARNMapper  shared_utils.TargetGroupARNMapper
	crossRegion           bool
}

// GetCloud returns the Cloud for this context.
func (r *ReconcileContext) GetCloud() services.Cloud { return r.Cloud }

// GetSubnetsResolver returns the SubnetsResolver for this context.
func (r *ReconcileContext) GetSubnetsResolver() networking.SubnetsResolver { return r.SubnetsResolver }

// GetVPCInfoProvider returns the VPCInfoProvider for this context.
func (r *ReconcileContext) GetVPCInfoProvider() networking.VPCInfoProvider { return r.VPCInfoProvider }

// GetElbv2TaggingManager returns the ELBV2 tagging manager for this context, or nil to use the default.
func (r *ReconcileContext) GetElbv2TaggingManager() elbv2deploy.TaggingManager {
	return r.Elbv2TaggingManager
}

// GetBackendSGProvider returns the BackendSGProvider for this context, or nil to use the default (default region).
func (r *ReconcileContext) GetBackendSGProvider() networking.BackendSGProvider {
	return r.BackendSGProvider
}

// GetSecurityGroupResolver returns the SecurityGroupResolver for this context, or nil to use the default.
func (r *ReconcileContext) GetSecurityGroupResolver() networking.SecurityGroupResolver {
	return r.SecurityGroupResolver
}

// GetCertDiscovery returns the CertDiscovery for this context, or nil to use the default (default region's ACM).
func (r *ReconcileContext) GetCertDiscovery() certs.CertDiscovery { return r.CertDiscovery }

// GetTargetGroupARNMapper returns the TargetGroupARNMapper for this context, or nil to use the default.
func (r *ReconcileContext) GetTargetGroupARNMapper() shared_utils.TargetGroupARNMapper {
	return r.TargetGroupARNMapper
}

// IsCrossRegion returns true when the gateway targets a region different from the controller's default.
func (r *ReconcileContext) IsCrossRegion() bool { return r.crossRegion }

// CloudProvider returns a ReconcileContext for a given region and optional LoadBalancerConfiguration spec.
// For the default region it returns the default context; for other regions it resolves VPC and creates (or caches) a Cloud and resolvers.
type CloudProvider interface {
	GetReconcileContext(ctx context.Context, region string, spec *elbv2gw.LoadBalancerConfigurationSpec) (*ReconcileContext, error)
}

// NewDefaultCloudProvider returns a CloudProvider that uses the default cloud for the default region
// and creates Clouds for other regions with VPC resolution from spec (vpcId, vpcSelector, or first subnet).
// k8sClient is used to create region-scoped BackendSGProvider for non-default regions.
func NewDefaultCloudProvider(
	defaultCloud services.Cloud,
	defaultSubnetsResolver networking.SubnetsResolver,
	defaultVPCInfoProvider networking.VPCInfoProvider,
	baseConfig awspkg.CloudConfig,
	controllerConfig config.ControllerConfig,
	k8sClient client.Client,
	metricsCollector *awsmetrics.Collector,
	logger logr.Logger,
) CloudProvider {
	return &defaultCloudProvider{
		defaultCloud:           defaultCloud,
		defaultSubnetsResolver: defaultSubnetsResolver,
		defaultVPCInfoProvider: defaultVPCInfoProvider,
		baseConfig:             baseConfig,
		controllerConfig:       controllerConfig,
		k8sClient:              k8sClient,
		metricsCollector:       metricsCollector,
		logger:                 logger,
		cache:                  make(map[string]*ReconcileContext),
	}
}

var _ CloudProvider = &defaultCloudProvider{}

type defaultCloudProvider struct {
	defaultCloud           services.Cloud
	defaultSubnetsResolver networking.SubnetsResolver
	defaultVPCInfoProvider networking.VPCInfoProvider
	baseConfig             awspkg.CloudConfig
	controllerConfig       config.ControllerConfig
	k8sClient              client.Client
	metricsCollector       *awsmetrics.Collector
	logger                 logr.Logger
	mu                     sync.RWMutex
	cache                  map[string]*ReconcileContext
}

func (p *defaultCloudProvider) GetReconcileContext(ctx context.Context, region string, spec *elbv2gw.LoadBalancerConfigurationSpec) (*ReconcileContext, error) {
	defaultRegion := p.defaultCloud.Region()
	if region == "" || region == defaultRegion {
		return &ReconcileContext{
			Cloud:           p.defaultCloud,
			SubnetsResolver: p.defaultSubnetsResolver,
			VPCInfoProvider: p.defaultVPCInfoProvider,
		}, nil
	}

	// Resolve VPC for the target region
	vpcID, err := p.resolveVPCForRegion(ctx, region, spec)
	if err != nil {
		return nil, err
	}

	cacheKey := region + ":" + vpcID
	p.mu.RLock()
	if ctx, ok := p.cache[cacheKey]; ok {
		p.mu.RUnlock()
		return ctx, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if ctx, ok := p.cache[cacheKey]; ok {
		return ctx, nil
	}

	cloud, err := awspkg.NewCloudForRegion(p.baseConfig, region, vpcID, p.controllerConfig.ClusterName, p.metricsCollector, p.logger, awspkg.DefaultLbStabilizationTime)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create cloud for region %q vpc %q", region, vpcID)
	}

	azInfoProvider := networking.NewDefaultAZInfoProvider(cloud.EC2(), p.logger.WithName("az-info-provider"))
	vpcInfoProvider := networking.NewDefaultVPCInfoProvider(cloud.EC2(), p.logger.WithName("vpc-info-provider"))
	subnetsResolver := networking.NewDefaultSubnetsResolver(
		azInfoProvider,
		cloud.EC2(),
		cloud.VpcID(),
		p.controllerConfig.ClusterName,
		p.controllerConfig.FeatureGates.Enabled(config.SubnetsClusterTagCheck),
		p.controllerConfig.FeatureGates.Enabled(config.ALBSingleSubnet),
		p.controllerConfig.FeatureGates.Enabled(config.SubnetDiscoveryByReachability),
		p.logger.WithName("subnets-resolver"),
	)

	elbv2TaggingManager := elbv2deploy.NewDefaultTaggingManager(cloud.ELBV2(), cloud.VpcID(), p.controllerConfig.FeatureGates, cloud.RGT(), p.logger.WithName("elbv2-tagging"))
	enableGatewayCheck := p.controllerConfig.FeatureGates.Enabled(config.NLBGatewayAPI) || p.controllerConfig.FeatureGates.Enabled(config.ALBGatewayAPI)
	backendSGProvider := networking.NewBackendSGProvider(
		p.controllerConfig.ClusterName,
		p.controllerConfig.BackendSecurityGroup,
		cloud.VpcID(),
		cloud.EC2(),
		p.k8sClient,
		p.controllerConfig.DefaultTags,
		enableGatewayCheck,
		p.logger.WithName("backend-sg-provider").WithName(region),
	)
	sgResolver := networking.NewDefaultSecurityGroupResolver(cloud.EC2(), cloud.VpcID())
	certDiscovery := certs.NewACMCertDiscovery(cloud.ACM(), p.controllerConfig.IngressConfig.AllowedCertificateAuthorityARNs, p.logger.WithName("cert-discovery").WithName(region))
	tgARNMapper := shared_utils.NewTargetGroupNameToArnMapper(cloud.ELBV2())
	reconcileCtx := &ReconcileContext{
		Cloud:                 cloud,
		SubnetsResolver:       subnetsResolver,
		VPCInfoProvider:       vpcInfoProvider,
		Elbv2TaggingManager:   elbv2TaggingManager,
		BackendSGProvider:     backendSGProvider,
		SecurityGroupResolver: sgResolver,
		CertDiscovery:         certDiscovery,
		TargetGroupARNMapper:  tgARNMapper,
		crossRegion:           true,
	}
	p.cache[cacheKey] = reconcileCtx
	return reconcileCtx, nil
}

// resolveVPCForRegion resolves the VPC ID for the target region from spec (vpcId, vpcSelector, or first subnet).
func (p *defaultCloudProvider) resolveVPCForRegion(ctx context.Context, region string, spec *elbv2gw.LoadBalancerConfigurationSpec) (string, error) {
	if spec == nil {
		return "", errors.New("when region differs from controller default, set vpcId, vpcSelector, or loadBalancerSubnets with identifiers in LoadBalancerConfiguration")
	}

	if spec.VpcID != nil && *spec.VpcID != "" {
		return *spec.VpcID, nil
	}

	ec2Client, err := awspkg.NewEC2ClientForRegion(p.baseConfig, region, p.metricsCollector, p.logger)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create EC2 client for region %q", region)
	}

	if spec.VpcSelector != nil && len(*spec.VpcSelector) > 0 {
		vpcID, err := p.resolveVPCFromSelector(ctx, ec2Client, *spec.VpcSelector)
		if err != nil {
			return "", err
		}
		return vpcID, nil
	}

	if spec.LoadBalancerSubnets != nil && len(*spec.LoadBalancerSubnets) > 0 {
		first := (*spec.LoadBalancerSubnets)[0]
		if first.Identifier != "" {
			vpcID, err := p.resolveVPCFromFirstSubnet(ctx, ec2Client, first.Identifier)
			if err != nil {
				return "", err
			}
			return vpcID, nil
		}
	}

	return "", errors.New("when region differs from controller default, set vpcId, vpcSelector, or loadBalancerSubnets with identifiers in LoadBalancerConfiguration")
}

func (p *defaultCloudProvider) resolveVPCFromSelector(ctx context.Context, ec2Client services.EC2, selector map[string][]string) (string, error) {
	filters := make([]ec2types.Filter, 0, len(selector))
	for tagKey, values := range selector {
		if len(values) == 0 {
			continue
		}
		filters = append(filters, ec2types.Filter{
			Name:   awssdk.String("tag:" + tagKey),
			Values: values,
		})
	}
	if len(filters) == 0 {
		return "", errors.New("vpcSelector must have at least one tag key with values")
	}

	vpcs, err := ec2Client.DescribeVPCsAsList(ctx, &ec2sdk.DescribeVpcsInput{
		Filters: filters,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to describe VPCs by tag selector")
	}
	if len(vpcs) == 0 {
		return "", errors.New("no VPC found matching vpcSelector in target region")
	}
	if len(vpcs) > 1 {
		return "", fmt.Errorf("multiple VPCs (%d) found matching vpcSelector in target region; exactly one is required", len(vpcs))
	}
	return awssdk.ToString(vpcs[0].VpcId), nil
}

func (p *defaultCloudProvider) resolveVPCFromFirstSubnet(ctx context.Context, ec2Client services.EC2, subnetIDOrName string) (string, error) {
	subnets, err := ec2Client.DescribeSubnetsAsList(ctx, &ec2sdk.DescribeSubnetsInput{
		SubnetIds: []string{subnetIDOrName},
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to describe subnet %q in target region", subnetIDOrName)
	}
	if len(subnets) == 0 {
		return "", fmt.Errorf("subnet %q not found in target region", subnetIDOrName)
	}
	if subnets[0].VpcId == nil {
		return "", fmt.Errorf("subnet %q has no VpcId", subnetIDOrName)
	}
	return *subnets[0].VpcId, nil
}
