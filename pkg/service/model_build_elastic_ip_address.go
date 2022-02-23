package service

import (
	"context"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
)

func (t *defaultModelBuildTask) buildElasticIPAddress(ctx context.Context, resID string) (*ec2model.ElasticIPAddress, error) {
	eipSpec, err := t.buildElasticIPAddressSpec(ctx, resID)
	if err != nil {
		return nil, err
	}
	eip := ec2model.NewElasticIPAddress(t.stack, resID, eipSpec)
	return eip, nil
}

func (t *defaultModelBuildTask) buildElasticIPAddressSpec(ctx context.Context, resID string) (ec2model.ElasticIPAddressSpec, error) {
	var publicIpv4Pool string
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixEIPIpv4Pool, &publicIpv4Pool, t.service.Annotations)

	tags, err := t.buildElasticIPAddressTags(ctx)
	if err != nil {
		return ec2model.ElasticIPAddressSpec{}, err
	}

	spec := ec2model.ElasticIPAddressSpec{
		PublicIPv4PoolID: publicIpv4Pool,
		Tags:             tags,
	}
	return spec, nil
}

func (t *defaultModelBuildTask) buildElasticIPAddressTags(ctx context.Context) (map[string]string, error) {
	return t.buildAdditionalResourceTags(ctx)
}
