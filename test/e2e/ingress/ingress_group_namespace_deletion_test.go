package ingress

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/http"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/manifest"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("IngressGroup member namespace deletion", func() {
	// While a namespace is Terminating, the API server rejects new objects in it, but its Ingress has no
	// deletionTimestamp until the namespace controller reaches it. The test holds a namespace in that state
	// with a ValidatingAdmissionPolicy that blocks deleting its Ingress and Service, joins its Ingress to an
	// IngressGroup, and then adds a member in another namespace. The group must keep reconciling.
	It("other members keep reconciling while a member's namespace is terminating", func() {
		ctx := context.Background()
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
		Expect(stack.Deploy(ctx, tf)).To(Succeed())
		defer func() {
			Expect(stack.Cleanup(ctx, tf)).To(Succeed())
		}()
		dnsName := expectDNSNameFromIngressNonEmpty(ctx, tf, stack, "ns-1", "ing-1")
		expectDNSNameEventuallyAvailable(ctx, tf, dnsName)

		By("create an Ingress that this controller does not manage yet in a second namespace")
		holdNS, err := tf.NSManager.AllocateNamespace(ctx, "aws-lb-e2e")
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			Expect(client.IgnoreNotFound(tf.K8sClient.Delete(ctx, holdNS))).To(Succeed())
			waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			Expect(tf.NSManager.WaitUntilNamespaceDeleted(waitCtx, holdNS)).To(Succeed())
		}()
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: holdNS.Name, Name: "backend-x"},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app.kubernetes.io/name": "backend-x"},
				Ports:    []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(8080), Protocol: corev1.ProtocolTCP}},
			},
		}
		Expect(tf.K8sClient.Create(ctx, svc)).To(Succeed())
		exact := networking.PathTypeExact
		ingX := manifest.NewIngressBuilder().
			WithAnnotations(map[string]string{"kubernetes.io/ingress.class": "e2e-unmanaged"}).
			AddHTTPRoute("", networking.HTTPIngressPath{
				Path:     "/path-x",
				PathType: &exact,
				Backend: networking.IngressBackend{Service: &networking.IngressServiceBackend{
					Name: svc.Name,
					Port: networking.ServiceBackendPort{Number: 80},
				}},
			}).
			Build(holdNS.Name, "ing-x")
		Expect(tf.K8sClient.Create(ctx, ingX)).To(Succeed())

		By("hold the second namespace in Terminating")
		// Deferred calls run in reverse order: the binding goes first, so the namespace can finish deleting.
		policy, binding := buildDeletionHoldPolicy(holdNS.Name)
		Expect(tf.K8sClient.Create(ctx, policy)).To(Succeed())
		defer func() {
			Expect(client.IgnoreNotFound(tf.K8sClient.Delete(ctx, policy))).To(Succeed())
		}()
		Expect(tf.K8sClient.Create(ctx, binding)).To(Succeed())
		defer func() {
			Expect(client.IgnoreNotFound(tf.K8sClient.Delete(ctx, binding))).To(Succeed())
		}()
		Eventually(func() error {
			return tf.K8sClient.Delete(ctx, svc.DeepCopy(), client.DryRunAll)
		}, 1*time.Minute, 2*time.Second).Should(MatchError(ContainSubstring(deletionHoldMessage)))
		// The API server sets the namespace phase to Terminating before the delete call returns.
		Expect(tf.K8sClient.Delete(ctx, holdNS)).To(Succeed())

		By("join the Ingress in the terminating namespace to the IngressGroup")
		oldIngX := ingX.DeepCopy()
		ingX.Annotations = groupMemberAnnotations(groupName)
		Expect(tf.K8sClient.Patch(ctx, ingX, client.MergeFrom(oldIngX))).To(Succeed())

		By("add a member to the IngressGroup in the long-lived namespace")
		newIng := manifest.NewIngressBuilder().
			WithAnnotations(groupMemberAnnotations(groupName)).
			AddHTTPRoute("", networking.HTTPIngressPath{
				Path:     "/path-b",
				PathType: &exact,
				Backend: networking.IngressBackend{Service: &networking.IngressServiceBackend{
					Name: "backend-a",
					Port: networking.ServiceBackendPort{Number: 80},
				}},
			}).
			Build(stack.FindIngress("ns-1", "ing-1").Namespace, "ing-2")
		Expect(tf.K8sClient.Create(ctx, newIng)).To(Succeed())
		defer func() {
			Expect(client.IgnoreNotFound(tf.K8sClient.Delete(ctx, newIng))).To(Succeed())
		}()

		By("expect the new member to reconcile successfully")
		// ing-x joined before ing-2 was created, through the same Ingress watch, so the reconcile that sees ing-2
		// also sees ing-x. The controller records SuccessfullyReconciled only on the members of the reconcile, and
		// only after the whole group deploys, including every TargetGroupBinding.
		Eventually(func(g Gomega) {
			g.Expect(ingressEventMessages(ctx, g, newIng, k8s.IngressEventReasonSuccessfullyReconciled)).NotTo(BeEmpty(),
				"deploy failures: %v", ingressEventMessages(ctx, g, newIng, k8s.IngressEventReasonFailedDeployModel))
		}, utils.IngressReconcileTimeout, utils.PollIntervalShort).Should(Succeed())
		Eventually(func() error {
			return tf.HTTPVerifier.VerifyURL(fmt.Sprintf("http://%v/path-b", dnsName), http.ResponseBodyMatches([]byte("backend-a")))
		}, utils.PollTimeoutMedium, utils.PollIntervalMedium).Should(Succeed())
	})
})

const deletionHoldMessage = "held by aws-load-balancer-controller e2e test"

// buildDeletionHoldPolicy blocks deleting Ingresses and Services in the namespace, so that the namespace
// stays Terminating with its Ingress still present.
func buildDeletionHoldPolicy(namespace string) (*admissionregistrationv1.ValidatingAdmissionPolicy, *admissionregistrationv1.ValidatingAdmissionPolicyBinding) {
	name := "e2e-hold-" + namespace
	failurePolicy := admissionregistrationv1.Fail
	policy := &admissionregistrationv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: &failurePolicy,
			MatchConstraints: &admissionregistrationv1.MatchResources{
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregistrationv1.RuleWithOperations{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Delete},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"", "networking.k8s.io"},
							APIVersions: []string{"v1"},
							Resources:   []string{"services", "ingresses"},
						},
					},
				}},
			},
			Validations: []admissionregistrationv1.Validation{{Expression: "false", Message: deletionHoldMessage}},
		},
	}
	binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName:        policy.Name,
			ValidationActions: []admissionregistrationv1.ValidationAction{admissionregistrationv1.Deny},
			MatchResources: &admissionregistrationv1.MatchResources{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": namespace},
				},
			},
		},
	}
	return policy, binding
}

// groupMemberAnnotations makes an Ingress an internet-facing member of the IngressGroup.
func groupMemberAnnotations(groupName string) map[string]string {
	annotations := map[string]string{
		"kubernetes.io/ingress.class":           "alb",
		"alb.ingress.kubernetes.io/scheme":      "internet-facing",
		"alb.ingress.kubernetes.io/target-type": "ip",
		"alb.ingress.kubernetes.io/group.name":  groupName,
	}
	if tf.Options.IPFamily == framework.IPv6 {
		annotations["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
	}
	return annotations
}

// ingressEventMessages returns the messages of the events on ing with the reason.
func ingressEventMessages(ctx context.Context, g Gomega, ing *networking.Ingress, reason string) []string {
	events := &corev1.EventList{}
	g.Expect(tf.K8sClient.List(ctx, events, client.InNamespace(ing.Namespace), client.MatchingFields{
		"involvedObject.kind": "Ingress",
		"involvedObject.name": ing.Name,
		"reason":              reason,
	})).To(Succeed())
	var messages []string
	for _, event := range events.Items {
		messages = append(messages, event.Message)
	}
	return messages
}
