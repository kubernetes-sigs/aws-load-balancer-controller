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
// IngressClassParams take precedence over all member annotations. Without a class configuration,
// every member that sets the annotation must set the same value.
func (t *defaultModelBuildTask) buildLogDeliveries(_ context.Context, lb *elbv2model.LoadBalancer) error {
	if !t.featureGates.Enabled(config.LogDelivery) {
		for _, member := range t.ingGroup.Members {
			if member.IngClassConfig.IngClassParams != nil && member.IngClassConfig.IngClassParams.Spec.LogDelivery != nil {
				t.logger.Info("ignoring log delivery configuration because the LogDelivery feature gate is disabled", "ingressClassParams", member.IngClassConfig.IngClassParams.Name)
			}
			if _, exists := member.Ing.Annotations[annotations.AnnotationPrefixIngress+"/"+annotations.IngressSuffixLogDelivery]; exists {
				t.logger.Info("ignoring log delivery configuration because the LogDelivery feature gate is disabled", "ingress", k8s.NamespacedName(member.Ing).String())
			}
		}
		return nil
	}
	classConfigs, hasClassConfigs, err := t.buildIngressClassLogDeliveryConfigs()
	if err != nil {
		return err
	}
	if hasClassConfigs {
		_, err := logdeliverymodel.BuildLogDeliveries(t.stack, elbv2model.LoadBalancerTypeApplication, lb.LoadBalancerARN(), classConfigs, lb.Spec.Tags)
		return err
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
	_, err = logdeliverymodel.BuildLogDeliveries(t.stack, elbv2model.LoadBalancerTypeApplication, lb.LoadBalancerARN(), explicitConfigs, lb.Spec.Tags)
	return err
}

// buildIngressClassLogDeliveryConfigs resolves explicit class settings, preserving empty-list overrides and rejecting conflicts between classes.
func (t *defaultModelBuildTask) buildIngressClassLogDeliveryConfigs() ([]logdeliverymodel.Config, bool, error) {
	var explicitConfigs []logdeliverymodel.Config
	explicitClass := ""
	for _, member := range t.ingGroup.Members {
		params := member.IngClassConfig.IngClassParams
		if params == nil || params.Spec.LogDelivery == nil {
			continue
		}
		configs := make([]logdeliverymodel.Config, 0, len(*params.Spec.LogDelivery))
		for _, delivery := range *params.Spec.LogDelivery {
			cfg := logdeliverymodel.Config{
				LogType:                delivery.LogType,
				DestinationARN:         delivery.DestinationARN,
				DeliveryDestinationARN: delivery.DeliveryDestinationARN,
				OutputFormat:           delivery.OutputFormat,
				FieldDelimiter:         delivery.FieldDelimiter,
			}
			if delivery.S3DeliveryConfiguration != nil {
				cfg.S3DeliveryConfiguration = &logdeliverymodel.S3DeliveryConfiguration{
					SuffixPath:               delivery.S3DeliveryConfiguration.SuffixPath,
					EnableHiveCompatiblePath: delivery.S3DeliveryConfiguration.EnableHiveCompatiblePath,
				}
			}
			configs = append(configs, cfg)
		}
		if explicitClass != "" && !reflect.DeepEqual(explicitConfigs, configs) {
			return nil, false, errors.Errorf("conflicting log delivery configuration in IngressClassParams %v and %v", explicitClass, params.Name)
		}
		explicitConfigs = configs
		explicitClass = params.Name
	}
	return explicitConfigs, explicitClass != "", nil
}
