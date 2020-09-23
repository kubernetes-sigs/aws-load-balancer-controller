package ingress

import (
	"encoding/json"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
)

// getStrategicPatchBytes returns the strategic patch to change Ingress from oldIng to newIng
func getStrategicPatchBytes(oldIng *networking.Ingress, newIng *networking.Ingress) ([]byte, error) {
	oldData, err := json.Marshal(oldIng)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to Marshal oldData for Ingress: %v", k8s.NamespacedName(oldIng).String())
	}

	newData, err := json.Marshal(newIng)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to Marshal newData for Ingress: %v", k8s.NamespacedName(newIng).String())
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, networking.Ingress{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to CreateTwoWayMergePatch for Ingress: %v", k8s.NamespacedName(newIng).String())
	}
	return patchBytes, nil
}
