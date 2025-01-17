package lbc

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	rgtsdk "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	networkLoadBalancerStr = "nlb"
	resourceTypeALB        = "elasticloadbalancing:loadbalancer/app"
	resourceTypeNLB        = "elasticloadbalancing:loadbalancer/net"
)

type MetricCollector interface {
	// ObservePodReadinessGateReady this metric is useful to determine how fast pods are becoming ready in the load balancer.
	// Due to some architectural constraints, we can only emit this metric for pods that are using readiness gates.
	ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration)

	// UpdateManagedK8sResourceMetrics fetches and updates managed k8s resources metrics.
	UpdateManagedK8sResourceMetrics(ctx context.Context) error

	// UpdateManagedALBMetrics updates managed ALB count metrics
	UpdateManagedALBMetrics(ctx context.Context) error

	//UpdateManagedNLBMetrics updates managed NLB count metrics
	UpdateManagedNLBMetrics(ctx context.Context) error
}

type collector struct {
	instruments      *instruments
	runtimeClient    client.Client
	rgt              services.RGT
	finalizerKeyWord string
	clusterTagKey    string
	clusterTagVal    string
}

type noOpCollector struct{}

func (n *noOpCollector) ObservePodReadinessGateReady(_ string, _ string, _ time.Duration) {
}

func (n *noOpCollector) UpdateManagedK8sResourceMetrics(_ context.Context) error {
	return nil
}

func (n *noOpCollector) UpdateManagedALBMetrics(_ context.Context) error {
	return nil
}

func (n *noOpCollector) UpdateManagedNLBMetrics(_ context.Context) error {
	return nil
}

func NewCollector(registerer prometheus.Registerer, runtimeClient client.Client, rgt services.RGT, finalizerKeyWord string, clusterTagKey string, clusterTagVal string) MetricCollector {
	if registerer == nil || runtimeClient == nil {
		return &noOpCollector{}
	}

	instruments := newInstruments(registerer)
	return &collector{
		instruments:      instruments,
		runtimeClient:    runtimeClient,
		rgt:              rgt,
		finalizerKeyWord: finalizerKeyWord,
		clusterTagKey:    clusterTagKey,
		clusterTagVal:    clusterTagVal,
	}
}

func (c *collector) ObservePodReadinessGateReady(namespace string, tgbName string, duration time.Duration) {
	c.instruments.podReadinessFlipSeconds.With(prometheus.Labels{
		labelNamespace: namespace,
		labelName:      tgbName,
	}).Observe(duration.Seconds())
}

func (c *collector) UpdateManagedK8sResourceMetrics(ctx context.Context) error {
	listOpts := &client.ListOptions{
		Namespace: "",
	}
	ingressCount, serviceCount, tgbCount := 0, 0, 0
	// Fetch ingress count
	ingressList := &networkingv1.IngressList{}
	err := c.runtimeClient.List(ctx, ingressList, listOpts)
	if err != nil {
		return err
	}
	for _, ingress := range ingressList.Items {
		for _, finalizer := range ingress.Finalizers {
			if strings.Contains(finalizer, c.finalizerKeyWord) {
				ingressCount++
				break
			}
		}
	}
	c.instruments.managedIngressCount.Set(float64(ingressCount))

	// Fetch service count
	serviceList := &corev1.ServiceList{}
	err = c.runtimeClient.List(ctx, serviceList, listOpts)
	if err != nil {
		return err
	}
	for _, service := range serviceList.Items {
		hasMatchingFinalizer := false
		for _, finalizer := range service.Finalizers {
			if strings.Contains(finalizer, c.finalizerKeyWord) {
				hasMatchingFinalizer = true
				break
			}
		}

		if hasMatchingFinalizer && service.Spec.LoadBalancerClass != nil && strings.Contains(*service.Spec.LoadBalancerClass, networkLoadBalancerStr) {
			serviceCount++
		}
	}
	c.instruments.managedServiceCount.Set(float64(serviceCount))

	// Fetch TargetGroupBinding count
	tgbList := &elbv2api.TargetGroupBindingList{}
	err = c.runtimeClient.List(ctx, tgbList, listOpts)
	if err != nil {
		return err
	}
	for _, tgb := range tgbList.Items {
		for _, finalizer := range tgb.Finalizers {
			if strings.Contains(finalizer, c.finalizerKeyWord) {
				tgbCount++
				break
			}
		}
	}
	c.instruments.managedTGBCount.Set(float64(tgbCount))

	return nil
}

func (c *collector) UpdateManagedALBMetrics(ctx context.Context) error {
	count, err := c.getManagedAWSResourceMetrics(ctx, resourceTypeALB)
	if err != nil {
		return err
	}
	c.instruments.managedALBCount.Set(float64(count))
	return nil
}

func (c *collector) UpdateManagedNLBMetrics(ctx context.Context) error {
	count, err := c.getManagedAWSResourceMetrics(ctx, resourceTypeNLB)
	if err != nil {
		return err
	}
	c.instruments.managedNLBCount.Set(float64(count))
	return nil
}

func (c *collector) getManagedAWSResourceMetrics(ctx context.Context, resourceType string) (count int, err error) {
	req := &rgtsdk.GetResourcesInput{
		ResourceTypeFilters: []string{resourceType},
		TagFilters: []rgttypes.TagFilter{
			{
				Key:    awssdk.String(c.clusterTagKey),
				Values: []string{c.clusterTagVal},
			},
		},
	}
	resources, err := c.rgt.GetResourcesAsList(ctx, req)
	if err != nil {
		return 0, err
	}
	return len(resources), nil
}
