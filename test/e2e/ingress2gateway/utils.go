package ingress2gateway

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	planStabilizeTimeout = 3 * time.Minute
	// shellCommandTimeout caps how long any kubectl/lbc-migrate invocation may
	// run before the test gives up. Without it, a hung subprocess would stall
	// the entire suite indefinitely.
	shellCommandTimeout = 2 * time.Minute
)

// runMigrateTool runs lbc-migrate as a subprocess with the given extra flags.
// When namespace is non-empty, --from-cluster and --namespace are added automatically.
func runMigrateTool(namespace, outputDir string, extraArgs ...string) error {
	args := []string{"--output-dir", outputDir}
	if namespace != "" {
		args = append(args, "--from-cluster", "--namespaces", namespace)
	}
	args = append(args, extraArgs...)
	return runLBCMigrate(args)
}

// runLBCMigrate executes lbc-migrate with the given args and wraps the output on failure.
func runLBCMigrate(args []string) error {
	out, err := runShell("lbc-migrate", args...)
	if err != nil {
		return fmt.Errorf("lbc-migrate failed: %v\noutput: %s", err, string(out))
	}
	return nil
}

// runShell runs name with args under shellCommandTimeout and returns combined output.
func runShell(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shellCommandTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// expectDryRunPlan waits for the Ingress ALB to be fully provisioned (LB DNS populated
// in status), then reads the dry-run-plan annotation. Since the controller writes the
// annotation during model building (before deployment), DNS appearing in status means
// the full reconcile is complete and the annotation is finalized.
// Verifies the annotation is absent on all secondary group members.
func expectDryRunPlan(ctx context.Context, tf *framework.Framework, primary *networking.Ingress, secondaries ...*networking.Ingress) string {
	Eventually(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(primary), primary)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(findIngressDNS(primary)).NotTo(BeEmpty(),
			"primary ingress ALB DNS not yet populated (reconcile in progress)")
	}, utils.CertReconcileTimeout, utils.PollIntervalShort).Should(Succeed())

	plan := primary.Annotations[annotationDryRunPlan]
	Expect(plan).NotTo(BeEmpty(), "dry-run-plan annotation missing on primary ingress")

	for _, sec := range secondaries {
		err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(sec), sec)
		Expect(err).NotTo(HaveOccurred())
		Expect(sec.Annotations[annotationDryRunPlan]).To(BeEmpty(),
			"dry-run-plan annotation should be absent on secondary member %s/%s",
			sec.Namespace, sec.Name)
	}
	return plan
}

// expectGatewayDryRunPlan waits for the gateway controller to write a stable dry-run-plan
// annotation. The controller may reconcile multiple times as routes attach, so we poll
// until the plan stops changing (same value across 2 consecutive reads).
func expectGatewayDryRunPlan(ctx context.Context, tf *framework.Framework, gw *gwv1.Gateway) string {
	var plan string
	var lastPlan string
	stableCount := 0
	Eventually(func(g Gomega) {
		err := tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)
		g.Expect(err).NotTo(HaveOccurred())
		plan = gw.Annotations[gwDryRunPlan]
		g.Expect(plan).ShouldNot(BeEmpty())
		if plan == lastPlan {
			stableCount++
		} else {
			stableCount = 0
		}
		lastPlan = plan
		g.Expect(stableCount).To(BeNumerically(">=", 2),
			"dry-run plan not yet stable (controller still reconciling)")
	}, planStabilizeTimeout, utils.PollIntervalShort).Should(Succeed())
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
		out, err := runShell("kubectl", "apply", "-f", f)
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

// applyYAMLDirRecursive applies all YAML files in a directory tree using kubectl apply -R.
func applyYAMLDirRecursive(dir string) error {
	out, err := runShell("kubectl", "apply", "-R", "-f", dir)
	if err != nil {
		return fmt.Errorf("kubectl apply -R failed for %s: %v\noutput: %s", dir, err, out)
	}
	return nil
}

// runMigrateToolWithInputDir runs lbc-migrate with --input-dir.
func runMigrateToolWithInputDir(inputDir, outputDir string, extraArgs ...string) error {
	args := append([]string{"--output-dir", outputDir, "--input-dir", inputDir}, extraArgs...)
	return runLBCMigrate(args)
}

// runMigrateToolWithFile runs lbc-migrate with -f (file input mode).
func runMigrateToolWithFile(inputFile, outputDir string, extraArgs ...string) error {
	args := append([]string{"--output-dir", outputDir, "-f", inputFile}, extraArgs...)
	return runLBCMigrate(args)
}

// writeResourcesToTempFile serializes multiple k8s objects to a temp YAML file
// joined by "---\n" so the migration tool's -f mode can consume them.
func writeResourcesToTempFile(objects ...client.Object) string {
	docs := make([][]byte, 0, len(objects))
	for _, obj := range objects {
		docs = append(docs, marshalObjectToYAML(obj))
	}

	f, err := os.CreateTemp("", "i2g-input-*.yaml")
	Expect(err).NotTo(HaveOccurred())
	_, err = f.Write(bytes.Join(docs, []byte("---\n")))
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Close()).To(Succeed())
	return f.Name()
}

// writeResourcesToDir writes each k8s object as a separate YAML file in the given directory.
func writeResourcesToDir(dir string, objects ...client.Object) {
	for i, obj := range objects {
		filename := fmt.Sprintf("%03d-%s-%s.yaml", i, obj.GetNamespace(), obj.GetName())
		Expect(os.WriteFile(filepath.Join(dir, filename), marshalObjectToYAML(obj), 0644)).To(Succeed())
	}
}

// marshalObjectToYAML serializes obj as a YAML document with apiVersion/kind
// populated. controller-runtime clients leave TypeMeta empty when round-tripping
// objects, so we set it explicitly before marshaling.
func marshalObjectToYAML(obj client.Object) []byte {
	groupVersionKind := groupVersionKindForObject(obj)
	obj.GetObjectKind().SetGroupVersionKind(groupVersionKind)
	data, err := sigsyaml.Marshal(obj)
	Expect(err).NotTo(HaveOccurred(), "marshal %T: %v", obj, err)
	return data
}

// groupVersionKindForObject returns the GroupVersionKind for the supported test types.
func groupVersionKindForObject(obj client.Object) schema.GroupVersionKind {
	switch obj.(type) {
	case *networking.Ingress:
		return networking.SchemeGroupVersion.WithKind("Ingress")
	case *networking.IngressClass:
		return networking.SchemeGroupVersion.WithKind("IngressClass")
	case *corev1.Service:
		return corev1.SchemeGroupVersion.WithKind("Service")
	case *elbv2api.IngressClassParams:
		return elbv2api.GroupVersion.WithKind("IngressClassParams")
	default:
		Expect(obj).To(BeNil(), "unsupported object type %T for YAML marshaling", obj)
		return schema.GroupVersionKind{}
	}
}

// findServiceInNamespace fetches a Service object from the cluster.
func findServiceInNamespace(ctx context.Context, tf *framework.Framework, namespace, name string) *corev1.Service {
	svc := &corev1.Service{}
	err := tf.K8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, svc)
	Expect(err).NotTo(HaveOccurred())
	return svc
}
