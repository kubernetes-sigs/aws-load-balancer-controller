package model

import (
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
)

// buildLogDeliveries builds the Gateway's log deliveries when the feature gate is enabled.
func (baseBuilder *baseModelBuilder) buildLogDeliveries(stack core.Stack, lb *elbv2model.LoadBalancer, gwConfigs []elbv2gw.LogDeliveryConfiguration) error {
	if !baseBuilder.featureGates.Enabled(config.LogDelivery) {
		if len(gwConfigs) > 0 {
			baseBuilder.logger.Info("ignoring log delivery configuration because the LogDelivery feature gate is disabled", "stackID", stack.StackID().String())
		}
		return nil
	}
	_, err := logdeliverymodel.BuildLogDeliveries(stack, baseBuilder.loadBalancerType, lb.LoadBalancerARN(), buildLogDeliveryConfigs(gwConfigs), lb.Spec.Tags)
	return err
}

// buildLogDeliveryConfigs converts the LoadBalancerConfiguration log deliveries into the shared log delivery config.
func buildLogDeliveryConfigs(gwConfigs []elbv2gw.LogDeliveryConfiguration) []logdeliverymodel.Config {
	configs := make([]logdeliverymodel.Config, 0, len(gwConfigs))
	for _, gwConfig := range gwConfigs {
		config := logdeliverymodel.Config{
			LogType:                gwConfig.LogType,
			DestinationARN:         gwConfig.DestinationArn,
			DeliveryDestinationARN: gwConfig.DeliveryDestinationArn,
			OutputFormat:           gwConfig.OutputFormat,
			FieldDelimiter:         gwConfig.FieldDelimiter,
		}
		if gwConfig.S3DeliveryConfiguration != nil {
			config.S3DeliveryConfiguration = &logdeliverymodel.S3DeliveryConfiguration{
				SuffixPath:               gwConfig.S3DeliveryConfiguration.SuffixPath,
				EnableHiveCompatiblePath: gwConfig.S3DeliveryConfiguration.EnableHiveCompatiblePath,
			}
		}
		configs = append(configs, config)
	}
	return configs
}
