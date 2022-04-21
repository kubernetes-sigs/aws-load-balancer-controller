package service

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
)

const (
	defaultRawEnabled string = "false"
)

func (t *defaultModelBuildTask) buildEndpointService(ctx context.Context) error {
	enabled, err := t.buildEnabled(ctx)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	acceptanceRequired, err := t.buildAcceptanceRequired(ctx)
	if err != nil {
		return err
	}

	allowedPrinciples := t.buildAllowedPrinciples(ctx)
	privateDNSName := t.buildPrivateDNSName(ctx)

	tags, err := t.buildListenerTags(ctx)
	if err != nil {
		return err
	}

	esSpec := ec2model.VPCEndpointServiceSpec{
		AcceptanceRequired:      &acceptanceRequired,
		NetworkLoadBalancerArns: []core.StringToken{t.loadBalancer.LoadBalancerARN()},
		PrivateDNSName:          privateDNSName,
		Tags:                    tags,
	}

	es := ec2model.NewVPCEndpointService(t.stack, "VPCEndpointService", esSpec)

	espSpec := ec2model.VPCEndpointServicePermissionsSpec{
		AllowedPrinciples: allowedPrinciples,
		ServiceId:         es.ServiceID(),
	}

	_ = ec2model.NewVPCEndpointServicePermissions(t.stack, "VPCEndpointServicePermissions", espSpec)

	return nil
}

func (t *defaultModelBuildTask) buildEnabled(_ context.Context) (bool, error) {
	rawEnabled := defaultRawEnabled
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixEndpointServiceEnabled, &rawEnabled, t.service.Annotations)
	// We could use strconv here but we want to be explicit
	switch rawEnabled {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.Errorf("invalid service annotation %v, value must be one of [%v, %v]", annotations.SvcLBSuffixEndpointServiceEnabled, true, false)
	}
}

func (t *defaultModelBuildTask) buildAcceptanceRequired(_ context.Context) (bool, error) {
	rawAcceptanceRequired := ""
	_ = t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixEndpointServiceAcceptanceRequired, &rawAcceptanceRequired, t.service.Annotations)
	// We could use strconv here but we want to be explicit
	switch rawAcceptanceRequired {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.Errorf("invalid service annotation %v, value must be one of [%v, %v]", annotations.SvcLBSuffixEndpointServiceAcceptanceRequired, true, false)
	}
}

func (t *defaultModelBuildTask) buildAllowedPrinciples(_ context.Context) []string {
	var rawAllowedPrinciples []string
	if exists := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixEndpointServiceAllowedPrincipals, &rawAllowedPrinciples, t.service.Annotations); !exists {
		return []string{}
	}
	return rawAllowedPrinciples
}

func (t *defaultModelBuildTask) buildPrivateDNSName(_ context.Context) *string {
	rawPrivateDNSName := ""
	if exists := t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixEndpointServicePrivateDNSName, &rawPrivateDNSName, t.service.Annotations); !exists {
		return nil
	}
	return &rawPrivateDNSName
}
