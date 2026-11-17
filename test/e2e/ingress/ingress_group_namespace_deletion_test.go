package ingress

import (
	"context"
	"fmt"
	"strings"
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
		url := fmt.Sprintf("http://%v%v", dnsName, "/path-a")
		Eventually(func() error {
			return tf.HTTPVerifier.VerifyURL(url, http.ResponseBodyMatches([]byte("backend-a")))
		}, utils.IngressReconcileTimeout, utils.PollIntervalMedium).Should(Succeed())

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
		Expect(tf.K8sClient.Delete(ctx, holdNS)).To(Succeed())
		Eventually(func(g Gomega) {
			ns := &corev1.Namespace{}
			g.Expect(tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(holdNS), ns)).To(Succeed())
			g.Expect(ns.Status.Phase).To(Equal(corev1.NamespaceTerminating))
		}, 1*time.Minute, 2*time.Second).Should(Succeed())

		By("join the Ingress in the terminating namespace to the IngressGroup")
		Expect(patchIngressAnnotations(ctx, ingX, groupMemberAnnotations(groupName))).To(Succeed())

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

		By("expect the new member to reconcile successfully with no terminating-namespace failure")
		// The new Ingress has no earlier events, so a SuccessfullyReconciled event on it comes from a reconcile
		// that loaded it. The group controller records that event only after the whole deployment succeeds,
		// including every TargetGroupBinding.
		Eventually(func(g Gomega) {
			g.Expect(terminatingNamespaceDeployFailures(ctx, newIng)).To(BeEmpty())
			g.Expect(reconcileSuccessCount(ctx, newIng)).To(BeNumerically(">", 0))
		}, utils.IngressReconcileTimeout, utils.PollIntervalShort).Should(Succeed())
		Eventually(func() error {
			return tf.HTTPVerifier.VerifyURL(fmt.Sprintf("http://%v/path-b", dnsName), http.ResponseBodyMatches([]byte("backend-a")))
		}, utils.IngressReconcileTimeout, utils.PollIntervalMedium).Should(Succeed())
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

func patchIngressAnnotations(ctx context.Context, ing *networking.Ingress, annotations map[string]string) error {
	current := &networking.Ingress{}
	if err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(ing), current); err != nil {
		return err
	}
	updated := current.DeepCopy()
	if updated.Annotations == nil {
		updated.Annotations = map[string]string{}
	}
	for key, value := range annotations {
		updated.Annotations[key] = value
	}
	return tf.K8sClient.Patch(ctx, updated, client.MergeFrom(current))
}

// terminatingNamespaceDeployFailures returns the FailedDeployModel event messages on ing that were caused
// by creating an object in a namespace that is being deleted.
func terminatingNamespaceDeployFailures(ctx context.Context, ing *networking.Ingress) []string {
	var failures []string
	for _, event := range ingressEvents(ctx, ing, k8s.IngressEventReasonFailedDeployModel) {
		if strings.Contains(event.Message, "because it is being terminated") {
			failures = append(failures, event.Message)
		}
	}
	return failures
}

// reconcileSuccessCount returns how many times the controller recorded a successful reconcile on ing.
func reconcileSuccessCount(ctx context.Context, ing *networking.Ingress) int32 {
	var count int32
	for _, event := range ingressEvents(ctx, ing, k8s.IngressEventReasonSuccessfullyReconciled) {
		count += event.Count
	}
	return count
}

func ingressEvents(ctx context.Context, ing *networking.Ingress, reason string) []corev1.Event {
	eventList := &corev1.EventList{}
	Expect(tf.K8sClient.List(ctx, eventList, client.InNamespace(ing.Namespace))).To(Succeed())
	var events []corev1.Event
	for _, event := range eventList.Items {
		if event.InvolvedObject.Kind == "Ingress" && event.InvolvedObject.Name == ing.Name && event.Reason == reason {
			events = append(events, event)
		}
	}
	return events
}
