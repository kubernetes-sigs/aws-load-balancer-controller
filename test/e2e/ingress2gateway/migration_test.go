package ingress2gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/console"
	i2gutils "sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/manifest"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Ingress to Gateway Migration Tests", func() {
	var ctx context.Context

	BeforeEach(func() {
		if !tf.Options.EnableMigrationTests {
			Skip("Skipping migration tests")
		}
		ctx = context.Background()
	})

	// Context: dry-run plan comparison tests.
	// These tests apply Ingress and Gateway with dry-run, then compare the dry-run plan
	// annotations from both controllers to verify the migration tool produces equivalent
	// resources. Faster than full cutover (no need to verify real ALB traffic).
	Context("dry-run plan comparison", func() {
		var (
			sandboxNS    *corev1.Namespace
			sandboxNS2   *corev1.Namespace
			ingClasses   []*networking.IngressClass
			icpResources []client.Object
		)

		AfterEach(func() {
			// Delete all namespaces in parallel — cascading deletion handles all
			// namespaced resources (Ingresses, Services, etc.) and the controller
			// removes finalizers as part of group reconciliation. Matches the pattern
			// in test/e2e/ingress/multi_path_backend.go.
			var nsWG sync.WaitGroup
			for _, ns := range []*corev1.Namespace{sandboxNS, sandboxNS2} {
				if ns == nil {
					continue
				}
				nsWG.Add(1)
				go func(ns *corev1.Namespace) {
					defer nsWG.Done()
					_ = tf.K8sClient.Delete(ctx, ns)
					_ = tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns)
				}(ns)
			}
			nsWG.Wait()

			for _, ic := range ingClasses {
				_ = tf.K8sClient.Delete(ctx, ic)
			}
			for _, obj := range icpResources {
				_ = tf.K8sClient.Delete(ctx, obj)
			}

			ingClasses = nil
			icpResources = nil
			sandboxNS = nil
			sandboxNS2 = nil
		})

		// Test 1: Complex single Ingress with many annotation categories.
		// CLI: --from-cluster
		It("should produce equivalent plan for complex single ingress", func() {
			By("setting up namespace and services")
			ns, err := tf.NSManager.AllocateNamespace(ctx, "i2g-e2e-cmp1")
			Expect(err).NotTo(HaveOccurred())
			sandboxNS = ns

			createServicesAndWait(ctx, tf, []serviceSpec{
				{Namespace: ns.Name, Name: "svc-api", Annotations: map[string]string{annotationHealthCheckPath: "/api-health"}},
				{Namespace: ns.Name, Name: "svc-static"},
			})

			By("creating IngressClass and Ingress")
			ingressClassName := fmt.Sprintf("alb-%s", ns.Name)
			ingClass := &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{Name: ingressClassName},
				Spec:       networking.IngressClassSpec{Controller: ingressController},
			}
			Expect(tf.K8sClient.Create(ctx, ingClass)).To(Succeed())
			ingClasses = append(ingClasses, ingClass)

			ing := buildComplexSingleIngress(ns.Name, ingressClassName)
			Expect(tf.K8sClient.Create(ctx, ing)).To(Succeed())

			By("waiting for Ingress dry-run plan")
			ingressPlan := expectDryRunPlan(ctx, tf, ing)
			Expect(ingressPlan).NotTo(BeEmpty())

			By("running lbc-migrate --from-cluster")
			outputDir, err := os.MkdirTemp("", "i2g-e2e-cmp1-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(outputDir)

			err = runMigrateTool(ns.Name, outputDir, "--dry-run")
			Expect(err).NotTo(HaveOccurred())

			By("applying generated Gateway manifests")
			err = applyYAMLDir(outputDir)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Gateway dry-run plan")
			gw := findGatewayInNamespace(ctx, tf, ns.Name)
			Expect(gw).NotTo(BeNil())
			gatewayPlan := expectGatewayDryRunPlan(ctx, tf, gw)
			Expect(gatewayPlan).NotTo(BeEmpty())

			By("comparing plans")
			userSpecified := console.BuildUserSpecifiedFields(ing.Annotations)
			assertPlansEquivalent(ingressPlan, gatewayPlan, userSpecified)
		})

		// Test 2: Cross-namespace IngressGroup with ICP + actions.
		// CLI: -f (file mode)
		It("should produce equivalent plan for cross-namespace group via file mode", func() {
			By("setting up namespaces and services")
			nsA, err := tf.NSManager.AllocateNamespace(ctx, "i2g-e2e-cmp2a")
			Expect(err).NotTo(HaveOccurred())
			sandboxNS = nsA

			nsB, err := tf.NSManager.AllocateNamespace(ctx, "i2g-e2e-cmp2b")
			Expect(err).NotTo(HaveOccurred())
			sandboxNS2 = nsB

			createServicesAndWait(ctx, tf, []serviceSpec{
				{Namespace: nsA.Name, Name: "svc-blue"},
				{Namespace: nsA.Name, Name: "svc-green"},
				{Namespace: nsB.Name, Name: "svc-search"},
			})

			By("creating IngressClass with IngressClassParams")
			ingressClassName := fmt.Sprintf("alb-%s", nsA.Name)
			icpScheme := elbv2api.LoadBalancerSchemeInternetFacing
			icp := &elbv2api.IngressClassParams{
				ObjectMeta: metav1.ObjectMeta{Name: ingressClassName},
				Spec: elbv2api.IngressClassParamsSpec{
					Scheme: &icpScheme,
					Tags:   []elbv2api.Tag{{Key: "ManagedBy", Value: "platform"}},
				},
			}
			Expect(tf.K8sClient.Create(ctx, icp)).To(Succeed())
			icpResources = append(icpResources, icp)

			elbv2Group := "elbv2.k8s.aws"
			ingClass := &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{Name: ingressClassName},
				Spec: networking.IngressClassSpec{
					Controller: ingressController,
					Parameters: &networking.IngressClassParametersReference{
						APIGroup: &elbv2Group,
						Kind:     "IngressClassParams",
						Name:     ingressClassName,
					},
				},
			}
			Expect(tf.K8sClient.Create(ctx, ingClass)).To(Succeed())
			ingClasses = append(ingClasses, ingClass)

			By("creating grouped Ingresses")
			groupName := fmt.Sprintf("cross-ns-%s", nsA.Name)
			ingA := buildGroupMemberA(nsA.Name, groupName, ingressClassName)
			ingB := buildGroupMemberB(nsB.Name, groupName, ingressClassName)

			Expect(tf.K8sClient.Create(ctx, ingA)).To(Succeed())
			Expect(tf.K8sClient.Create(ctx, ingB)).To(Succeed())

			By("waiting for Ingress dry-run plan")
			ingressPlan := expectDryRunPlan(ctx, tf, ingA, ingB)
			Expect(ingressPlan).NotTo(BeEmpty())

			By("running lbc-migrate via -f (file mode)")
			outputDir, err := os.MkdirTemp("", "i2g-e2e-cmp2-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(outputDir)

			inputFile := writeResourcesToTempFile(ingClass, icp, ingA, ingB,
				findServiceInNamespace(ctx, tf, nsA.Name, "svc-blue"),
				findServiceInNamespace(ctx, tf, nsA.Name, "svc-green"),
				findServiceInNamespace(ctx, tf, nsB.Name, "svc-search"),
			)
			defer os.Remove(inputFile)

			err = runMigrateToolWithFile(inputFile, outputDir, "--dry-run")
			Expect(err).NotTo(HaveOccurred())

			By("applying generated Gateway manifests")
			err = applyYAMLDir(outputDir)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Gateway dry-run plan")
			gw := findGatewayInNamespace(ctx, tf, nsA.Name)
			Expect(gw).NotTo(BeNil())
			gatewayPlan := expectGatewayDryRunPlan(ctx, tf, gw)
			Expect(gatewayPlan).NotTo(BeEmpty())

			By("comparing plans")
			userSpecified := console.BuildUserSpecifiedFields(ingA.Annotations)
			assertPlansEquivalent(ingressPlan, gatewayPlan, userSpecified)
		})

		// Test 3: Multi-group cluster with mixed ingresses.
		// CLI: --input-dir + --split=namespace
		It("should correctly partition multiple groups and standalone ingresses", func() {
			By("setting up namespaces")
			nsPlatform, err := tf.NSManager.AllocateNamespace(ctx, "i2g-e2e-cmp3-plat")
			Expect(err).NotTo(HaveOccurred())
			sandboxNS = nsPlatform

			nsTeamA, err := tf.NSManager.AllocateNamespace(ctx, "i2g-e2e-cmp3-a")
			Expect(err).NotTo(HaveOccurred())
			sandboxNS2 = nsTeamA

			By("creating services with running pods")
			createServicesAndWait(ctx, tf, []serviceSpec{
				{Namespace: nsPlatform.Name, Name: "svc-platform-1"},
				{Namespace: nsPlatform.Name, Name: "svc-platform-2"},
				{Namespace: nsTeamA.Name, Name: "svc-standalone"},
			})

			By("creating IngressClasses")
			albClassName := fmt.Sprintf("alb-%s", nsPlatform.Name)
			albClass := &networking.IngressClass{
				ObjectMeta: metav1.ObjectMeta{Name: albClassName},
				Spec:       networking.IngressClassSpec{Controller: ingressController},
			}
			Expect(tf.K8sClient.Create(ctx, albClass)).To(Succeed())
			ingClasses = append(ingClasses, albClass)

			By("creating platform group (2 members in ns-platform)")
			platformGroup := fmt.Sprintf("platform-%s", nsPlatform.Name)
			// Path length determines Gateway API priority; we make member-1's path longer
			// so its rule gets higher priority on both Ingress (group.order=1) and Gateway sides.
			platformIng1 := buildPlatformGroupMember(nsPlatform.Name, platformGroup, albClassName, "svc-platform-1", "1", "/api/v1")
			platformIng2 := buildPlatformGroupMember(nsPlatform.Name, platformGroup, albClassName, "svc-platform-2", "2", "/api")
			Expect(tf.K8sClient.Create(ctx, platformIng1)).To(Succeed())
			Expect(tf.K8sClient.Create(ctx, platformIng2)).To(Succeed())

			By("creating standalone ingress in ns-team-a (no group)")
			standaloneIng := buildStandaloneIngress(nsTeamA.Name, albClassName, "svc-standalone")
			Expect(tf.K8sClient.Create(ctx, standaloneIng)).To(Succeed())

			By("waiting for Ingress dry-run plans")
			platformPlan := expectDryRunPlan(ctx, tf, platformIng1, platformIng2)
			Expect(platformPlan).NotTo(BeEmpty())
			standalonePlan := expectDryRunPlan(ctx, tf, standaloneIng)
			Expect(standalonePlan).NotTo(BeEmpty())

			By("running lbc-migrate --input-dir with --split=namespace")
			inputDir, err := os.MkdirTemp("", "i2g-e2e-cmp3-input-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(inputDir)

			writeResourcesToDir(inputDir,
				albClass,
				platformIng1, platformIng2,
				standaloneIng,
				findServiceInNamespace(ctx, tf, nsPlatform.Name, "svc-platform-1"),
				findServiceInNamespace(ctx, tf, nsPlatform.Name, "svc-platform-2"),
				findServiceInNamespace(ctx, tf, nsTeamA.Name, "svc-standalone"),
			)

			outputDir, err := os.MkdirTemp("", "i2g-e2e-cmp3-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(outputDir)

			err = runMigrateToolWithInputDir(inputDir, outputDir, "--dry-run", "--split=namespace")
			Expect(err).NotTo(HaveOccurred())

			By("verifying split-by-namespace output layout")
			_, err = os.Stat(filepath.Join(outputDir, "gatewayclass.yaml"))
			Expect(err).NotTo(HaveOccurred(), "gatewayclass.yaml should exist at top level")
			_, err = os.Stat(filepath.Join(outputDir, nsPlatform.Name, "gateway-resources.yaml"))
			Expect(err).NotTo(HaveOccurred(), "namespace subdirectory for ns-platform should exist")
			_, err = os.Stat(filepath.Join(outputDir, nsTeamA.Name, "gateway-resources.yaml"))
			Expect(err).NotTo(HaveOccurred(), "namespace subdirectory for ns-team-a should exist")

			By("applying generated Gateway manifests recursively")
			err = applyYAMLDirRecursive(outputDir)
			Expect(err).NotTo(HaveOccurred())

			By("verifying correct number of Gateways produced")
			platformGWName := i2gutils.GetGroupGatewayName(platformGroup)
			platformGW := findGatewayByName(ctx, tf, nsPlatform.Name, platformGWName)
			Expect(platformGW).NotTo(BeNil(), "platform group Gateway %s/%s not found", nsPlatform.Name, platformGWName)

			standaloneGWName := i2gutils.GetGatewayName(nsTeamA.Name, "standalone")
			standaloneGW := findGatewayByName(ctx, tf, nsTeamA.Name, standaloneGWName)
			Expect(standaloneGW).NotTo(BeNil(), "standalone Gateway %s/%s not found", nsTeamA.Name, standaloneGWName)

			By("comparing platform group plans")
			platformGWPlan := expectGatewayDryRunPlan(ctx, tf, platformGW)
			userSpecifiedPlatform := console.BuildUserSpecifiedFields(platformIng1.Annotations)
			assertPlansEquivalent(platformPlan, platformGWPlan, userSpecifiedPlatform)

			By("comparing standalone plans")
			standaloneGWPlan := expectGatewayDryRunPlan(ctx, tf, standaloneGW)
			userSpecifiedStandalone := console.BuildUserSpecifiedFields(standaloneIng.Annotations)
			assertPlansEquivalent(standalonePlan, standaloneGWPlan, userSpecifiedStandalone)
		})
	})

	// Context: full cutover workflow test.
	// This test creates a real ALB from an Ingress, runs the migration tool, applies
	// the Gateway manifests in dry-run mode, then triggers a real cutover and verifies
	// the Gateway-side ALB matches the Ingress-side ALB (listeners, rules, target groups,
	// tags, and traffic).
	Context("full cutover workflow", func() {
		var (
			sandboxNS *corev1.Namespace
			ingClass  *networking.IngressClass
		)

		BeforeEach(func() {
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
})
