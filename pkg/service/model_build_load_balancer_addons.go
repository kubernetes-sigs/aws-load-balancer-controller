// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package service

import (
	"context"

	"github.com/pkg/errors"

	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/shield"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/shared_constants"
)

// buildLoadBalancerAddOns builds the load balancer add-ons, e.g. AWS Shield Advanced protection.
func (t *defaultModelBuildTask) buildLoadBalancerAddOns(ctx context.Context, lbARN core.StringToken) error {
	if _, err := t.buildShieldProtection(ctx, lbARN); err != nil {
		return err
	}
	return nil
}

// buildShieldProtection builds the AWS Shield Advanced protection for the load balancer.
// Shield Advanced protection is only supported on Network Load Balancers.
func (t *defaultModelBuildTask) buildShieldProtection(_ context.Context, lbARN core.StringToken) (*shieldmodel.Protection, error) {
	rawEnableProtection := false
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixShieldAdvancedProtection, &rawEnableProtection, t.service.Annotations)
	if err != nil {
		return nil, err
	}
	if !exists {
		// when the annotation is absent, keep the shield protection settings unchanged
		return nil, nil
	}
	if t.loadBalancer.Spec.Type != elbv2model.LoadBalancerTypeNetwork {
		return nil, errors.Errorf("shield advanced protection is only supported on NLB, but the service %s/%s does not build an NLB", t.service.Namespace, t.service.Name)
	}
	protection := shieldmodel.NewProtection(t.stack, shared_constants.ResourceIDLoadBalancer, shieldmodel.ProtectionSpec{
		Enabled:     rawEnableProtection,
		ResourceARN: lbARN,
	})
	return protection, nil
}
