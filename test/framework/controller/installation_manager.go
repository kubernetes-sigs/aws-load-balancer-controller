package controller

import (
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/helm"
)

// InstallationManager is responsible for manage controller installation in cluster.
type InstallationManager interface {
	ResetController() error
	UpgradeController(controllerImage string) error
}

// NewDefaultInstallationManager constructs new defaultInstallationManager.
func NewDefaultInstallationManager(helmReleaseManager helm.ReleaseManager, clusterName string, region string, vpcID string, helmChart string, logger logr.Logger) *defaultInstallationManager {
	return &defaultInstallationManager{
		helmReleaseManager: helmReleaseManager,
		clusterName:        clusterName,
		region:             region,
		vpcID:              vpcID,
		helmChart:          helmChart,

		namespace:        "kube-system",
		controllerSAName: "aws-load-balancer-controller",
		logger:           logger,
	}
}

var _ InstallationManager = &defaultInstallationManager{}

// InstallationManager based on helm
type defaultInstallationManager struct {
	helmReleaseManager helm.ReleaseManager
	clusterName        string
	region             string
	vpcID              string
	helmChart          string

	namespace        string
	controllerSAName string
	logger           logr.Logger
}

func (m *defaultInstallationManager) ResetController() error {
	vals := m.computeDefaultHelmVals()
	_, err := m.helmReleaseManager.InstallOrUpgradeRelease(m.helmChart,
		m.namespace, AWSLoadBalancerControllerHelmRelease, vals,
		helm.WithTimeout(AWSLoadBalancerControllerInstallationTimeout))
	return err
}

func (m *defaultInstallationManager) UpgradeController(controllerImage string) error {
	imageRepo, imageTag, err := splitImageRepoAndTag(controllerImage)
	if err != nil {
		return err
	}
	vals := m.computeDefaultHelmVals()
	vals["image"] = map[string]interface{}{
		"repository": imageRepo,
		"tag":        imageTag,
	}
	vals["podLabels"] = map[string]string{
		"revision": string(uuid.NewUUID()),
	}
	_, err = m.helmReleaseManager.InstallOrUpgradeRelease(m.helmChart,
		m.namespace, AWSLoadBalancerControllerHelmRelease, vals,
		helm.WithTimeout(AWSLoadBalancerControllerInstallationTimeout))
	return err
}

func (m *defaultInstallationManager) computeDefaultHelmVals() map[string]interface{} {
	vals := make(map[string]interface{})
	vals["clusterName"] = m.clusterName
	vals["region"] = m.region
	vals["vpcId"] = m.vpcID
	vals["serviceAccount"] = map[string]interface{}{
		"create": false,
		"name":   m.controllerSAName,
	}
	return vals
}

// splitImageRepoAndTag parses a docker image in format <imageRepo>:<imageTag> into `imageRepo` and `imageTag`
func splitImageRepoAndTag(dockerImage string) (string, string, error) {
	parts := strings.Split(dockerImage, ":")
	if len(parts) != 2 {
		return "", "", errors.Errorf("dockerImage expects <imageRepo>:<imageTag>, got: %s", dockerImage)
	}
	return parts[0], parts[1], nil
}
