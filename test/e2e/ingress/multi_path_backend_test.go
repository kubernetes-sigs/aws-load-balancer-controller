package ingress

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("test ingresses with multiple path and backends", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		if tf.Options.ControllerImage != "" {
			By(fmt.Sprintf("ensure cluster installed with controller: %s", tf.Options.ControllerImage), func() {
				err := tf.CTRLInstallationManager.UpgradeController(tf.Options.ControllerImage)
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(60 * time.Second)
			})
		}
	})

	AfterEach(func() {
		// TODO, force cleanup all left AWS resources if any.
		// TODO, force cleanup all left K8s resources if any.
	})

	Context("with podReadinessGate enabled", func() {
		It("standalone Ingress should behaves correctly", func() {
			// TODO: Once instance mode is supported in IPv6, the backendConfigA can be removed and reverted
			backendConfigA := BackendConfig{
				Replicas:   3,
				TargetType: elbv2model.TargetTypeInstance,
				HTTPBody:   "backend-a",
			}
			if tf.Options.IPFamily == "IPv6" {
				backendConfigA = BackendConfig{
					Replicas:   3,
					TargetType: elbv2model.TargetTypeIP,
					HTTPBody:   "backend-a",
				}
			}
			stack := NewMultiPathBackendStack(map[string]NamespacedResourcesConfig{
				"ns-1": {
					IngCFGs: map[string]MultiPathIngressConfig{
						"ing-1": {
							PathCFGs: []PathConfig{
								{
									Path:      "/path-a",
									BackendID: "backend-a",
								},
								{
									Path:      "/path-b",
									BackendID: "backend-b",
								},
							},
						},
					},
					BackendCFGs: map[string]BackendConfig{
						"backend-a": backendConfigA,
						"backend-b": {
							Replicas:   3,
							TargetType: elbv2model.TargetTypeIP,
							HTTPBody:   "backend-b",
						},
					},
				},
			}, true)

			By("deploy stack")
			err := stack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())

			By("expect dns name from Ingresses be non-empty")
			dnsName := expectDNSNameFromIngressNonEmpty(ctx, tf, stack, "ns-1", "ing-1")

			By(fmt.Sprintf("expect dns name eventually be available: %v", dnsName), func() {
				expectDNSNameEventuallyAvailable(ctx, tf, dnsName)
			})

			time.Sleep(60 * time.Second)

			url := fmt.Sprintf("http://%v%v", dnsName, "/path-a")
			By(fmt.Sprintf("expect %v returns %v", url, "backend-a"), func() {
				err = tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-a")))
				Expect(err).NotTo(HaveOccurred())
			})

			url = fmt.Sprintf("http://%v%v", dnsName, "/path-b")
			By(fmt.Sprintf("expect %v returns %v", url, "backend-b"), func() {
				err = tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-b")))
				Expect(err).NotTo(HaveOccurred())
			})

			err = stack.Cleanup(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("IngressGroup across namespaces should behaves correctly", func() {
			// TODO: Once instance mode is supported in IPv6, the backendConfigA and backendConfigD can be removed and reverted
			backendConfigA := BackendConfig{
				Replicas:   3,
				TargetType: elbv2model.TargetTypeInstance,
				HTTPBody:   "backend-a",
			}
			backendConfigD := BackendConfig{
				Replicas:   3,
				TargetType: elbv2model.TargetTypeInstance,
				HTTPBody:   "backend-d",
			}
			if tf.Options.IPFamily == "IPv6" {
				backendConfigA = BackendConfig{
					Replicas:   3,
					TargetType: elbv2model.TargetTypeIP,
					HTTPBody:   "backend-a",
				}
				backendConfigD = BackendConfig{
					Replicas:   3,
					TargetType: elbv2model.TargetTypeIP,
					HTTPBody:   "backend-d",
				}
			}
			groupName := fmt.Sprintf("e2e-group.%v", utils.RandomDNS1123Label(8))
			stack := NewMultiPathBackendStack(map[string]NamespacedResourcesConfig{
				"ns-1": {
					IngCFGs: map[string]MultiPathIngressConfig{
						"ing-1": {
							GroupName: groupName,
							PathCFGs: []PathConfig{
								{
									Path:      "/path-a",
									BackendID: "backend-a",
								},
								{
									Path:      "/path-b",
									BackendID: "backend-b",
								},
							},
						},
						"ing-2": {
							GroupName: groupName,
							PathCFGs: []PathConfig{
								{
									Path:      "/path-c",
									BackendID: "backend-c",
								},
							},
						},
					},
					BackendCFGs: map[string]BackendConfig{
						"backend-a": backendConfigA,
						"backend-b": {
							Replicas:   3,
							TargetType: elbv2model.TargetTypeIP,
							HTTPBody:   "backend-b",
						},
						"backend-c": {
							Replicas:   3,
							TargetType: elbv2model.TargetTypeIP,
							HTTPBody:   "backend-c",
						},
					},
				},
				"ns-2": {
					IngCFGs: map[string]MultiPathIngressConfig{
						"ing-3": {
							GroupName: groupName,
							PathCFGs: []PathConfig{
								{
									Path:      "/path-d",
									BackendID: "backend-d",
								},
								{
									Path:      "/path-e",
									BackendID: "backend-e",
								},
							},
						},
					},
					BackendCFGs: map[string]BackendConfig{
						"backend-d": backendConfigD,
						"backend-e": {
							Replicas:   3,
							TargetType: elbv2model.TargetTypeIP,
							HTTPBody:   "backend-e",
						},
					},
				},
			}, true)

			By("deploy stack")
			err := stack.Deploy(ctx, tf)
			Expect(err).NotTo(HaveOccurred())

			By("expect dns name from Ingresses be non-empty")
			dnsName := expectDNSNameFromIngressNonEmpty(ctx, tf, stack, "ns-1", "ing-1")
			dnsName2 := expectDNSNameFromIngressNonEmpty(ctx, tf, stack, "ns-1", "ing-2")
			dnsName3 := expectDNSNameFromIngressNonEmpty(ctx, tf, stack, "ns-2", "ing-3")

			Expect(dnsName).To(Equal(dnsName2))
			Expect(dnsName2).To(Equal(dnsName3))

			By(fmt.Sprintf("expect dns name eventually be available: %v", dnsName), func() {
				expectDNSNameEventuallyAvailable(ctx, tf, dnsName)
			})

			time.Sleep(60 * time.Second)

			url := fmt.Sprintf("http://%v%v", dnsName, "/path-a")
			By(fmt.Sprintf("expect %v returns %v", url, "backend-a"), func() {
				err = tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-a")))
				Expect(err).NotTo(HaveOccurred())
			})

			url = fmt.Sprintf("http://%v%v", dnsName, "/path-b")
			By(fmt.Sprintf("expect %v returns %v", url, "backend-b"), func() {
				err = tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-b")))
				Expect(err).NotTo(HaveOccurred())
			})

			url = fmt.Sprintf("http://%v%v", dnsName, "/path-c")
			By(fmt.Sprintf("expect %v returns %v", url, "backend-c"), func() {
				err = tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-c")))
				Expect(err).NotTo(HaveOccurred())
			})

			url = fmt.Sprintf("http://%v%v", dnsName, "/path-d")
			By(fmt.Sprintf("expect %v returns %v", url, "backend-d"), func() {
				err = tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-d")))
				Expect(err).NotTo(HaveOccurred())
			})

			url = fmt.Sprintf("http://%v%v", dnsName, "/path-e")
			By(fmt.Sprintf("expect %v returns %v", url, "backend-e"), func() {
				err = tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-e")))
				Expect(err).NotTo(HaveOccurred())
			})

			err = stack.Cleanup(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func expectDNSNameFromIngressNonEmpty(ctx context.Context, f *framework.Framework, stack *multiPathBackendStack, nsID string, ingID string) string {
	ing := stack.FindIngress(nsID, ingID)
	Expect(ing).NotTo(BeNil())
	err := f.K8sClient.Get(ctx, k8s.NamespacedName(ing), ing)
	Expect(err).NotTo(HaveOccurred())

	dnsName := FindIngressDNSName(ing)
	Expect(dnsName).NotTo(BeEmpty())
	return dnsName
}

func expectDNSNameEventuallyAvailable(ctx context.Context, f *framework.Framework, dnsName string) {
	lbARN, err := f.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
	Expect(err).NotTo(HaveOccurred())
	err = f.LBManager.WaitUntilLoadBalancerAvailable(ctx, lbARN)
	Expect(err).NotTo(HaveOccurred())

	err = utils.WaitUntilDNSNameAvailable(ctx, dnsName)
	Expect(err).NotTo(HaveOccurred())
}
