package ingress

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
)

// buildLogDeliveries builds the CloudWatch Logs deliveries of the IngressGroup's load balancer.
// Every member that sets the annotation must set the same value.
func (t *defaultModelBuildTask) buildLogDeliveries(_ context.Context, lb *elbv2model.LoadBalancer) error {
	if !t.featureGates.Enabled(config.LogDelivery) {
		for _, member := range t.ingGroup.Members {
			if _, exists := member.Ing.Annotations[annotations.AnnotationPrefixIngress+"/"+annotations.IngressSuffixLogDelivery]; exists {
				t.logger.Info("ignoring log delivery configuration because the LogDelivery feature gate is disabled", "ingress", k8s.NamespacedName(member.Ing).String())
			}
		}
		return nil
	}
	var explicitConfigs []logdeliverymodel.Config
	explicitMember := ""
	for _, member := range t.ingGroup.Members {
		var configs []logdeliverymodel.Config
		exists, err := t.annotationParser.ParseJSONAnnotation(annotations.IngressSuffixLogDelivery, &configs, member.Ing.Annotations)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		memberKey := k8s.NamespacedName(member.Ing).String()
		if explicitMember != "" && !reflect.DeepEqual(explicitConfigs, configs) {
			return errors.Errorf("conflicting log delivery configuration in ingresses %v and %v", explicitMember, memberKey)
		}
		explicitConfigs = configs
		explicitMember = memberKey
	}
	_, err := logdeliverymodel.BuildLogDeliveries(t.stack, elbv2model.LoadBalancerTypeApplication, lb.LoadBalancerARN(), explicitConfigs, lb.Spec.Tags)
	return err
}
