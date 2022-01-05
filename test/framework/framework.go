package framework

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/controller"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/helm"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	awsresources "sigs.k8s.io/aws-load-balancer-controller/test/framework/resources/aws"
	k8sresources "sigs.k8s.io/aws-load-balancer-controller/test/framework/resources/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Framework struct {
	Options   Options
	RestCfg   *rest.Config
	K8sClient client.Client
	Cloud     aws.Cloud

	CTRLInstallationManager controller.InstallationManager
	NSManager               k8sresources.NamespaceManager
	DPManager               k8sresources.DeploymentManager
	SVCManager              k8sresources.ServiceManager
	INGManager              k8sresources.IngressManager
	LBManager               awsresources.LoadBalancerManager
	TGManager               awsresources.TargetGroupManager

	HTTPVerifier http.Verifier

	Logger utils.GinkgoLogger
}

func InitFramework() (*Framework, error) {
	err := globalOptions.Validate()
	if err != nil {
		return nil, err
	}
	restCfg := ctrl.GetConfigOrDie()

	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)
	elbv2api.AddToScheme(k8sSchema)

	k8sClient, err := client.New(restCfg, client.Options{Scheme: k8sSchema})
	if err != nil {
		return nil, err
	}

	cloud, err := aws.NewCloud(aws.CloudConfig{
		Region:         globalOptions.AWSRegion,
		VpcID:          globalOptions.AWSVPCID,
		MaxRetries:     3,
		ThrottleConfig: throttle.NewDefaultServiceOperationsThrottleConfig(),
	}, nil)
	if err != nil {
		return nil, err
	}

	logger := utils.NewGinkgoLogger()

	f := &Framework{
		Options:   globalOptions,
		RestCfg:   restCfg,
		K8sClient: k8sClient,
		Cloud:     cloud,

		CTRLInstallationManager: buildControllerInstallationManager(globalOptions, logger),
		NSManager:               k8sresources.NewDefaultNamespaceManager(k8sClient, logger),
		DPManager:               k8sresources.NewDefaultDeploymentManager(k8sClient, logger),
		SVCManager:              k8sresources.NewDefaultServiceManager(k8sClient, logger),
		INGManager:              k8sresources.NewDefaultIngressManager(k8sClient, logger),
		LBManager:               awsresources.NewDefaultLoadBalancerManager(cloud.ELBV2(), logger),
		TGManager:               awsresources.NewDefaultTargetGroupManager(cloud.ELBV2(), logger),

		HTTPVerifier: http.NewDefaultVerifier(),

		Logger: logger,
	}

	return f, nil
}

func buildControllerInstallationManager(options Options, logger logr.Logger) controller.InstallationManager {
	helmReleaseManager := helm.NewDefaultReleaseManager(options.KubeConfig, logger)
	ctrlInstallationManager := controller.NewDefaultInstallationManager(helmReleaseManager, options.ClusterName, options.AWSRegion, options.AWSVPCID, options.HelmChart, logger)
	return ctrlInstallationManager
}
