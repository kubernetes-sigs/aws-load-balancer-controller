package gateway

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/addon"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/gateway/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strconv"
	"strings"
)

var (
	albAddons = []addon.Addon{addon.WAFv2, addon.Shield}
	nlbAddons []addon.Addon
)

const (
	trueString  = "true"
	falseString = "false"
)

// getStoredAddonConfig parses the addon configuration stored in a Gateways' annotation into their representation in native go structs.
func getStoredAddonConfig(gateway *gwv1.Gateway) []addon.AddonMetadata {
	res := make([]addon.AddonMetadata, 0)

	if gateway.Annotations == nil {
		return res
	}

	for annotationKey, annotationValue := range gateway.Annotations {
		if strings.HasPrefix(annotationKey, constants.GatewayLBPrefixEnabledAddon) {
			for _, ao := range addon.AllAddons {
				if annotationKey == generateAddOnKey(ao) {
					res = append(res, addon.AddonMetadata{
						Name:    ao,
						Enabled: parseAddOnEnabledValue(annotationValue),
					})
				}
			}

		}
	}

	return res
}

// generateAddOnKey translates an addon into the respective annotation key value.
func generateAddOnKey(a addon.Addon) string {
	return fmt.Sprintf("%s%s", constants.GatewayLBPrefixEnabledAddon, strings.ToLower(string(a)))
}

// parseAddOnEnabledValue parses an annotation key value into a boolean, assuming false if the value is malformed.
func parseAddOnEnabledValue(e string) bool {
	b, err := strconv.ParseBool(e)
	if err != nil {
		// Assume corrupted value is false (todo - maybe error log?)
		return false
	}
	return b
}

// diffAddOns determines the additions and subtractions when comparing old (the previous reconcile run result) and new (the current reconcile run result)
func diffAddOns(old []addon.Addon, new []addon.AddonMetadata) (sets.Set[addon.Addon], sets.Set[addon.Addon]) {
	additions := sets.New[addon.Addon]()
	removals := sets.New[addon.Addon]()

	oldSet := sets.New(old...)
	newSet := sets.New[addon.Addon]()

	for _, newItem := range new {
		if newItem.Enabled {
			newSet.Insert(newItem.Name)
		}

		if !oldSet.Has(newItem.Name) && newItem.Enabled {
			additions.Insert(newItem.Name)
		}

	}

	for _, aOld := range old {
		if !newSet.Has(aOld) {
			removals.Insert(aOld)
		}
	}

	return additions, removals
}

// persistNewAddOns persists the enabled addons to the Gateway annotations. This assumes that addOns is the complete addon set.
func persistAddOns(ctx context.Context, k8sClient client.Client, gw *gwv1.Gateway, changes []addon.Addon, remove bool) error {
	annotations := make(map[string]string)
	if gw.Annotations != nil {
		for k, v := range gw.Annotations {
			annotations[k] = v
		}
	}

	var annotationValue = trueString
	if remove {
		annotationValue = falseString
	}

	for _, ao := range changes {
		annotations[generateAddOnKey(ao)] = annotationValue
	}

	gwOld := gw.DeepCopy()
	gw.Annotations = annotations
	return k8sClient.Patch(ctx, gw, client.MergeFrom(gwOld))
}
