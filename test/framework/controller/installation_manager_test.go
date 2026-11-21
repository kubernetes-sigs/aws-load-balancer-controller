package controller

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	for _, tt := range []struct {
		name        string
		targetAgent bool
		certMgmt    bool
		logDelivery bool
	}{
		{name: "no optional gates"},
		{name: "target control without log delivery", targetAgent: true},
		{name: "certificate management without log delivery", certMgmt: true},
		{name: "other gates without log delivery", targetAgent: true, certMgmt: true},
		{name: "log delivery alone", logDelivery: true},
		{name: "all optional gates", targetAgent: true, certMgmt: true, logDelivery: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			releaseManager := &recordingReleaseManager{}
			manager := NewDefaultInstallationManager(releaseManager, "cluster", "us-west-2", "vpc", "chart", logr.Discard())
			image := "controller:v3.6.0"
			if tt.logDelivery {
				image = "controller:pr"
			}
			require.NoError(t, manager.UpgradeController(image, true, tt.targetAgent, tt.certMgmt, tt.logDelivery))
			if !tt.targetAgent && !tt.certMgmt && !tt.logDelivery {
				assert.NotContains(t, releaseManager.values, "controllerConfig")
				return
			}
			gates := releaseManager.values["controllerConfig"].(map[string]interface{})["featureGates"].(map[string]interface{})
			if tt.logDelivery {
				assert.Equal(t, true, gates["LogDelivery"])
			} else {
				assert.NotContains(t, gates, "LogDelivery", "older images reject an unknown gate even when its value is false")
			}
			assert.Equal(t, tt.targetAgent, gates["ALBTargetControlAgent"])
			assert.Equal(t, tt.certMgmt, gates["EnableCertificateManagement"])
		})
	}
}
