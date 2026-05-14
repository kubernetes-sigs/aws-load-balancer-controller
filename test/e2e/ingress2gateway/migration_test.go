package ingress2gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/manifest"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ingress to gateway migration", func() {
	var (
		ctx       context.Context
		sandboxNS *corev1.Namespace
		ingClass  *networking.IngressClass
	)

	BeforeEach(func() {
		if !tf.Options.EnableMigrationTests {
			Skip("Skipping migration tests")
		}
		ctx = context.Background()
		By("setup sandbox namespace")
		ns, err := tf.NSManager.AllocateNamespace(ctx, "i2g-e2e")
		Expect(err).NotTo(HaveOccurred())
		sandboxNS = ns
	})

	AfterEach(func() {
		if sandboxNS != nil {
			By("teardown sandbox namespace")
			ingList := &networking.IngressList{}
			_ = tf.K8sClient.List(ctx, ingList, client.InNamespace(sandboxNS.Name))
			for i := range ingList.Items {
				_ = tf.K8sClient.Delete(ctx, &ingList.Items[i])
			}
			for i := range ingList.Items {
				_ = tf.INGManager.WaitUntilIngressDeleted(ctx, &ingList.Items[i])
			}
			deleteGatewayResources(ctx, tf, sandboxNS.Name)
			if ingClass != nil {
				_ = tf.K8sClient.Delete(ctx, ingClass)
			}
			err := tf.K8sClient.Delete(ctx, sandboxNS)
			Expect(err).Should(SatisfyAny(BeNil(), Satisfy(apierrs.IsNotFound)))
			err = tf.NSManager.WaitUntilNamespaceDeleted(ctx, sandboxNS)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should produce expected gateway manifests from file input", func() {
		outputDir, err := os.MkdirTemp("", "i2g-vanilla-*")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(outputDir)

		By("running lbc-migrate with --file input", func() {
			err = runMigrateTool("", outputDir,
				"--file", "testdata/vanilla_ingress.yaml",
			)
			Expect(err).NotTo(HaveOccurred())
		})

		By("comparing output against expected gateway file", func() {
			actual, err := os.ReadFile(filepath.Join(outputDir, "gateway-resources.yaml"))
			Expect(err).NotTo(HaveOccurred())

			expected, err := os.ReadFile("testdata/expected_vanilla_gateway.yaml")
			Expect(err).NotTo(HaveOccurred())

			Expect(string(actual)).To(Equal(string(expected)), "generated gateway manifest does not match expected")
		})
	})

	It("should migrate an IngressGroup with multiple paths, tags, and health checks through dry-run and full cutover", func() {
		groupName := fmt.Sprintf("e2e-mig-%s", sandboxNS.Name)

		// --- Phase 1: Create Ingress baseline ---
		By("creating backend services")
		appBuilder := manifest.NewFixedResponseServiceBuilder()

		dpA, svcA := appBuilder.WithHTTPBody(bodyServiceA).Build(sandboxNS.Name, "svc-a", tf.Options.TestImageRegistry)
		dpB, svcB := appBuilder.WithHTTPBody(bodyServiceB).Build(sandboxNS.Name, "svc-b", tf.Options.TestImageRegistry)
		dpC, svcC := appBuilder.WithHTTPBody(bodyServiceC).Build(sandboxNS.Name, "svc-c", tf.Options.TestImageRegistry)

		for _, obj := range []client.Object{dpA, svcA, dpB, svcB, dpC, svcC} {
			Expect(tf.K8sClient.Create(ctx, obj)).To(Succeed())
		}
		_, err := tf.DPManager.WaitUntilDeploymentReady(ctx, dpA)
		Expect(err).NotTo(HaveOccurred())
		_, err = tf.DPManager.WaitUntilDeploymentReady(ctx, dpB)
		Expect(err).NotTo(HaveOccurred())
		_, err = tf.DPManager.WaitUntilDeploymentReady(ctx, dpC)
		Expect(err).NotTo(HaveOccurred())

		By("creating IngressGroup (2 members)")
		ipFamily := ""
		if tf.Options.IPFamily == framework.IPv6 {
			ipFamily = "IPv6"
		}
		group := buildBasicIngressGroup(sandboxNS.Name, groupName, ipFamily, svcA.Name, svcB.Name, svcC.Name)

		ingClass = group.IngressClass
		Expect(tf.K8sClient.Create(ctx, ingClass)).To(Succeed())

		expectedTraffic := []trafficCase{
			{pathRoot, hostAdmin, bodyServiceC},
			{pathAPI, hostApp, bodyServiceA},
			{pathHealth, hostApp, bodyServiceB},
		}

		for _, ing := range group.Ingresses {
			Expect(tf.K8sClient.Create(ctx, ing)).To(Succeed())
		}

		By("waiting for Ingress ALB to be provisioned")
		primaryIng := group.Ingresses[0]
		var ingressLBDNS string
		Eventually(func(g Gomega) {
			err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(primaryIng), primaryIng)
			g.Expect(err).NotTo(HaveOccurred())
			ingressLBDNS = findIngressDNS(primaryIng)
			g.Expect(ingressLBDNS).NotTo(BeEmpty())
		}, utils.CertReconcileTimeout, utils.PollIntervalShort).Should(Succeed())
		tf.Logger.Info("ingress DNS populated", "dns", ingressLBDNS)

		ingressLBARN, err := tf.LBManager.FindLoadBalancerByDNSName(ctx, ingressLBDNS)
		Expect(err).NotTo(HaveOccurred())
		Expect(ingressLBARN).NotTo(BeEmpty())

		tf.Logger.Info("waiting for ingress ALB to be available")
		err = tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, ingressLBARN)
		Expect(err).NotTo(HaveOccurred())

		By("verifying traffic to ingress ALB", func() {
			ExpectLBDNSResolvable(ctx, tf, ingressLBDNS)
			verifyTraffic(tf, ingressLBDNS, expectedTraffic)
		})

		By("verifying dry-run-plan annotation on primary member only", func() {
			expectDryRunPlan(ctx, tf, group.Ingresses[0], group.Ingresses[1:]...)
		})

		// --- Phase 2: Run migration tool ---
		outputDir, err := os.MkdirTemp("", "i2g-e2e-*")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(outputDir)

		By("running lbc-migrate tool", func() {
			err = runMigrateTool(sandboxNS.Name, outputDir, "--dry-run")
			Expect(err).NotTo(HaveOccurred())
		})

		By("verifying migration output files exist", func() {
			outputFiles, err := filepath.Glob(filepath.Join(outputDir, "*.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(outputFiles).NotTo(BeEmpty())
		})

		// --- Phase 3: Apply in dry-run mode and verify ---
		By("applying generated Gateway manifests (dry-run mode)", func() {
			err = applyYAMLDir(outputDir)
			Expect(err).NotTo(HaveOccurred())
		})

		By("verifying generated resource counts", func() {
			verifyResourceCounts(ctx, tf, sandboxNS.Name, expectedResourceCounts{
				gateways:                   1,
				httpRoutes:                 2,
				loadBalancerConfigurations: 1,
				targetGroupConfigurations:  3,
				listenerRuleConfigurations: 3,
			})
		})

		gw := findGatewayInNamespace(ctx, tf, sandboxNS.Name)
		Expect(gw).NotTo(BeNil(), "no Gateway found in namespace %s", sandboxNS.Name)

		By("verifying Gateway has dry-run annotation", func() {
			Expect(gw.Annotations[gwDryRunAnnotation]).To(Equal("true"))
		})

		By("waiting for Gateway dry-run-plan annotation", func() {
			expectGatewayDryRunPlan(ctx, tf, gw)
		})

		By("verifying NO ALB is created for dry-run Gateway", func() {
			expectNoALBForGateway(ctx, tf, gw)
		})

		// --- Phase 4: Full cutover ---
		By("removing dry-run annotation to trigger real reconciliation", func() {
			removeDryRunAnnotation(ctx, tf, gw)
		})

		By("waiting for Gateway ALB to be provisioned")
		gatewayLBARN, gatewayLBDNS := expectGatewayALBProvisioned(ctx, tf, gw)

		tf.Logger.Info("waiting for gateway ALB to be available")
		err = tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, gatewayLBARN)
		Expect(err).NotTo(HaveOccurred())

		By("comparing ALB configurations: listeners, rules, and target groups", func() {
			compareALBConfigurations(ctx, tf, ingressLBARN, gatewayLBARN)
		})

		By("comparing ALB configurations: tags", func() {
			migrationTagLBValue := fmt.Sprintf("ingress-group/%s", groupName)
			compareTags(ctx, tf, ingressLBARN, gatewayLBARN, migrationTagLBValue, map[string]string{
				"Team":      "e2e",
				"Component": "migration",
			})
		})

		By("verifying traffic to Gateway ALB", func() {
			ExpectLBDNSResolvable(ctx, tf, gatewayLBDNS)
			verifyTraffic(tf, gatewayLBDNS, expectedTraffic)
		})
	})
})
