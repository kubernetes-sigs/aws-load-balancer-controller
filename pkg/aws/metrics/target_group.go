package metrics

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

func RegisterTargetGroupInfoMetric(registerer prometheus.Registerer) (*prometheus.GaugeVec, error) {

	targetGroupInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricSubsystemAWS,
		Name:      metricTargetGroupBinding,
		Help:      "Information available about target group",
	}, []string{labelNamespace, labelService, labelTargetGroup})

	err := registerer.Register(targetGroupInfo)
	return targetGroupInfo, err
}

func LabelsForTargetGroupBinding(resTGB *elbv2api.TargetGroupBinding) map[string]string {

	// Extracting value of TargetGroup dimension in CloudWatch
	// https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-cloudwatch-metrics.html#load-balancer-metric-dimensions-alb
	targetGroup := resTGB.Spec.TargetGroupARN[strings.LastIndex(resTGB.Spec.TargetGroupARN, ":")+1:]
	return map[string]string{
		labelNamespace:   resTGB.Namespace,
		labelService:     resTGB.Spec.ServiceRef.Name,
		labelTargetGroup: targetGroup,
	}
}
