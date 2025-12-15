package globalaccelerator

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/service"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

var _ = Describe("GlobalAccelerator with multiple endpoint types", func() {
	var (
		ctx       context.Context
		agaStack  *ResourceStack
		svcStack  *service.ResourceStack
		ingStack  *ingress.ResourceStack
		namespace string
		svcName   string
		ingName   string
		aga       *agav1beta1.GlobalAccelerator
		baseName  string
	)

	BeforeEach(func() {
		if !tf.Options.EnableAGATests {
			Skip("Skipping Global Accelerator tests (requires --enable-aga-tests)")
		}
		ctx = context.Background()
		ns, err := tf.NSManager.AllocateNamespace(ctx, "aga-multi-e2e")
		Expect(err).NotTo(HaveOccurred())
		namespace = ns.Name
		tf.Logger.Info("allocated namespace for multi-endpoint test", "namespace", namespace)
		baseName = "aga-multi-" + utils.RandomDNS1123Label(8)
		svcName = baseName + "-service"
		ingName = baseName + "-ingress"
		labels := map[string]string{"app": baseName}

		// Deploy Ingress endpoint resources first
		ingDeployment := createDeployment(baseName+"-ing", namespace, labels)
		nodeSvc := createNodePortService(baseName+"-nodesvc", namespace, labels)
		ing := createALBIngress(ingName, namespace, baseName+"-nodesvc")
		ingStack = ingress.NewResourceStack([]*appsv1.Deployment{ingDeployment}, []*corev1.Service{nodeSvc}, []*networkingv1.Ingress{ing})
		err = ingStack.Deploy(ctx, tf)
		Expect(err).NotTo(HaveOccurred())

		// Deploy Service endpoint resources in the same namespace
		svcDeployment := createDeployment(baseName+"-svc", namespace, labels)
		nlbAnnotations := createServiceAnnotations("nlb-ip", "internet-facing", tf.Options.IPFamily)
		nlbSvc := createLoadBalancerService(svcName, labels, nlbAnnotations)
		nlbSvc.Namespace = namespace
		svcStack = service.NewResourceStack(svcDeployment, nlbSvc, nil, namespace, true)
		err = svcStack.Deploy(ctx, tf)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if agaStack != nil {
			err := agaStack.Cleanup(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
		}
		if ingStack != nil {
			err := ingStack.Cleanup(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
		}
		if svcStack != nil {
			err := svcStack.Cleanup(ctx, tf)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Multiple endpoint types with port overrides", func() {
		It("Should create GlobalAccelerator with Service and Ingress endpoints", func() {
			acceleratorName := "aga-multi-" + utils.RandomDNS1123Label(6)
			protocol := agav1beta1.GlobalAcceleratorProtocolTCP
			gaName := "aga-" + utils.RandomDNS1123Label(8)

			tf.Logger.Info("creating GlobalAccelerator with multiple endpoints in same namespace",
				"namespace", namespace,
				"svcName", svcName,
				"ingName", ingName)

			aga = createAGA(gaName, namespace, acceleratorName, agav1beta1.IPAddressTypeIPV4, &[]agav1beta1.GlobalAcceleratorListener{{
				Protocol:       &protocol,
				PortRanges:     &[]agav1beta1.PortRange{{FromPort: 80, ToPort: 80}},
				ClientAffinity: agav1beta1.ClientAffinityNone,
				EndpointGroups: &[]agav1beta1.GlobalAcceleratorEndpointGroup{{
					TrafficDialPercentage: awssdk.Int32(100),
					Endpoints: &[]agav1beta1.GlobalAcceleratorEndpoint{
						{Type: agav1beta1.GlobalAcceleratorEndpointTypeService, Name: awssdk.String(svcName)},
						{Type: agav1beta1.GlobalAcceleratorEndpointTypeIngress, Name: awssdk.String(ingName)},
					},
				}},
			}})

			By("deploying GlobalAccelerator with multiple endpoint types", func() {
				agaStack = NewResourceStack(aga)
				err := agaStack.Deploy(ctx, tf)
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying AWS configuration", func() {
				gaARN := agaStack.GetGlobalAcceleratorARN()
				err := verifyGlobalAcceleratorConfiguration(ctx, tf, gaARN, GlobalAcceleratorExpectation{
					Name:          acceleratorName,
					IPAddressType: string(types.IpAddressTypeIpv4),
					Status:        string(types.AcceleratorStatusDeployed),
					Listeners: []ListenerExpectation{{
						Protocol:       "TCP",
						PortRanges:     []PortRangeExpectation{{FromPort: 80, ToPort: 80}},
						ClientAffinity: string(types.ClientAffinityNone),
						EndpointGroups: []EndpointGroupExpectation{{
							TrafficDialPercentage: 100,
							NumEndpoints:          1,
						}},
					}},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			By("verifying GlobalAccelerator status fields", func() {
				verifyAGAStatusFields(agaStack)
			})

			By("verifying traffic flows through GlobalAccelerator", func() {
				err := verifyAGATrafficFlows(ctx, tf, agaStack)
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})
})
