package aws

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
)

const (
	metricTargetGroupBinding = "target_group_info"

	labelNamespace = "namespace"
	// Name matches label when importing target group metrics using cloudwatch_exporter
	labelTargetGroup = "target_group"
)

type TargetGroupCollector interface {
	RegisterTargetGroupBinding(resTGB *elbv2api.TargetGroupBinding)
	DeRegisterTargetGroupBinding(resTGB *elbv2api.TargetGroupBinding)
}
type collector struct {
	infoMetric *prometheus.GaugeVec
}

type noOpCollector struct{}

func (n noOpCollector) RegisterTargetGroupBinding(_ *elbv2api.TargetGroupBinding) {}

func (n noOpCollector) DeRegisterTargetGroupBinding(_ *elbv2api.TargetGroupBinding) {}

func NewTargetGroupCollector(registerer prometheus.Registerer) TargetGroupCollector {
	if registerer == nil {
		return &noOpCollector{}
	}
	return &collector{registerTargetGroupInfoMetric(registerer)}
}

func registerTargetGroupInfoMetric(registerer prometheus.Registerer) *prometheus.GaugeVec {

	targetGroupInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricSubSystem,
		Name:      metricTargetGroupBinding,
		Help:      "Information available about target group",
	}, []string{labelNamespace, labelService, labelTargetGroup})

	registerer.MustRegister(targetGroupInfo)
	return targetGroupInfo
}

func (c *collector) RegisterTargetGroupBinding(resTGB *elbv2api.TargetGroupBinding) {
	c.infoMetric.With(getLabelsForTargetGroupBinding(resTGB)).Set(1)
}

func (c *collector) DeRegisterTargetGroupBinding(resTGB *elbv2api.TargetGroupBinding) {
	c.infoMetric.Delete(getLabelsForTargetGroupBinding(resTGB))
}

func getLabelsForTargetGroupBinding(resTGB *elbv2api.TargetGroupBinding) map[string]string {

	// Extracting value of TargetGroup dimension in CloudWatch
	// https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-cloudwatch-metrics.html#load-balancer-metric-dimensions-alb
	targetGroup := resTGB.Spec.TargetGroupARN[strings.LastIndex(resTGB.Spec.TargetGroupARN, ":")+1:]
	return map[string]string{
		labelNamespace:   resTGB.Namespace,
		labelService:     resTGB.Spec.ServiceRef.Name,
		labelTargetGroup: targetGroup,
	}
}
