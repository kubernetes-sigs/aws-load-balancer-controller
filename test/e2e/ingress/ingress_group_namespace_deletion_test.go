package ingress

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("IngressGroup member namespace deletion", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// A namespace that holds one member of a shared IngressGroup is deleted right after its Ingress is
	// created. The namespace controller deletes the Ingress only after it has gone through the other
	// resource types in the namespace, so for several seconds the namespace is Terminating while its
	// Ingress has no deletionTimestamp. The controller must not try to create resources in that namespace,
	// and the other members of the IngressGroup must keep reconciling.
	It("other members keep reconciling and the deleted namespace finishes deleting", func() {
		groupName := fmt.Sprintf("e2e-group.%v", utils.RandomDNS1123Label(8))
		stack := NewMultiPathBackendStack(map[string]NamespacedResourcesConfig{
			"ns-1": {
				IngCFGs: map[string]MultiPathIngressConfig{
					"ing-1": {
						GroupName: groupName,
						PathCFGs:  []PathConfig{{Path: "/path-a", BackendID: "backend-a"}},
					},
				},
				BackendCFGs: map[string]BackendConfig{
					"backend-a": {Replicas: 2, TargetType: elbv2model.TargetTypeIP, HTTPBody: "backend-a"},
				},
			},
		}, false)

		By("deploy the long-lived member of the IngressGroup")
		err := stack.Deploy(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			Expect(stack.Cleanup(ctx, tf)).To(Succeed())
		}()

		dnsName := expectDNSNameFromIngressNonEmpty(ctx, tf, stack, "ns-1", "ing-1")
		expectDNSNameEventuallyAvailable(ctx, tf, dnsName)
		url := fmt.Sprintf("http://%v%v", dnsName, "/path-a")
		Eventually(func() error {
			return tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-a")))
		}, utils.IngressReconcileTimeout, utils.PollIntervalMedium).Should(Succeed())

		By("create a short-lived member namespace and delete it right after its Ingress is created")
		shortLivedNS, err := tf.NSManager.AllocateNamespace(ctx, "aws-lb-e2e")
		Expect(err).NotTo(HaveOccurred())
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: shortLivedNS.Name, Name: "backend-x"},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app.kubernetes.io/name": "backend-x"},
				Ports:    []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: corev1.ProtocolTCP}},
			},
		}
		Expect(tf.K8sClient.Create(ctx, svc)).To(Succeed())
		exact := networking.PathTypeExact
		ing := &networking.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: shortLivedNS.Name,
				Name:      "ing-x",
				Annotations: map[string]string{
					"kubernetes.io/ingress.class":           "alb",
					"alb.ingress.kubernetes.io/scheme":      "internet-facing",
					"alb.ingress.kubernetes.io/target-type": "ip",
					"alb.ingress.kubernetes.io/group.name":  groupName,
				},
			},
			Spec: networking.IngressSpec{
				Rules: []networking.IngressRule{{
					IngressRuleValue: networking.IngressRuleValue{HTTP: &networking.HTTPIngressRuleValue{
						Paths: []networking.HTTPIngressPath{{
							Path:     "/path-x",
							PathType: &exact,
							Backend: networking.IngressBackend{Service: &networking.IngressServiceBackend{
								Name: svc.Name,
								Port: networking.ServiceBackendPort{Number: 80},
							}},
						}},
					}},
				}},
			},
		}
		if tf.Options.IPFamily == framework.IPv6 {
			ing.Annotations["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
		}
		Expect(tf.K8sClient.Create(ctx, ing)).To(Succeed())
		deleteStart := metav1.Now()
		Expect(tf.K8sClient.Delete(ctx, shortLivedNS)).To(Succeed())

		By("expect the short-lived namespace to finish deleting")
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		Expect(tf.NSManager.WaitUntilNamespaceDeleted(waitCtx, shortLivedNS)).To(Succeed())

		By("expect no deploy failure caused by the terminating namespace on the remaining member")
		remainingIng := stack.FindIngress("ns-1", "ing-1")
		Consistently(func() []string {
			return terminatingNamespaceDeployFailures(ctx, remainingIng, deleteStart)
		}, 1*time.Minute, utils.PollIntervalMedium).Should(BeEmpty())

		By("expect the remaining member to keep serving traffic")
		Expect(tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-a")))).To(Succeed())
	})
})

// terminatingNamespaceDeployFailures returns the FailedDeployModel events recorded on ing after since
// that were caused by creating content in a namespace that is being deleted.
func terminatingNamespaceDeployFailures(ctx context.Context, ing *networking.Ingress, since metav1.Time) []string {
	eventList := &corev1.EventList{}
	Expect(tf.K8sClient.List(ctx, eventList, client.InNamespace(ing.Namespace))).To(Succeed())
	var failures []string
	for _, event := range eventList.Items {
		if event.InvolvedObject.Kind != "Ingress" || event.InvolvedObject.Name != ing.Name {
			continue
		}
		if event.Reason != k8s.IngressEventReasonFailedDeployModel || event.LastTimestamp.Before(&since) {
			continue
		}
		if strings.Contains(event.Message, "because it is being terminated") {
			failures = append(failures, event.Message)
		}
	}
	return failures
}
