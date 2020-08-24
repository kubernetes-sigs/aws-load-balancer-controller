package framework

import (
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/test/utils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Framework struct {
	Options   Options
	RestCfg   *rest.Config
	K8sClient client.Client
	AwsClient aws.Cloud

	Logger   *zap.Logger
	StopChan <-chan struct{}
}

func New(options Options) *Framework {
	err := options.Validate()
	Expect(err).ToNot(HaveOccurred())

	restCfg, err := clientcmd.BuildConfigFromFlags("", options.KubeConfig)
	Expect(err).ToNot(HaveOccurred())

	k8sSchema := runtime.NewScheme()
	scheme.AddToScheme(k8sSchema)

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

	awsClient, err := aws.NewCloud(aws.CloudConfig{
		Region:         options.AwsRegion,
	}, nil)
	Expect(err).NotTo(HaveOccurred())

	return &Framework{
		Options:   options,
		RestCfg:   restCfg,
		K8sClient: k8sClient,
		AwsClient: awsClient,
		Logger:    utils.NewGinkgoLogger(),
		StopChan:  stopChan,
	}
}
