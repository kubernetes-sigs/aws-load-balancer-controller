package service

import (
	"context"

	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
)

// buildLogDeliveries builds the CloudWatch Logs deliveries of the Service's Network Load Balancer.
func (t *defaultModelBuildTask) buildLogDeliveries(_ context.Context) error {
	if !t.featureGates.Enabled(config.LogDelivery) {
		if _, exists := t.service.Annotations["service.beta.kubernetes.io/"+annotations.SvcLBSuffixLogDelivery]; exists {
			t.logger.Info("ignoring log delivery configuration because the LogDelivery feature gate is disabled", "service", k8s.NamespacedName(t.service).String())
		}
		return nil
	}
	var configs []logdeliverymodel.Config
	if _, err := t.annotationParser.ParseJSONAnnotation(annotations.SvcLBSuffixLogDelivery, &configs, t.service.Annotations); err != nil {
		return err
	}
	_, err := logdeliverymodel.BuildLogDeliveries(t.stack, elbv2model.LoadBalancerTypeNetwork, t.loadBalancer.LoadBalancerARN(), configs, t.loadBalancer.Spec.Tags)
	return err
}
