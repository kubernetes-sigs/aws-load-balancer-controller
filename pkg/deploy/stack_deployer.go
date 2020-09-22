package deploy

import (
	"context"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/services"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/ec2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/shield"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/tagging"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/wafregional"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/wafv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StackDeployer will deploy a resource stack into AWS and K8S.
type StackDeployer interface {
	// Deploy a resource stack.
	Deploy(ctx context.Context, stack core.Stack) error
}

// NewDefaultStackDeployer constructs new defaultStackDeployer.
func NewDefaultStackDeployer(cloud aws.Cloud, k8sClient client.Client,
	networkingSGManager networking.SecurityGroupManager, networkingSGReconciler networking.SecurityGroupReconciler,
	clusterName string, tagPrefix string, logger logr.Logger) *defaultStackDeployer {
	taggingProvider := tagging.NewDefaultProvider(tagPrefix, clusterName)
	elbv2TaggingManager := elbv2.NewDefaultTaggingManager(cloud.ELBV2(), logger)

	return &defaultStackDeployer{
		k8sClient:                           k8sClient,
		ec2Client:                           cloud.EC2(),
		elbv2Client:                         cloud.ELBV2(),
		taggingProvider:                     taggingProvider,
		networkingSGManager:                 networkingSGManager,
		ec2SGManager:                        ec2.NewDefaultSecurityGroupManager(cloud.EC2(), taggingProvider, networkingSGReconciler, cloud.VpcID(), logger),
		elbv2TaggingManager:                 elbv2TaggingManager,
		elbv2LBManager:                      elbv2.NewDefaultLoadBalancerManager(cloud.ELBV2(), taggingProvider, elbv2TaggingManager, logger),
		elbv2LSManager:                      elbv2.NewDefaultListenerManager(cloud.ELBV2(), logger),
		elbv2LRManager:                      elbv2.NewDefaultListenerRuleManager(cloud.ELBV2(), logger),
		elbv2TGManager:                      elbv2.NewDefaultTargetGroupManager(cloud.ELBV2(), taggingProvider, elbv2TaggingManager, cloud.VpcID(), logger),
		elbv2TGBManager:                     elbv2.NewDefaultTargetGroupBindingManager(k8sClient, taggingProvider, logger),
		wafv2WebACLAssociationManager:       wafv2.NewDefaultWebACLAssociationManager(cloud.WAFv2(), logger),
		wafRegionalWebACLAssociationManager: wafregional.NewDefaultWebACLAssociationManager(cloud.WAFRegional(), logger),
		shieldProtectionManager:             shield.NewDefaultProtectionManager(cloud.Shield(), logger),
		vpcID:                               cloud.VpcID(),
		logger:                              logger,
	}
}

var _ StackDeployer = &defaultStackDeployer{}

// defaultStackDeployer is the default implementation for StackDeployer
type defaultStackDeployer struct {
	k8sClient                           client.Client
	ec2Client                           services.EC2
	elbv2Client                         services.ELBV2
	taggingProvider                     tagging.Provider
	networkingSGManager                 networking.SecurityGroupManager
	ec2SGManager                        ec2.SecurityGroupManager
	elbv2TaggingManager                 elbv2.TaggingManager
	elbv2LBManager                      elbv2.LoadBalancerManager
	elbv2LSManager                      elbv2.ListenerManager
	elbv2LRManager                      elbv2.ListenerRuleManager
	elbv2TGManager                      elbv2.TargetGroupManager
	elbv2TGBManager                     elbv2.TargetGroupBindingManager
	wafv2WebACLAssociationManager       wafv2.WebACLAssociationManager
	wafRegionalWebACLAssociationManager wafregional.WebACLAssociationManager
	shieldProtectionManager             shield.ProtectionManager
	vpcID                               string

	logger logr.Logger
}

type ResourceSynthesizer interface {
	Synthesize(ctx context.Context) error
	PostSynthesize(ctx context.Context) error
}

// Deploy a resource stack.
func (d *defaultStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	synthesizers := []ResourceSynthesizer{
		ec2.NewSecurityGroupSynthesizer(d.ec2Client, d.taggingProvider, d.networkingSGManager, d.ec2SGManager, d.vpcID, d.logger, stack),
		elbv2.NewTargetGroupSynthesizer(d.elbv2Client, d.taggingProvider, d.elbv2TaggingManager, d.elbv2TGManager, d.logger, stack),
		elbv2.NewTargetGroupBindingSynthesizer(d.k8sClient, d.taggingProvider, d.elbv2TGBManager, d.logger, stack),
		elbv2.NewLoadBalancerSynthesizer(d.elbv2Client, d.taggingProvider, d.elbv2TaggingManager, d.elbv2LBManager, d.logger, stack),
		elbv2.NewListenerSynthesizer(d.elbv2Client, d.elbv2LSManager, d.logger, stack),
		elbv2.NewListenerRuleSynthesizer(d.elbv2Client, d.elbv2LRManager, d.logger, stack),
		wafv2.NewWebACLAssociationSynthesizer(d.wafv2WebACLAssociationManager, d.logger, stack),
		wafregional.NewWebACLAssociationSynthesizer(d.wafRegionalWebACLAssociationManager, d.logger, stack),
		shield.NewProtectionSynthesizer(d.shieldProtectionManager, d.logger, stack),
	}
	for _, synthesizer := range synthesizers {
		if err := synthesizer.Synthesize(ctx); err != nil {
			return err
		}
	}
	for i := len(synthesizers) - 1; i >= 0; i-- {
		if err := synthesizers[i].PostSynthesize(ctx); err != nil {
			return err
		}
	}

	return nil
}
