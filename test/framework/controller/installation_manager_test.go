package controller

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	helmrelease "helm.sh/helm/v3/pkg/release"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/helm"
)

type recordingReleaseManager struct {
	helm.ReleaseManager
	values map[string]interface{}
}

func (m *recordingReleaseManager) InstallOrUpgradeRelease(_, _, _ string, values map[string]interface{}, _ ...helm.ActionOption) (*helmrelease.Release, error) {
	m.values = values
	return &helmrelease.Release{}, nil
}

func TestUpgradeController_logDeliveryFeatureGate(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default leaves the gate off", true: "opt-in enables the gate"}[enabled], func(t *testing.T) {
			releaseManager := &recordingReleaseManager{}
			manager := NewDefaultInstallationManager(releaseManager, "cluster", "us-west-2", "vpc", "chart", logr.Discard())
			assert.NoError(t, manager.UpgradeController("controller:pr", true, true, true, enabled))
			gates := releaseManager.values["controllerConfig"].(map[string]interface{})["featureGates"].(map[string]interface{})
			assert.Equal(t, enabled, gates["LogDelivery"])
			assert.Equal(t, true, gates["ALBTargetControlAgent"])
			assert.Equal(t, true, gates["EnableCertificateManagement"])
		})
	}
}
