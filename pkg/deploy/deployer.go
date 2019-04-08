package deploy

import (
	"context"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
)

type Deployer interface {
	Deploy(ctx context.Context, stack *build.LoadBalancingStack) error
}

// Actuator is responsible for setup & cleanup an resource or a set of resources.
// During setup, it's responsible for identify existing resources and configure them.
// During cleanup, it's responsible for GC extra resources.
type Actuator interface {
	// Initialize is responsible for identify existing resources and configure them.
	Initialize(ctx context.Context) error

	// Finalize is responsible for GC extra resources.
	Finalize(ctx context.Context) error
}

func NewDeployer(cloud cloud.Cloud, ebRepo backend.EndpointBindingRepo) Deployer {
	tagProvider := NewTagProvider(cloud)

	return &defaultDeployer{
		cloud:       cloud,
		ebRepo:      ebRepo,
		tagProvider: tagProvider,
	}
}

type defaultDeployer struct {
	cloud       cloud.Cloud
	ebRepo      backend.EndpointBindingRepo
	tagProvider TagProvider
}

func (d *defaultDeployer) Deploy(ctx context.Context, stack *build.LoadBalancingStack) error {
	targetGroupActuator := NewTargetGroupActuator(d.cloud, d.tagProvider, stack)
	endpointBindingActuator := NewEndpointBindingActuator(d.ebRepo, stack)
	lbSecurityGroupActuator := NewLBSecurityGroupActuator(d.cloud, d.tagProvider, stack)
	loadBalancerActuator := NewLoadBalancerActuator(d.cloud, d.tagProvider, stack)

	actuators := []Actuator{targetGroupActuator, endpointBindingActuator, lbSecurityGroupActuator, loadBalancerActuator}
	for _, actuator := range actuators {
		if err := actuator.Initialize(ctx); err != nil {
			return err
		}
	}
	for i := len(actuators) - 1; i >= 0; i-- {
		if err := actuators[i].Finalize(ctx); err != nil {
			return err
		}
	}

	return nil
}
