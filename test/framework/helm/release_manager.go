package helm

import (
	"fmt"
	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// ReleaseManager is responsible for manage helm releases
type ReleaseManager interface {
	// install or upgrade helm release
	InstallOrUpgradeRelease(chartRepo string, chartName string,
		namespace string, releaseName string, vals map[string]interface{}) (*release.Release, error)

	// install helm release
	InstallRelease(chartRepo string, chartName string,
		namespace string, releaseName string, vals map[string]interface{}) (*release.Release, error)

	// upgrade helm release
	UpgradeRelease(chartRepo string, chartName string,
		namespace string, releaseName string, vals map[string]interface{}) (*release.Release, error)
}

// NewDefaultReleaseManager constructs new defaultReleaseManager.
func NewDefaultReleaseManager(kubeConfig string, logger logr.Logger) *defaultReleaseManager {
	return &defaultReleaseManager{
		kubeConfig: kubeConfig,
		logger:     logger,
	}
}

var _ ReleaseManager = &defaultReleaseManager{}

// default implementation for ReleaseManager
type defaultReleaseManager struct {
	kubeConfig string
	logger     logr.Logger
}

func (m *defaultReleaseManager) InstallOrUpgradeRelease(chartRepo string, chartName string,
	namespace string, releaseName string, vals map[string]interface{}) (*release.Release, error) {
	actionCFG := m.obtainActionConfig(namespace)
	historyAction := action.NewHistory(actionCFG)
	historyAction.Max = 1
	if _, err := historyAction.Run(releaseName); err == driver.ErrReleaseNotFound {
		return m.InstallRelease(chartRepo, chartName, namespace, releaseName, vals)
	} else {
		return m.UpgradeRelease(chartRepo, chartName, namespace, releaseName, vals)
	}
}

func (m *defaultReleaseManager) InstallRelease(chartRepo string, chartName string,
	namespace string, releaseName string, vals map[string]interface{}) (*release.Release, error) {

	actionCFG := m.obtainActionConfig(namespace)
	installAction := action.NewInstall(actionCFG)
	installAction.ChartPathOptions.RepoURL = chartRepo
	installAction.Namespace = namespace
	installAction.SkipCRDs = false
	installAction.Wait = true
	installAction.ReleaseName = releaseName

	cp, err := installAction.ChartPathOptions.LocateChart(chartName, cli.New())
	chartRequested, err := loader.Load(cp)
	if err != nil {
		return nil, err
	}

	return installAction.Run(chartRequested, vals)
}

func (m *defaultReleaseManager) UpgradeRelease(chartRepo string, chartName string,
	namespace string, releaseName string, vals map[string]interface{}) (*release.Release, error) {

	actionCFG := m.obtainActionConfig(namespace)
	upgradeAction := action.NewUpgrade(actionCFG)
	upgradeAction.ChartPathOptions.RepoURL = chartRepo
	upgradeAction.Namespace = namespace
	upgradeAction.ResetValues = true
	upgradeAction.Wait = true

	cp, err := upgradeAction.ChartPathOptions.LocateChart(chartName, cli.New())
	chartRequested, err := loader.Load(cp)
	if err != nil {
		return nil, err
	}
	return upgradeAction.Run(releaseName, chartRequested, vals)
}

func (m *defaultReleaseManager) obtainActionConfig(namespace string) *action.Configuration {
	cfgFlags := genericclioptions.NewConfigFlags(false)
	cfgFlags.KubeConfig = &m.kubeConfig
	cfgFlags.Namespace = &namespace
	actionConfig := new(action.Configuration)
	actionConfig.Init(cfgFlags, namespace, "secrets", func(format string, v ...interface{}) {
		message := fmt.Sprintf(format, v...)
		m.logger.Info(message)
	})
	return actionConfig
}
