package globalaccelerator

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	agav1beta1 "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EndpointStack defines the interface for endpoint resource stacks (Service/Ingress/Gateway)
type EndpointStack interface {
	Deploy(context.Context, *framework.Framework) error
	Cleanup(context.Context, *framework.Framework) error
	GetNamespace() string
}

// ResourceStack orchestrates the deployment of endpoint resources with GlobalAccelerator
type ResourceStack struct {
	endpointStack EndpointStack                 // Endpoint resources (Service/Ingress/Gateway)
	agaSpec       *agav1beta1.GlobalAccelerator // Desired GlobalAccelerator specification
	deployedAGA   *agav1beta1.GlobalAccelerator // Deployed GlobalAccelerator with AWS status
}

// NewResourceStack creates a new ResourceStack with endpoint resources and GlobalAccelerator spec
func NewResourceStack(endpointStack EndpointStack, agaSpec *agav1beta1.GlobalAccelerator) *ResourceStack {
	return &ResourceStack{
		endpointStack: endpointStack,
		agaSpec:       agaSpec,
	}
}

// Deploy creates endpoint resources first, then GlobalAccelerator
func (s *ResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	// Deploy endpoint resources (Ingress/Service/Gateway) first
	if s.endpointStack != nil {
		if err := s.endpointStack.Deploy(ctx, f); err != nil {
			return err
		}
	}
	// Create GlobalAccelerator and wait for AWS provisioning
	if s.agaSpec != nil {
		if err := s.createGlobalAccelerator(ctx, f); err != nil {
			return err
		}
		if err := s.waitUntilGlobalAcceleratorReady(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

// UpdateGlobalAccelerator applies changes to the GlobalAccelerator spec and waits for reconciliation
func (s *ResourceStack) UpdateGlobalAccelerator(ctx context.Context, f *framework.Framework, updateFn func(*agav1beta1.GlobalAccelerator)) error {
	oldSpec := s.agaSpec.DeepCopy()
	updateFn(s.agaSpec)
	if err := f.K8sClient.Patch(ctx, s.agaSpec, client.MergeFrom(oldSpec)); err != nil {
		return err
	}
	return s.waitUntilGlobalAcceleratorReady(ctx, f)
}

// Cleanup deletes GlobalAccelerator first, then endpoint resources (reverse order of Deploy)
func (s *ResourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	// Delete GlobalAccelerator first
	if s.agaSpec != nil {
		if err := s.deleteGlobalAccelerator(ctx, f); err != nil {
			return err
		}
	}
	// Delete endpoint resources
	if s.endpointStack != nil {
		return s.endpointStack.Cleanup(ctx, f)
	}
	return nil
}

func (s *ResourceStack) GetGlobalAcceleratorARN() string {
	if s.deployedAGA != nil && s.deployedAGA.Status.AcceleratorARN != nil {
		return *s.deployedAGA.Status.AcceleratorARN
	}
	return ""
}

func (s *ResourceStack) GetGlobalAcceleratorDNSName() string {
	if s.deployedAGA != nil && s.deployedAGA.Status.DNSName != nil {
		return *s.deployedAGA.Status.DNSName
	}
	return ""
}

func (s *ResourceStack) GetGlobalAcceleratorDualStackDNSName() string {
	if s.deployedAGA != nil && s.deployedAGA.Status.DualStackDnsName != nil {
		return *s.deployedAGA.Status.DualStackDnsName
	}
	return ""
}

func (s *ResourceStack) createGlobalAccelerator(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating globalaccelerator", "aga", k8s.NamespacedName(s.agaSpec))
	if err := f.K8sClient.Create(ctx, s.agaSpec); err != nil {
		return err
	}
	f.Logger.Info("created globalaccelerator", "aga", k8s.NamespacedName(s.agaSpec))
	return nil
}

func (s *ResourceStack) waitUntilGlobalAcceleratorReady(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("waiting for globalaccelerator to be ready", "aga", k8s.NamespacedName(s.agaSpec))
	var err error
	s.deployedAGA, err = waitUntilGlobalAcceleratorActive(ctx, f, s.agaSpec)
	if err != nil {
		return err
	}
	f.Logger.Info("globalaccelerator is ready", "aga", k8s.NamespacedName(s.agaSpec))
	return nil
}

func (s *ResourceStack) deleteGlobalAccelerator(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("deleting globalaccelerator", "aga", k8s.NamespacedName(s.agaSpec))
	if err := f.K8sClient.Delete(ctx, s.agaSpec); err != nil {
		return err
	}
	if err := waitUntilGlobalAcceleratorDeleted(ctx, f, s.agaSpec); err != nil {
		return err
	}
	f.Logger.Info("deleted globalaccelerator", "aga", k8s.NamespacedName(s.agaSpec))
	return nil
}

// waitUntilGlobalAcceleratorActive polls until GlobalAccelerator is provisioned in AWS with status DEPLOYED
func waitUntilGlobalAcceleratorActive(ctx context.Context, f *framework.Framework, aga *agav1beta1.GlobalAccelerator) (*agav1beta1.GlobalAccelerator, error) {
	observedAGA := &agav1beta1.GlobalAccelerator{}
	return observedAGA, wait.PollImmediateUntil(utils.PollIntervalMedium, func() (bool, error) {
		if err := f.K8sClient.Get(ctx, k8s.NamespacedName(aga), observedAGA); err != nil {
			return false, err
		}
		// Check if AWS has populated ARN and DNS
		if observedAGA.Status.AcceleratorARN == nil || observedAGA.Status.DNSName == nil {
			return false, nil
		}
		// Check if status is DEPLOYED
		if observedAGA.Status.Status != nil && *observedAGA.Status.Status == "DEPLOYED" {
			return true, nil
		}
		return false, nil
	}, ctx.Done())
}

// waitUntilGlobalAcceleratorDeleted polls until GlobalAccelerator resource is removed from K8s
func waitUntilGlobalAcceleratorDeleted(ctx context.Context, f *framework.Framework, aga *agav1beta1.GlobalAccelerator) error {
	observedAGA := &agav1beta1.GlobalAccelerator{}
	return wait.PollImmediateUntil(utils.PollIntervalMedium, func() (bool, error) {
		if err := f.K8sClient.Get(ctx, k8s.NamespacedName(aga), observedAGA); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, ctx.Done())
}
