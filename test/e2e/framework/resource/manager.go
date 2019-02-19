package resource

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

type Manager struct {
	*NamespaceManager
	*DeploymentManager
	*ServiceManager
	*IngressManager
}

func NewManager(cs kubernetes.Interface) *Manager {
	return &Manager{
		NamespaceManager:  NewNamespaceManager(cs),
		DeploymentManager: NewDeploymentManager(cs),
		ServiceManager:    NewServiceManager(cs),
		IngressManager:    NewIngressManager(cs),
	}
}

func (f *Manager) Cleanup(ctx context.Context) error {
	// Currently, clean up namespace deletes everything else as well :D.
	if err := f.NamespaceManager.Cleanup(ctx); err != nil {
		return err
	}
	return nil
}
