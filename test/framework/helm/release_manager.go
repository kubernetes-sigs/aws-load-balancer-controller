package helm

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// ActionOptions contains general helm action options
type ActionOptions struct {
	// The duration to wait for helm operations. when zero, wait is disabled.
	Timeout time.Duration
}

// ApplyOptions applies all ActionOption
func (opts *ActionOptions) ApplyOptions(options []ActionOption) {
	for _, option := range options {
		option(opts)
	}
}

// ActionOption configures ActionOptions.
type ActionOption func(opts *ActionOptions)

// WithTimeout configures timeout for helm action
func WithTimeout(timeout time.Duration) ActionOption {
	return func(opts *ActionOptions) {
		opts.Timeout = timeout
	}
}

// ReleaseManager is responsible for manage helm releases
type ReleaseManager interface {
	// InstallOrUpgradeRelease install or upgrade helm release
	InstallOrUpgradeRelease(chartName string,
		namespace string, releaseName string, vals map[string]interface{}, opts ...ActionOption) (*release.Release, error)

	// InstallRelease install helm release
	InstallRelease(chartName string,
		namespace string, releaseName string, vals map[string]interface{}, opts ...ActionOption) (*release.Release, error)

	// UpgradeRelease upgrade helm release
	UpgradeRelease(chartName string,
		namespace string, releaseName string, vals map[string]interface{}, opts ...ActionOption) (*release.Release, error)
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

func (m *defaultReleaseManager) InstallOrUpgradeRelease(chartName string,
	namespace string, releaseName string, vals map[string]interface{}, opts ...ActionOption) (*release.Release, error) {
	actionCFG := m.obtainActionConfig(namespace)
	historyAction := action.NewHistory(actionCFG)
	historyAction.Max = 1
	if _, err := historyAction.Run(releaseName); err == driver.ErrReleaseNotFound {
		return m.InstallRelease(chartName, namespace, releaseName, vals, opts...)
	} else {
		return m.UpgradeRelease(chartName, namespace, releaseName, vals, opts...)
	}
}

func (m *defaultReleaseManager) InstallRelease(chartName string,
	namespace string, releaseName string, vals map[string]interface{}, opts ...ActionOption) (*release.Release, error) {

	actionCFG := m.obtainActionConfig(namespace)
	installAction := action.NewInstall(actionCFG)
	installAction.Namespace = namespace
	installAction.SkipCRDs = false
	installAction.ReleaseName = releaseName

	actionOpts := ActionOptions{}
	actionOpts.ApplyOptions(opts)
	if actionOpts.Timeout != 0 {
		installAction.Wait = true
		installAction.Timeout = actionOpts.Timeout
	}

	cp, err := installAction.ChartPathOptions.LocateChart(chartName, cli.New())
	if err != nil {
		return nil, err
	}
	chartRequested, err := loader.Load(cp)
	if err != nil {
		return nil, err
	}

	return installAction.Run(chartRequested, vals)
}

func (m *defaultReleaseManager) UpgradeRelease(chartName string,
	namespace string, releaseName string, vals map[string]interface{}, opts ...ActionOption) (*release.Release, error) {

	actionCFG := m.obtainActionConfig(namespace)
	upgradeAction := action.NewUpgrade(actionCFG)
	upgradeAction.Namespace = namespace
	upgradeAction.ResetValues = true

	actionOpts := ActionOptions{}
	actionOpts.ApplyOptions(opts)
	if actionOpts.Timeout != 0 {
		upgradeAction.Wait = true
		upgradeAction.Timeout = actionOpts.Timeout
	}

	cp, err := upgradeAction.ChartPathOptions.LocateChart(chartName, cli.New())
	if err != nil {
		return nil, err
	}
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
