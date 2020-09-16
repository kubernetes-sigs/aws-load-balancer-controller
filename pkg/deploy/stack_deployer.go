package deploy

import (
	"context"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/services"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/ec2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy/tagging"
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
func NewDefaultStackDeployer(k8sClient client.Client, ec2Client services.EC2, elbv2Client services.ELBV2,
	networkingSGManager networking.SecurityGroupManager, networkingSGReconciler networking.SecurityGroupReconciler,
	vpcID string, clusterName string, tagPrefix string, logger logr.Logger) *defaultStackDeployer {
	taggingProvider := tagging.NewDefaultProvider(tagPrefix, clusterName)
	elbv2TaggingManager := elbv2.NewDefaultTaggingManager(elbv2Client, logger)

	return &defaultStackDeployer{
		k8sClient:           k8sClient,
		ec2Client:           ec2Client,
		elbv2Client:         elbv2Client,
		taggingProvider:     taggingProvider,
		networkingSGManager: networkingSGManager,
		ec2SGManager:        ec2.NewDefaultSecurityGroupManager(ec2Client, taggingProvider, networkingSGReconciler, vpcID, logger),
		elbv2TaggingManager: elbv2TaggingManager,
		elbv2LBManager:      elbv2.NewDefaultLoadBalancerManager(elbv2Client, taggingProvider, elbv2TaggingManager, logger),
		elbv2LSManager:      elbv2.NewDefaultListenerManager(elbv2Client, logger),
		elbv2LRManager:      elbv2.NewDefaultListenerRuleManager(elbv2Client, logger),
		elbv2TGManager:      elbv2.NewDefaultTargetGroupManager(elbv2Client, taggingProvider, elbv2TaggingManager, vpcID, logger),
		elbv2TGBManager:     elbv2.NewDefaultTargetGroupBindingManager(k8sClient, taggingProvider, logger),
		vpcID:               vpcID,
		logger:              logger,
	}
}

var _ StackDeployer = &defaultStackDeployer{}

// defaultStackDeployer is the default implementation for StackDeployer
type defaultStackDeployer struct {
	k8sClient           client.Client
	ec2Client           services.EC2
	elbv2Client         services.ELBV2
	taggingProvider     tagging.Provider
	networkingSGManager networking.SecurityGroupManager
	ec2SGManager        ec2.SecurityGroupManager
	elbv2TaggingManager elbv2.TaggingManager
	elbv2LBManager      elbv2.LoadBalancerManager
	elbv2LSManager      elbv2.ListenerManager
	elbv2LRManager      elbv2.ListenerRuleManager
	elbv2TGManager      elbv2.TargetGroupManager
	elbv2TGBManager     elbv2.TargetGroupBindingManager
	vpcID               string

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
