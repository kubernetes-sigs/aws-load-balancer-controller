package logdelivery

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/e2e/gateway/test_resources"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/manifest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	ingressAnnotation = "alb.ingress.kubernetes.io/log-delivery"
	serviceAnnotation = "service.beta.kubernetes.io/aws-load-balancer-log-delivery"
)

var _ = Describe("vended log delivery", Label("LogDelivery"), func() {
	var ctx context.Context
	var namespace *corev1.Namespace
	BeforeEach(func() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 45*time.Minute)
		DeferCleanup(cancel)
		var err error
		namespace, err = tf.NSManager.AllocateNamespace(ctx, "lbc-log-delivery")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
			defer cancel()
			Expect(client.IgnoreNotFound(tf.K8sClient.Delete(cleanupCtx, namespace))).To(Succeed())
			Expect(tf.NSManager.WaitUntilNamespaceDeleted(cleanupCtx, namespace)).To(Succeed())
		})
	})

	deployIngress := func(configs []logdeliverymodel.Config) (*resourceFixture, *networking.Ingress, string) {
		deployment, service := manifest.NewFixedResponseServiceBuilder().Build(namespace.Name, "app", tf.Options.TestImageRegistry)
		class := &networking.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: namespace.Name}, Spec: networking.IngressClassSpec{Controller: "ingress.k8s.aws/alb"}}
		annotations := map[string]string{
			"alb.ingress.kubernetes.io/scheme":      "internet-facing",
			"alb.ingress.kubernetes.io/target-type": "ip",
			ingressAnnotation:                       encodeConfigs(configs),
		}
		if tf.Options.IPFamily == framework.IPv6 {
			annotations["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
		}
		pathType := networking.PathTypePrefix
		ingress := manifest.NewIngressBuilder().WithIngressClassName(class.Name).WithAnnotations(annotations).AddHTTPRoute("", networking.HTTPIngressPath{
			Path: "/", PathType: &pathType, Backend: networking.IngressBackend{Service: &networking.IngressServiceBackend{Name: service.Name, Port: networking.ServiceBackendPort{Number: 80}}},
		}).Build(namespace.Name, "ingress")
		fixture := newFixture(ctx, class, deployment, service, ingress)
		for _, cfg := range configs {
			if cfg.DeliveryDestinationARN != nil {
				fixture.externalARN = *cfg.DeliveryDestinationARN
			}
		}
		return fixture, ingress, fixture.waitForLB(ctx, ingress)
	}

	It("delivers ALB access logs to CloudWatch, shares destinations, and removes an empty configuration", func() {
		configs := []logdeliverymodel.Config{
			{LogType: logdeliverymodel.LogTypeALBAccessLogs, DestinationARN: &tf.Options.LogDeliveryLogGroupARN, OutputFormat: awssdk.String("json")},
			{LogType: logdeliverymodel.LogTypeALBConnectionLogs, DestinationARN: &tf.Options.LogDeliveryLogGroupARN, OutputFormat: awssdk.String("json")},
			{LogType: logdeliverymodel.LogTypeALBHealthCheckLogs, DestinationARN: awssdk.String(s3Destination(namespace.Name)), OutputFormat: awssdk.String("parquet")},
		}
		fixture, ingress, dnsName := deployIngress(configs)
		By("checking all three log types and the shared CloudWatch destination")
		state := fixture.expectState(ctx, configs)
		Expect(state.destinations).To(HaveLen(2))
		By("generating requests and waiting for an access-log record in the supplied log group")
		path := "log-delivery-" + namespace.Name
		expectLogEvents(ctx, dnsName, path, path, false)
		By("removing every delivery while leaving the load balancer available")
		patchAnnotation(ctx, ingress, ingressAnnotation, "[]")
		expectDeleted(ctx, state, "")
		Expect(tf.LBManager.GetLoadBalancerFromARN(ctx, fixture.lbARN)).NotTo(BeNil())
		Eventually(func() error { return sendTraffic(ctx, dnsName, path, false) }, reconcileTimeout, pollInterval).Should(Succeed())
	})

	It("updates S3 delivery settings in place, replaces the output format, and cleans up on Ingress deletion", func() {
		configs := []logdeliverymodel.Config{{
			LogType: logdeliverymodel.LogTypeALBAccessLogs, DestinationARN: awssdk.String(s3Destination(namespace.Name)),
			OutputFormat: awssdk.String("plain"), FieldDelimiter: awssdk.String("\t"),
			S3DeliveryConfiguration: &logdeliverymodel.S3DeliveryConfiguration{SuffixPath: awssdk.String("e2e-initial"), EnableHiveCompatiblePath: awssdk.Bool(false)},
		}}
		fixture, ingress, _ := deployIngress(configs)
		original := fixture.expectState(ctx, configs)
		By("updating field delimiter and S3 path without replacing the delivery")
		configs[0].FieldDelimiter = awssdk.String(",")
		configs[0].S3DeliveryConfiguration.SuffixPath = awssdk.String("e2e-updated")
		configs[0].S3DeliveryConfiguration.EnableHiveCompatiblePath = awssdk.Bool(true)
		patchAnnotation(ctx, ingress, ingressAnnotation, encodeConfigs(configs))
		updated := fixture.expectState(ctx, configs)
		Expect(updated.deliveries).To(HaveKey(firstDeliveryID(original)))
		By("changing the format to Parquet and removing the previous delivery and destination")
		configs[0].OutputFormat = awssdk.String("parquet")
		configs[0].FieldDelimiter = nil
		patchAnnotation(ctx, ingress, ingressAnnotation, encodeConfigs(configs))
		replaced := fixture.expectState(ctx, configs)
		Expect(replaced.deliveries).NotTo(HaveKey(firstDeliveryID(original)))
		original.sources = nil // The same source continues serving the replacement delivery.
		expectDeleted(ctx, original, "")
		By("deleting the Ingress and verifying all remaining delivery resources disappear")
		Expect(tf.K8sClient.Delete(ctx, ingress)).To(Succeed())
		expectDeleted(ctx, replaced, "")
	})

	It("preserves a supplied existing delivery destination after removing the annotation", func() {
		if tf.Options.LogDeliveryExistingDestinationARN == "" {
			Skip("requires --log-delivery-existing-destination-arn")
		}
		parsed, err := arn.Parse(tf.Options.LogDeliveryExistingDestinationARN)
		Expect(err).NotTo(HaveOccurred())
		groupARN, err := arn.Parse(tf.Options.LogDeliveryLogGroupARN)
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed.Service).To(Equal("logs"))
		Expect(parsed.Region).To(Equal(groupARN.Region))
		Expect(parsed.AccountID).To(Equal(groupARN.AccountID), "this ownership test reads a destination in the test account")
		name := strings.TrimPrefix(parsed.Resource, "delivery-destination:")
		before, err := logsClient.GetDeliveryDestination(ctx, &cloudwatchlogssdk.GetDeliveryDestinationInput{Name: awssdk.String(name)})
		Expect(err).NotTo(HaveOccurred())
		Expect(awssdk.ToString(before.DeliveryDestination.Arn)).To(Equal(tf.Options.LogDeliveryExistingDestinationARN))
		configs := []logdeliverymodel.Config{{LogType: logdeliverymodel.LogTypeALBAccessLogs, DeliveryDestinationARN: &tf.Options.LogDeliveryExistingDestinationARN}}
		fixture, ingress, _ := deployIngress(configs)
		fixture.externalARN = tf.Options.LogDeliveryExistingDestinationARN
		state := fixture.expectState(ctx, configs)
		patchAnnotation(ctx, ingress, ingressAnnotation, "")
		expectDeleted(ctx, state, fixture.externalARN)
		after, err := logsClient.GetDeliveryDestination(ctx, &cloudwatchlogssdk.GetDeliveryDestinationInput{Name: awssdk.String(name)})
		Expect(err).NotTo(HaveOccurred())
		Expect(after.DeliveryDestination).To(Equal(before.DeliveryDestination))
	})

	It("delivers TLS NLB Service access logs to CloudWatch and configures an S3 delivery", func() {
		certificate := requireCertificate()
		configs := []logdeliverymodel.Config{
			{LogType: logdeliverymodel.LogTypeNLBAccessLogs, DestinationARN: &tf.Options.LogDeliveryLogGroupARN, OutputFormat: awssdk.String("json")},
			{LogType: logdeliverymodel.LogTypeNLBAccessLogs, DestinationARN: awssdk.String(s3Destination(namespace.Name)), OutputFormat: awssdk.String("parquet")},
		}
		annotations := map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
			"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
			"service.beta.kubernetes.io/aws-load-balancer-scheme":          "internet-facing",
			"service.beta.kubernetes.io/aws-load-balancer-ssl-cert":        certificate,
			"service.beta.kubernetes.io/aws-load-balancer-ssl-ports":       "443",
			serviceAnnotation: encodeConfigs(configs),
		}
		if tf.Options.IPFamily == framework.IPv6 {
			annotations["service.beta.kubernetes.io/aws-load-balancer-ip-address-type"] = "dualstack"
		}
		deployment, service := manifest.NewFixedResponseServiceBuilder().WithServiceType(corev1.ServiceTypeLoadBalancer).WithPort(443).WithServiceAnnotations(annotations).Build(namespace.Name, "nlb", tf.Options.TestImageRegistry)
		fixture := newFixture(ctx, deployment, service)
		dnsName := fixture.waitForLB(ctx, service)
		state := fixture.expectState(ctx, configs)
		Expect(state.sources).To(HaveLen(1))
		Expect(state.destinations).To(HaveLen(2))
		By("sending TLS traffic and waiting for an NLB record containing this load balancer's ID")
		marker := fixture.lbARN[strings.LastIndex(fixture.lbARN, "/")+1:]
		expectLogEvents(ctx, dnsName, "log-delivery-"+namespace.Name, marker, true)
		patchAnnotation(ctx, service, serviceAnnotation, "[]")
		expectDeleted(ctx, state, "")
	})

	for _, network := range []bool{false, true} {
		name := "ALB"
		if network {
			name = "NLB"
		}
		It("configures and removes "+name+" Gateway log delivery through LoadBalancerConfiguration", func() {
			if !tf.Options.EnableGatewayTests {
				Skip("requires --enable-gateway-tests and the Gateway API/controller CRDs")
			}
			logType := logdeliverymodel.LogTypeALBAccessLogs
			controllerName := test_resources.ALBGatewayControllerName
			listener := gwv1.Listener{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType}
			var certificate string
			if network {
				certificate = requireCertificate()
				logType = logdeliverymodel.LogTypeNLBAccessLogs
				controllerName = test_resources.NLBGatewayControllerName
				listener = gwv1.Listener{Name: "tls", Port: 443, Protocol: gwv1.TLSProtocolType, TLS: &gwv1.ListenerTLSConfig{Mode: new(gwv1.TLSModeTerminate), CertificateRefs: []gwv1.SecretObjectReference{{Name: "tls-cert"}}}}
			}
			configs := []logdeliverymodel.Config{{LogType: logType, DestinationARN: &tf.Options.LogDeliveryLogGroupARN, OutputFormat: awssdk.String("json")}}
			deployment, service := manifest.NewFixedResponseServiceBuilder().Build(namespace.Name, "app", tf.Options.TestImageRegistry)
			configuration := &elbv2gw.LoadBalancerConfiguration{ObjectMeta: metav1.ObjectMeta{Namespace: namespace.Name, Name: "logs"}, Spec: elbv2gw.LoadBalancerConfigurationSpec{
				Scheme: new(elbv2gw.LoadBalancerSchemeInternetFacing), LogDelivery: []elbv2gw.LogDeliveryConfiguration{{LogType: logType, DestinationArn: &tf.Options.LogDeliveryLogGroupARN, OutputFormat: awssdk.String("json")}},
			}}
			if tf.Options.IPFamily == framework.IPv6 {
				configuration.Spec.IpAddressType = new(elbv2gw.LoadBalancerIpAddressTypeDualstack)
			}
			if network {
				configuration.Spec.ListenerConfigurations = &[]elbv2gw.ListenerConfiguration{{ProtocolPort: "TLS:443", DefaultCertificate: &certificate}}
			}
			class := &gwv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: namespace.Name}, Spec: gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(controllerName)}}
			gateway := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: namespace.Name, Name: "gateway"}, Spec: gwv1.GatewaySpec{
				GatewayClassName: gwv1.ObjectName(class.Name), Listeners: []gwv1.Listener{listener}, Infrastructure: &gwv1.GatewayInfrastructure{ParametersRef: &gwv1.LocalParametersReference{Group: "gateway.k8s.aws", Kind: "LoadBalancerConfiguration", Name: configuration.Name}},
			}}
			parents := []gwv1.ParentReference{{Name: gwv1.ObjectName(gateway.Name)}}
			backend := gwv1.BackendRef{BackendObjectReference: gwv1.BackendObjectReference{Name: gwv1.ObjectName(service.Name), Port: new(gwv1.PortNumber(80))}}
			var route client.Object
			if network {
				route = &gwv1.TCPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: namespace.Name, Name: "route"}, Spec: gwv1.TCPRouteSpec{CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: parents}, Rules: []gwv1.TCPRouteRule{{BackendRefs: []gwv1.BackendRef{backend}}}}}
			} else {
				route = &gwv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: namespace.Name, Name: "route"}, Spec: gwv1.HTTPRouteSpec{CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: parents}, Rules: []gwv1.HTTPRouteRule{{BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: backend}}}}}}
			}
			fixture := newFixture(ctx, deployment, service, configuration, class, gateway, route)
			dnsName := fixture.waitForLB(ctx, gateway)
			state := fixture.expectState(ctx, configs)
			Eventually(func() error { return sendTraffic(ctx, dnsName, "log-delivery-"+namespace.Name, network) }, reconcileTimeout, pollInterval).Should(Succeed())
			Expect(tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(configuration), configuration)).To(Succeed())
			base := configuration.DeepCopy()
			configuration.Spec.LogDelivery = nil
			Expect(tf.K8sClient.Patch(ctx, configuration, client.MergeFrom(base))).To(Succeed())
			expectDeleted(ctx, state, "")
		})
	}
})

func encodeConfigs(configs []logdeliverymodel.Config) string {
	encoded, err := json.Marshal(configs)
	Expect(err).NotTo(HaveOccurred())
	return string(encoded)
}

func s3Destination(namespace string) string {
	return strings.TrimRight(tf.Options.LogDeliveryS3BucketARN, "/") + "/" + namespace
}

func requireCertificate() string {
	if tf.Options.CertificateARNs == "" {
		Skip("NLB log delivery requires --certificate-arns for a TLS listener")
	}
	return strings.TrimSpace(strings.Split(tf.Options.CertificateARNs, ",")[0])
}

func firstDeliveryID(state deliveryState) string {
	Expect(state.deliveries).To(HaveLen(1))
	for id := range state.deliveries {
		return id
	}
	return ""
}
