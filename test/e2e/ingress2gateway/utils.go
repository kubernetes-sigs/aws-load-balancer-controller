package ingress2gateway

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/gomega"
	networking "k8s.io/api/networking/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// runMigrateTool runs lbc-migrate as a subprocess with the given extra flags.
// When namespace is non-empty, --from-cluster and --namespace are added automatically.
func runMigrateTool(namespace, outputDir string, extraArgs ...string) error {
	args := []string{"--output-dir", outputDir}
	if namespace != "" {
		args = append(args, "--from-cluster", "--namespaces", namespace)
	}
	args = append(args, extraArgs...)
	cmd := exec.Command("lbc-migrate", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lbc-migrate failed: %v\noutput: %s", err, string(out))
	}
	return nil
}

// expectDryRunPlan waits for the dry-run-plan annotation to appear on the primary ingress
// and verifies it is absent on all secondary members, using a single polling loop.
func expectDryRunPlan(ctx context.Context, tf *framework.Framework, primary *networking.Ingress, secondaries ...*networking.Ingress) string {
	var plan string
	Eventually(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(primary), primary)
		g.Expect(err).NotTo(HaveOccurred())
		plan = primary.Annotations[annotationDryRunPlan]
		g.Expect(plan).ShouldNot(BeEmpty())

		for _, sec := range secondaries {
			err = tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(sec), sec)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(sec.Annotations[annotationDryRunPlan]).To(BeEmpty())
		}
	}, utils.IngressReconcileTimeout, utils.PollIntervalShort).Should(Succeed())
	return plan
}

// expectGatewayDryRunPlan waits for the gateway controller to write the dry-run-plan annotation.
func expectGatewayDryRunPlan(ctx context.Context, tf *framework.Framework, gw *gwv1.Gateway) string {
	var plan string
	Eventually(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)
		g.Expect(err).NotTo(HaveOccurred())
		plan = gw.Annotations[gwDryRunPlan]
		g.Expect(plan).ShouldNot(BeEmpty())
	}, utils.CertReconcileTimeout, utils.PollIntervalShort).Should(Succeed())
	return plan
}

// expectNoALBForGateway verifies no ALB is created for a gateway (dry-run mode).
func expectNoALBForGateway(ctx context.Context, tf *framework.Framework, gw *gwv1.Gateway) {
	Consistently(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(gw.Status.Addresses).To(BeEmpty())
	}, utils.IngressReconcileTimeout, utils.PollIntervalShort).Should(Succeed())
}

// expectGatewayALBProvisioned waits for the gateway to get an ALB address and returns its ARN and DNS.
func expectGatewayALBProvisioned(ctx context.Context, tf *framework.Framework, gw *gwv1.Gateway) (string, string) {
	var lbDNS string
	Eventually(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(gw.Status.Addresses).NotTo(BeEmpty())
		lbDNS = gw.Status.Addresses[0].Value
		g.Expect(lbDNS).NotTo(BeEmpty())
	}, utils.CertReconcileTimeout, utils.PollIntervalShort).Should(Succeed())

	lbARN, err := tf.LBManager.FindLoadBalancerByDNSName(ctx, lbDNS)
	Expect(err).NotTo(HaveOccurred())
	Expect(lbARN).NotTo(BeEmpty())
	return lbARN, lbDNS
}

// removeDryRunAnnotation patches the gateway to remove the dry-run annotation, triggering real reconciliation.
func removeDryRunAnnotation(ctx context.Context, tf *framework.Framework, gw *gwv1.Gateway) {
	gwOld := gw.DeepCopy()
	delete(gw.Annotations, gwDryRunAnnotation)
	err := tf.K8sClient.Patch(ctx, gw, client.MergeFrom(gwOld))
	Expect(err).NotTo(HaveOccurred())
}

// ExpectLBDNSResolvable waits for the LB DNS to become resolvable.
func ExpectLBDNSResolvable(ctx context.Context, tf *framework.Framework, lbDNS string) {
	tf.Logger.Info("waiting for LB DNS to be resolvable", "dns", lbDNS)
	dnsCtx, cancel := context.WithTimeout(ctx, tf.Options.DNSTimeout)
	defer cancel()
	err := utils.WaitUntilDNSNameAvailable(dnsCtx, lbDNS)
	Expect(err).NotTo(HaveOccurred())
}

// findGatewayInNamespace finds the first Gateway in the namespace.
func findGatewayInNamespace(ctx context.Context, tf *framework.Framework, namespace string) *gwv1.Gateway {
	gwList := &gwv1.GatewayList{}
	err := tf.K8sClient.List(ctx, gwList, client.InNamespace(namespace))
	Expect(err).NotTo(HaveOccurred())
	if len(gwList.Items) == 0 {
		return nil
	}
	return &gwList.Items[0]
}

// applyYAMLDir applies all YAML files in a directory using kubectl.
func applyYAMLDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return err
	}
	for _, f := range files {
		out, err := exec.Command("kubectl", "apply", "-f", f).CombinedOutput()
		if err != nil {
			return fmt.Errorf("kubectl apply failed for %s: %v\noutput: %s", f, err, out)
		}
	}
	return nil
}

// deleteGatewayResources deletes all Gateway API and LBC CRD resources in a namespace.
func deleteGatewayResources(ctx context.Context, tf *framework.Framework, namespace string) {
	ns := client.InNamespace(namespace)
	_ = tf.K8sClient.DeleteAllOf(ctx, &elbv2gw.ListenerRuleConfiguration{}, ns)
	_ = tf.K8sClient.DeleteAllOf(ctx, &elbv2gw.TargetGroupConfiguration{}, ns)
	_ = tf.K8sClient.DeleteAllOf(ctx, &elbv2gw.LoadBalancerConfiguration{}, ns)
	_ = tf.K8sClient.DeleteAllOf(ctx, &gwv1.HTTPRoute{}, ns)
	_ = tf.K8sClient.DeleteAllOf(ctx, &gwv1.Gateway{}, ns)
}

func findIngressDNS(ing *networking.Ingress) string {
	for _, i := range ing.Status.LoadBalancer.Ingress {
		if i.Hostname != "" {
			return i.Hostname
		}
	}
	return ""
}
