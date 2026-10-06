package logdelivery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
)

var (
	tf           *framework.Framework
	logsClient   *cloudwatchlogssdk.Client
	logGroupName string
)

func TestLogDelivery(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Log Delivery Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	// Skip before InitFramework so the default invocation needs no AWS credentials or cluster.
	if !framework.GetOptions().EnableLogDeliveryTests {
		Skip("log delivery tests require --enable-log-delivery-tests")
	}
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())
	if tf.Options.ControllerImage != "" {
		Expect(tf.CTRLInstallationManager.UpgradeController(tf.Options.ControllerImage, true, true, tf.Options.EnableCertMgmtTests, true)).To(Succeed())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	Eventually(func(g Gomega) {
		deployment := &appsv1.Deployment{}
		g.Expect(tf.K8sClient.Get(ctx, types.NamespacedName{Namespace: "kube-system", Name: "aws-load-balancer-controller"}, deployment)).To(Succeed())
		enabled := false
		for _, container := range deployment.Spec.Template.Spec.Containers {
			for _, arg := range container.Args {
				if strings.HasPrefix(arg, "--feature-gates=") {
					for _, gate := range strings.Split(strings.TrimPrefix(arg, "--feature-gates="), ",") {
						if gate == "LogDelivery=true" {
							enabled = true
						}
					}
				}
			}
		}
		g.Expect(enabled).To(BeTrue(), "install the PR's controller image with --feature-gates=LogDelivery=true, or supply --controller-image")
		g.Expect(deployment.Status.ObservedGeneration).To(BeNumerically(">=", deployment.Generation))
		g.Expect(deployment.Status.UpdatedReplicas).To(Equal(*deployment.Spec.Replicas))
		g.Expect(deployment.Status.AvailableReplicas).To(Equal(*deployment.Spec.Replicas))
	}, 5*time.Minute, 5*time.Second).Should(Succeed())
	return nil
}, func(_ []byte) {
	var err error
	tf, err = framework.InitFramework()
	Expect(err).NotTo(HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(tf.Options.AWSRegion))
	Expect(err).NotTo(HaveOccurred())
	logsClient = cloudwatchlogssdk.NewFromConfig(cfg)
	groupARN, err := arn.Parse(tf.Options.LogDeliveryLogGroupARN)
	Expect(err).NotTo(HaveOccurred())
	Expect(groupARN.Region).To(Equal(tf.Options.AWSRegion), "the log group must be in the test region")
	logGroupName = strings.TrimSuffix(strings.TrimPrefix(groupARN.Resource, "log-group:"), ":*")
	Expect(readLogGroup(ctx)).NotTo(BeNil(), "provision and authorize the log group before running this suite")
})
