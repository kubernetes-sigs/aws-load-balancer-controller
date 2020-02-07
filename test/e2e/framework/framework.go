package framework

import (
	"context"
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/resource"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/utils"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/ticketmaster/aws-sdk-go-cache/cache"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// TODO(@M00nF1sh): migrate to use k8s test framework when it's isolated without pull in all dependencies.
// This is an simplified version of k8s test framework, since some dependencies don't work under go module :(.
// Framework supports common operations used by e2e tests; it will keep a client & a namespace for you.
type Framework struct {
	ClientSet       clientset.Interface
	Cloud           aws.CloudAPI
	ResourceManager *resource.Manager

	Options *Options

	// To make sure that this framework cleans up after itself, no matter what,
	// we install a Cleanup action before each test and clear it after.  If we
	// should abort, the AfterSuite hook should run all Cleanup actions.
	cleanupHandle CleanupActionHandle
}

// New makes a new framework and sets up a BeforeEach/AfterEach for you.
func New() *Framework {
	f := &Framework{
		Options: &globalOptions,
	}

	ginkgo.BeforeEach(f.BeforeEach)
	ginkgo.AfterEach(f.AfterEach)

	return f
}

func (f *Framework) BeforeEach() {
	// The fact that we need this feels like a bug in ginkgo.
	// https://github.com/onsi/ginkgo/issues/222
	if f.ClientSet == nil {
		var err error
		restCfg, err := f.buildRestConfig()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		f.ClientSet, err = clientset.NewForConfig(restCfg)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	if f.Cloud == nil {
		cc := cache.NewConfig(0 * time.Millisecond)
		reg := prometheus.NewRegistry()
		mc, _ := metric.NewCollector(reg, "alb")
		var err error
		f.Cloud, err = aws.New(aws.CloudConfig{Region: f.Options.AWSRegion, VpcID: f.Options.AWSVPCID}, f.Options.ClusterName, mc, false, cc)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	f.ResourceManager = resource.NewManager(f.ClientSet)
	f.cleanupHandle = AddCleanupAction(f.cleanupAction())
}

func (f *Framework) AfterEach() {
	RemoveCleanupAction(f.cleanupHandle)

	f.cleanupAction()()
}

func (f *Framework) cleanupAction() func() {
	resManager := f.ResourceManager
	return func() {
		if err := resManager.Cleanup(context.TODO()); err != nil {
			utils.Failf("%v", err)
		}
	}
}

func (f *Framework) buildRestConfig() (*rest.Config, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", f.Options.KubeConfig)
	if err != nil {
		return nil, err
	}
	restCfg.QPS = 20
	restCfg.Burst = 50
	return restCfg, nil
}
