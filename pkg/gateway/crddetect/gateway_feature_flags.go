package crddetect

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

const (
	// GatewayV1GroupVersion is the stable Gateway API group version.
	GatewayV1GroupVersion = "gateway.networking.k8s.io/v1"
	// GatewayV1Alpha2GroupVersion is the experimental Gateway API group version.
	GatewayV1Alpha2GroupVersion = "gateway.networking.k8s.io/v1alpha2"
	// LBCGatewayGroupVersion is the LBC-specific Gateway CRD group version.
	LBCGatewayGroupVersion = "gateway.k8s.aws/v1beta1"
)

var (
	// lbcGatewayKinds are the LBC-specific CRDs required by both ALB and NLB gateway controllers.
	lbcGatewayKinds = []string{"TargetGroupConfiguration", "LoadBalancerConfiguration", "ListenerRuleConfiguration"}

	albKinds         = map[string][]string{GatewayV1GroupVersion: {"Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute"}, LBCGatewayGroupVersion: lbcGatewayKinds}
	nlbKinds         = map[string][]string{GatewayV1GroupVersion: {"Gateway", "GatewayClass", "TLSRoute"}, GatewayV1Alpha2GroupVersion: {"TCPRoute", "UDPRoute"}, LBCGatewayGroupVersion: lbcGatewayKinds}
	listenerSetKinds = map[string][]string{GatewayV1GroupVersion: {"ListenerSet"}}
)

// ApplyGatewayCRDDetection checks for the presence of Gateway API CRDs and
// disables the corresponding feature flags when required CRDs are missing.
// It is called from main() after the k8s client is ready and before any
// controller reads the feature flags.
func ApplyGatewayCRDDetection(client k8s.DiscoveryClient, featureGates config.FeatureGates, logger logr.Logger) error {

	allDefaulted := featureGates.GetFeatureStatus(config.ALBGatewayAPI).IsDefaulted ||
		featureGates.GetFeatureStatus(config.NLBGatewayAPI).IsDefaulted ||
		featureGates.GetFeatureStatus(config.GatewayListenerSet).IsDefaulted

	if !allDefaulted {
		// User set all flags directly, do nothing.
		return nil
	}

	availableResources, err := k8s.DetectCRDs(client, sets.New(GatewayV1Alpha2GroupVersion, GatewayV1GroupVersion, LBCGatewayGroupVersion))
	if err != nil {
		return err
	}

	applyGatewayFeatureFlags(availableResources, featureGates, logger)
	return nil
}

func applyGatewayFeatureFlags(availableResources map[string]sets.Set[string], featureGates config.FeatureGates, logger logr.Logger) {

	albMissingKinds := missingKinds(albKinds, availableResources)
	if len(albMissingKinds) > 0 {
		logger.Info("Disabling ALBGatewayAPI: missing required CRDs",
			"missing", albMissingKinds)
		featureGates.Disable(config.ALBGatewayAPI)
	}

	nlbMissingKinds := missingKinds(nlbKinds, availableResources)
	if len(nlbMissingKinds) > 0 && featureGates.GetFeatureStatus(config.NLBGatewayAPI).IsDefaulted {
		logger.Info("Disabling NLBGatewayAPI: missing required CRDs",
			"missing", nlbMissingKinds)
		featureGates.Disable(config.NLBGatewayAPI)
	}

	listenerSetMissing := missingKinds(listenerSetKinds, availableResources)
	if len(listenerSetMissing) > 0 && featureGates.GetFeatureStatus(config.GatewayListenerSet).IsDefaulted {
		logger.Info("Disabling GatewayListenerSet: missing required CRDs", "missing", listenerSetMissing)
		featureGates.Disable(config.GatewayListenerSet)
	}
}

func missingKinds(desiredKinds map[string][]string, availableResources map[string]sets.Set[string]) []string {
	missing := make([]string, 0)

	for apiVersion, kinds := range desiredKinds {
		var ok bool
		var availableKinds sets.Set[string]
		if availableKinds, ok = availableResources[apiVersion]; !ok {
			missing = append(missing, kinds...)
			continue
		}
		for _, kind := range kinds {
			if !availableKinds.Has(kind) {
				missing = append(missing, kind)
			}
		}
	}

	return missing
}
