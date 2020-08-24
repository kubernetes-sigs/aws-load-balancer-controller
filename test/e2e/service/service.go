package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"math/big"
	"net/http"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"time"
)

const (
	ResourceTypeELBLoadBalancer = "elasticloadbalancing:loadbalancer"
)

type ServiceTest struct {
	Service *v1.Service
}

type TargetGroupHC struct {
	Protocol           string
	Path               string
	Port               string
	Interval           int64
	Timeout            int64
	HealthyThreshold   int64
	UnhealthyThreshold int64
}

type LoadBalancerExpectation struct {
	Type          string
	Scheme        string
	TargetType    string
	Listeners     map[string]string // listener port, protocol
	TargetGroups  map[string]string // target group port, protocol
	NumTargets    int
	TargetGroupHC *TargetGroupHC
}

func (m *ServiceTest) Create(ctx context.Context, f *framework.Framework, svc *v1.Service) error {
	err := f.K8sClient.Create(ctx, svc)
	if err != nil {
		return err
	}
	Eventually(func() (bool, error) {
		_, err := f.SVCManager.WaitUntilServiceActive(ctx, svc)
		return err == nil, err
	}, utils.PollTimeoutShort, utils.PollIntervalShort).Should(BeTrue())
	newSvc, err := f.SVCManager.WaitUntilServiceActive(ctx, svc)
	if err != nil {
		return err
	}
	m.Service = newSvc
	return nil
}

func (m *ServiceTest) Update(ctx context.Context, f *framework.Framework, svc *v1.Service, oldSvc *v1.Service) error {
	err := f.K8sClient.Patch(ctx, svc, client.MergeFrom(oldSvc))
	if err != nil {
		return err
	}
	newSvc, err := f.SVCManager.WaitUntilServiceActive(ctx, svc)
	m.Service = newSvc
	return nil
}

func (m *ServiceTest) Cleanup(ctx context.Context, f *framework.Framework, svc *v1.Service) error {
	if err := f.K8sClient.Delete(ctx, svc,
		client.PropagationPolicy(metav1.DeletePropagationForeground), client.GracePeriodSeconds(0)); err != nil {
		return err
	}
	if err := f.SVCManager.WaitUntilServiceDeleted(ctx, svc); err != nil {
		return err
	}
	return nil
}

func (m *ServiceTest) GetAwsLoadBalancerArns(ctx context.Context, f *framework.Framework) (lbArns []string, err error) {
	By("Querying Load Balancer ARNs", func() {
		// TODO: change resource tags to GA version
		tagFilters := []*resourcegroupstaggingapi.TagFilter{
			{
				Key:    aws.String("elbv2.k8s.aws/cluster"),
				Values: aws.StringSlice([]string{f.Options.ClusterName}),
			},
			{
				Key:    aws.String("service.k8s.aws/stack"),
				Values: aws.StringSlice([]string{k8s.NamespacedName(m.Service).String()}),
			},
			{
				Key:    aws.String("service.k8s.aws/resource"),
				Values: aws.StringSlice([]string{"LoadBalancer"}),
			},
		}

		req := &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters:          tagFilters,
			ResourceTypeFilters: aws.StringSlice([]string{ResourceTypeELBLoadBalancer}),
		}
		resources, err := f.Cloud.RGT().GetResourcesWithContext(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		for _, resource := range resources.ResourceTagMappingList {
			lbARN := aws.StringValue(resource.ResourceARN)
			lbArns = append(lbArns, lbARN)
		}
		Expect(len(lbArns)).To(Equal(1))
		Expect(lbArns[0]).ToNot(Equal(""))
	})
	return
}

func (m *ServiceTest) CheckLoadBalancerType(ctx context.Context, f *framework.Framework, lbArns []string, lbType, lbScheme string) error {
	By("Describing AWS Load Balancer", func() {
		lbs, err := f.Cloud.ELBV2().DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
			LoadBalancerArns: aws.StringSlice(lbArns),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(lbs.LoadBalancers)).To(Equal(1))
		lb := lbs.LoadBalancers[0]
		Expect(aws.StringValue(lb.Type)).To(Equal(lbType))
		Expect(aws.StringValue(lb.Scheme)).To(Equal(lbScheme))
	})
	return nil
}

func (m *ServiceTest) CheckLoadBalancerListeners(ctx context.Context, f *framework.Framework, lbArn string, expected LoadBalancerExpectation) error {
	By("Describing AWS Load Balancer Listeners", func() {
		listeners, err := f.Cloud.ELBV2().DescribeListenersWithContext(ctx, &elbv2.DescribeListenersInput{
			LoadBalancerArn: aws.String(lbArn),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(listeners.Listeners)).To(Equal(len(m.Service.Spec.Ports)))

		// Verify listeners port/protocol
		listenerPortSet := sets.NewString()
		for _, port := range m.Service.Spec.Ports {
			listenerPortSet.Insert(strconv.Itoa(int(port.Port)))
		}
		for _, ls := range listeners.Listeners {
			portStr := strconv.Itoa(int(aws.Int64Value(ls.Port)))
			Expect(listenerPortSet.Has(portStr)).To(BeTrue())
			Expect(aws.StringValue(ls.Protocol)).To(Equal(expected.Listeners[portStr]))
		}
	})
	return nil
}

func (m *ServiceTest) VerifyLoadBalancerAttributes(ctx context.Context, f *framework.Framework, expectedAttrs map[string]string) error {
	lbArns, err := m.GetAwsLoadBalancerArns(ctx, f)
	Expect(err).ToNot(HaveOccurred())
	resp, err := f.Cloud.ELBV2().DescribeLoadBalancerAttributesWithContext(ctx, &elbv2.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: aws.String(lbArns[0]),
	})
	Expect(err).ToNot(HaveOccurred())
	for _, attr := range resp.Attributes {
		if val, ok := expectedAttrs[aws.StringValue(attr.Key)]; ok && val != aws.StringValue(attr.Value) {
			return errors.Errorf("Attribute %v, expected %v, actual %v", aws.StringValue(attr.Key), val, aws.StringValue(attr.Value))
		}
	}
	return nil
}

func (m *ServiceTest) CheckTargetGroupHealth(ctx context.Context, f *framework.Framework, tgArn string, numTargets int) (bool, error) {

	resp, err := f.Cloud.ELBV2().DescribeTargetHealthWithContext(ctx, &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: aws.String(tgArn),
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(len(resp.TargetHealthDescriptions)).To(Equal(numTargets))

	healthy := true
	for _, thd := range resp.TargetHealthDescriptions {
		if aws.StringValue(thd.TargetHealth.State) != elbv2.TargetHealthStateEnumHealthy {
			healthy = false
			break
		}
	}
	return healthy, nil
}

func (m *ServiceTest) verifyTargetGroupHealthCheckConfig(tg *elbv2.TargetGroup, hc *TargetGroupHC) error {
	if hc != nil {
		Expect(aws.StringValue(tg.HealthCheckProtocol)).To(Equal(hc.Protocol))
		Expect(aws.StringValue(tg.HealthCheckPath)).To(Equal(hc.Path))
		Expect(aws.StringValue(tg.HealthCheckPort)).To(Equal(hc.Port))
		Expect(aws.Int64Value(tg.HealthCheckIntervalSeconds)).To(Equal(hc.Interval))
		Expect(aws.Int64Value(tg.HealthCheckTimeoutSeconds)).To(Equal(hc.Timeout))
		Expect(aws.Int64Value(tg.HealthyThresholdCount)).To(Equal(hc.HealthyThreshold))
		Expect(aws.Int64Value(tg.UnhealthyThresholdCount)).To(Equal(hc.UnhealthyThreshold))
	}
	return nil
}

func (m *ServiceTest) CheckTargetGroups(ctx context.Context, f *framework.Framework, lbArn string, expected LoadBalancerExpectation) error {
	tgArn := ""
	By("Querying for AWS Load Balancer target groups", func() {
		targetGroups, err := f.Cloud.ELBV2().DescribeTargetGroupsWithContext(ctx, &elbv2.DescribeTargetGroupsInput{
			LoadBalancerArn: aws.String(lbArn),
		})
		//TgtGroups
		Expect(err).NotTo(HaveOccurred())
		Expect(len(targetGroups.TargetGroups)).To(Equal(len(m.Service.Spec.Ports)))
		tgArn = aws.StringValue(targetGroups.TargetGroups[0].TargetGroupArn)
		for _, tg := range targetGroups.TargetGroups {
			Expect(aws.StringValue(tg.TargetType)).To(Equal(expected.TargetType))
			Expect(aws.StringValue(tg.Protocol)).To(Equal(expected.TargetGroups[strconv.Itoa(int(aws.Int64Value(tg.Port)))]))
			_, err := m.CheckTargetGroupHealth(ctx, f, aws.StringValue(tg.TargetGroupArn), expected.NumTargets)
			Expect(err).ToNot(HaveOccurred())
			err = m.verifyTargetGroupHealthCheckConfig(tg, expected.TargetGroupHC)
			Expect(err).ToNot(HaveOccurred())
		}
	})
	By("Waiting until targets are healthy", func() {
		Eventually(func() (bool, error) {
			return m.CheckTargetGroupHealth(ctx, f, tgArn, expected.NumTargets)
		}, utils.PollTimeoutLong, utils.PollIntervalLong).Should(BeTrue())
	})
	return nil
}

func (m *ServiceTest) VerifyAWSLoadBalancerResources(ctx context.Context, f *framework.Framework, expected LoadBalancerExpectation) error {
	By("Check LoadBanalcer on AWS", func() {
		lbArns, err := m.GetAwsLoadBalancerArns(ctx, f)
		Expect(err).ToNot(HaveOccurred())

		err = m.CheckLoadBalancerType(ctx, f, lbArns, expected.Type, expected.Scheme)
		Expect(err).ToNot(HaveOccurred())

		err = m.CheckLoadBalancerListeners(ctx, f, lbArns[0], expected)
		Expect(err).ToNot(HaveOccurred())

		err = m.CheckTargetGroups(ctx, f, lbArns[0], expected)
		Expect(err).ToNot(HaveOccurred())
	})
	return nil
}

func (m *ServiceTest) GetTargetGroupHealthCheckProtocol(ctx context.Context, f *framework.Framework) string {
	lbArns, err := m.GetAwsLoadBalancerArns(ctx, f)
	Expect(err).ToNot(HaveOccurred())
	targetGroups, err := f.Cloud.ELBV2().DescribeTargetGroupsWithContext(ctx, &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: aws.String(lbArns[0]),
	})
	Expect(err).NotTo(HaveOccurred())
	return aws.StringValue(targetGroups.TargetGroups[0].HealthCheckProtocol)
}

func (m *ServiceTest) GetListenerProtocol(ctx context.Context, f *framework.Framework, port string) string {
	protocol := ""
	By("Getting Loadbalancer listeners from AWS", func() {
		lbArns, err := m.GetAwsLoadBalancerArns(ctx, f)
		Expect(err).ToNot(HaveOccurred())

		listeners, err := f.Cloud.ELBV2().DescribeListenersWithContext(ctx, &elbv2.DescribeListenersInput{
			LoadBalancerArn: aws.String(lbArns[0]),
		})
		Expect(err).ToNot(HaveOccurred())
		for _, ls := range listeners.Listeners {
			if strconv.Itoa(int(aws.Int64Value(ls.Port))) == port {
				protocol = aws.StringValue(ls.Protocol)
			}
		}
	})
	return protocol
}

func (m *ServiceTest) SendTrafficToLB(ctx context.Context, f *framework.Framework) error {
	httpClient := http.Client{Timeout: utils.PollIntervalMedium}
	protocol := "http"
	if m.listenerTLS() {
		protocol = "https"
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}
	// Choose the first port for now, TODO: verify all listeners
	port := m.Service.Spec.Ports[0].Port
	noerr := false
	for i := 0; i < 10; i++ {
		resp, err := httpClient.Get(fmt.Sprintf("%s://%s:%v/from-tls-client", protocol, m.Service.Status.LoadBalancer.Ingress[0].Hostname, port))
		if err != nil {
			time.Sleep(2 * utils.PollIntervalLong)
			continue
		}
		noerr = true
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Unexpected HTTP status code %v", resp.StatusCode)
		}
		if resp.StatusCode == http.StatusOK {
			break
		}
	}
	if noerr {
		return nil
	}
	return fmt.Errorf("Unsuccessful after 10 retries")
}

func (m *ServiceTest) listenerTLS() bool {
	_, ok := m.Service.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-cert"]
	return ok
}

func (m *ServiceTest) targetGroupTLS() bool {
	return false
}

func (m *ServiceTest) GenerateAndImportCertToACM(ctx context.Context, f *framework.Framework, cn string) string {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).ToNot(HaveOccurred())

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour * 24 * 365)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	Expect(err).ToNot(HaveOccurred())

	cert := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Country:      []string{"US"},
			Locality:     []string{"Santa Clara"},
			Province:     []string{"CA"},
			Organization: []string{"E2E Tests"},
			CommonName:   cn,
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"*.elb.us-west-2.amazonaws.com"},
	}
	certDer, err := x509.CreateCertificate(rand.Reader, &cert, &cert, &priv.PublicKey, priv)
	Expect(err).ToNot(HaveOccurred())

	certPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDer})
	Expect(certPem).ToNot(BeNil())

	privDer, err := x509.MarshalPKCS8PrivateKey(priv)
	Expect(err).ToNot(HaveOccurred())

	keyPemStr := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDer})
	Expect(keyPemStr).ToNot(BeNil())

	// Upload to ACM, get the certificate ARN
	resp, err := f.Cloud.ACM().ImportCertificateWithContext(ctx, &acm.ImportCertificateInput{
		Certificate: certPem,
		PrivateKey:  keyPemStr,
	})
	Expect(err).ToNot(HaveOccurred())
	certArn := aws.StringValue(resp.CertificateArn)
	Expect(certArn).ToNot(BeNil())

	return certArn
}

func (m *ServiceTest) DeleteCertFromACM(ctx context.Context, f *framework.Framework, certArn string) error {
	_, err := f.Cloud.ACM().DeleteCertificateWithContext(ctx, &acm.DeleteCertificateInput{
		CertificateArn: aws.String(certArn),
	})
	return err
}
