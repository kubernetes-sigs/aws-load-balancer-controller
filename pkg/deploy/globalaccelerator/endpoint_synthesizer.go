package globalaccelerator

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	gamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/globalaccelerator"
)

// NewEndpointSynthesizer constructs new endpointSynthesizer
func NewEndpointSynthesizer(epManager EndpointManager, logger logr.Logger, stack core.Stack) *endpointSynthesizer {
	return &endpointSynthesizer{
		endpointManager: epManager,
		logger:          logger,
		stack:           stack,
	}
}

type endpointSynthesizer struct {
	endpointManager EndpointManager
	logger          logr.Logger
	stack           core.Stack
}

func (s *endpointSynthesizer) Synthesize(ctx context.Context) error {
	var resEndpoints []*gamodel.Endpoint
	s.stack.ListResources(&resEndpoints)
	resEndpointsByARN, err := mapResEndpointByResourceARN(resEndpoints)
	if err != nil {
		return err
	}

	var resLBs []*elbv2model.LoadBalancer
	s.stack.ListResources(&resLBs)
	for _, resLB := range resLBs {
		// Global Accelerator can only be created for ALB for now.
		if resLB.Spec.Type != elbv2model.LoadBalancerTypeApplication {
			continue
		}
		lbARN, err := resLB.LoadBalancerARN().Resolve(ctx)
		if err != nil {
			return err
		}
		resEndpoints := resEndpointsByARN[lbARN]

		if err := s.synthesizeGAEndpoints(ctx, lbARN, resEndpoints); err != nil {
			return err
		}
	}

	return nil
}

func (*endpointSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here.
	return nil
}

func (s *endpointSynthesizer) synthesizeGAEndpoints(ctx context.Context, lbARN string, resEndpoints []*gamodel.Endpoint) error {
	if len(resEndpoints) > 1 {
		return fmt.Errorf("[should never happen] multiple Global Accelerator Endpoints desired on LoadBalancer: %v", lbARN)
	}

	var desiredEndpointGroupARN string
	var desiredEndpointCreate bool
	if len(resEndpoints) == 1 {
		desiredEndpointGroupARN = resEndpoints[0].Spec.EndpointGroupARN
		desiredEndpointCreate = resEndpoints[0].Spec.Create
	}

	if desiredEndpointGroupARN != "" {
		// no lbARN means delete
		if !desiredEndpointCreate {
			s.endpointManager.DeleteEndpoint(ctx, desiredEndpointGroupARN, lbARN)
			return nil
		}

		existingEndpoint, err := s.endpointManager.GetEndpoint(ctx, desiredEndpointGroupARN, lbARN)
		if err != nil {
			return err
		}
		if existingEndpoint == "" {
			s.endpointManager.AddEndpoint(ctx, desiredEndpointGroupARN, lbARN)
		}
	}

	return nil
}

func mapResEndpointByResourceARN(resEndpoints []*gamodel.Endpoint) (map[string][]*gamodel.Endpoint, error) {
	resEndpointsByARN := make(map[string][]*gamodel.Endpoint, len(resEndpoints))
	ctx := context.Background()
	for _, resEndpoint := range resEndpoints {
		resARN, err := resEndpoint.Spec.ResourceARN.Resolve(ctx)
		if err != nil {
			return nil, err
		}
		resEndpointsByARN[resARN] = append(resEndpointsByARN[resARN], resEndpoint)
	}
	return resEndpointsByARN, nil
}
