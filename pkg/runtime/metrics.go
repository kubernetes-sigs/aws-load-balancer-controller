package runtime

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	LbGaugeLabelK8sResourceName      = "k8s_resource_name"
	LbGaugeLabelK8sResourceNamespace = "k8s_resource_namespace"
	LbGaugeLabelAWSLoadBalancerName  = "aws_load_balancer_name"
	LbGaugeLabelAWSLoadBalancerType  = "aws_load_balancer_type"
)

type ManagedResourcesCollector struct {
	LoadBalancersGauge *prometheus.GaugeVec
}

func NewMetricsCollector(metricsRegisterer prometheus.Registerer) (*ManagedResourcesCollector, error) {
	loadBalancerGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "managed_aws_load_balancers",
			Help: "Metric to track what Kubernetes resources that created AWS resources",
		},
		[]string{
			LbGaugeLabelK8sResourceName,
			LbGaugeLabelK8sResourceNamespace,
			LbGaugeLabelAWSLoadBalancerName,
			LbGaugeLabelAWSLoadBalancerType,
		},
	)
	if err := metricsRegisterer.Register(loadBalancerGauge); err != nil {
		return nil, err
	}

	return &ManagedResourcesCollector{
		LoadBalancersGauge: loadBalancerGauge,
	}, nil
}
