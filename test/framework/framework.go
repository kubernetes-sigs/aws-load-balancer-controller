package framework

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	awsresources "sigs.k8s.io/aws-load-balancer-controller/test/framework/resources/aws"
	k8sresources "sigs.k8s.io/aws-load-balancer-controller/test/framework/resources/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
)

var (
	frameworkSingleton         *Framework
	frameworkSingletonInitOnce sync.Once
)

type Framework struct {
	Options   Options
	RestCfg   *rest.Config
	K8sClient client.Client
	Cloud     aws.Cloud

	NSManager  k8sresources.NamespaceManager
	DPManager  k8sresources.DeploymentManager
	SVCManager k8sresources.ServiceManager
	INGManager k8sresources.IngressManager
	LBManager  awsresources.LoadBalancerManager

	HTTPVerifier http.Verifier

	Logger   logr.Logger
	StopChan <-chan struct{}
}

// New constructs new framework
func New() *Framework {
	frameworkSingletonInitOnce.Do(func() {
		frameworkSingleton = initFramework()
	})
	return frameworkSingleton
}

func initFramework() *Framework {
	err := globalOptions.Validate()
	Expect(err).NotTo(HaveOccurred())
	restCfg := ctrl.GetConfigOrDie()

	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)
	elbv2api.AddToScheme(k8sSchema)

	stopChan := ctrl.SetupSignalHandler()
	cache, err := cache.New(restCfg, cache.Options{Scheme: k8sSchema})
	Expect(err).NotTo(HaveOccurred())
	go func() {
		cache.Start(stopChan)
	}()

	cache.WaitForCacheSync(stopChan)
	realClient, err := client.New(restCfg, client.Options{Scheme: k8sSchema})
	Expect(err).NotTo(HaveOccurred())
	k8sClient := client.DelegatingClient{
		Reader: &client.DelegatingReader{
			CacheReader:  cache,
			ClientReader: realClient,
		},
		Writer:       realClient,
		StatusClient: realClient,
	}
	cloud, err := aws.NewCloud(aws.CloudConfig{
		Region:         globalOptions.AWSRegion,
		VpcID:          globalOptions.AWSVPCID,
		MaxRetries:     3,
		ThrottleConfig: throttle.NewDefaultServiceOperationsThrottleConfig(),
	}, nil)
	Expect(err).NotTo(HaveOccurred())

	logger := utils.NewGinkgoLogger()

	f := &Framework{
		Options:   globalOptions,
		RestCfg:   restCfg,
		K8sClient: k8sClient,
		Cloud:     cloud,

		NSManager:  k8sresources.NewDefaultNamespaceManager(k8sClient, logger),
		DPManager:  k8sresources.NewDefaultDeploymentManager(k8sClient, logger),
		SVCManager: k8sresources.NewDefaultServiceManager(k8sClient, logger),
		INGManager: k8sresources.NewDefaultIngressManager(k8sClient, logger),
		LBManager:  awsresources.NewDefaultLoadBalancerManager(cloud.ELBV2(), logger),

		HTTPVerifier: http.NewDefaultVerifier(),

		Logger:   logger,
		StopChan: stopChan,
	}

	return f
}
