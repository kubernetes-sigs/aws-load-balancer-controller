package deploy

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
)

func NewLoadBalancerActuator(cloud cloud.Cloud, tagProvider TagProvider, stack *build.LoadBalancingStack) Actuator {
	attrs := sets.NewString(
		build.DeletionProtectionEnabledKey,
		build.AccessLogsS3EnabledKey,
		build.AccessLogsS3BucketKey,
		build.AccessLogsS3PrefixKey,
	)
	if stack.LoadBalancer != nil && stack.LoadBalancer.Spec.LoadBalancerType == elbv2.LoadBalancerTypeEnumApplication {
		attrs.Insert(build.IdleTimeoutTimeoutSecondsKey, build.RoutingHTTP2EnabledKey)
	}

	return &loadBalancerActuator{
		cloud:          cloud,
		tagProvider:    tagProvider,
		stack:          stack,
		supportedAttrs: attrs,
	}
}

type loadBalancerActuator struct {
	cloud          cloud.Cloud
	tagProvider    TagProvider
	stack          *build.LoadBalancingStack
	supportedAttrs sets.String

	existingLBARNs []string
}

func (a *loadBalancerActuator) Initialize(ctx context.Context) error {
	lbARNs, err := a.findLoadBalancerByTags(ctx)
	if err != nil {
		return err
	}
	a.existingLBARNs = lbARNs

	if a.stack.LoadBalancer != nil {
		return a.reconcileLoadBalancer(ctx, a.stack.LoadBalancer, lbARNs)
	}
	return nil
}

func (a *loadBalancerActuator) Finalize(ctx context.Context) error {
	inUseLBARNs := sets.String{}
	if a.stack.LoadBalancer != nil {
		inUseLBARNs.Insert(a.stack.LoadBalancer.Status.ARN)
	}
	unUsedLBARNs := sets.NewString(a.existingLBARNs...).Difference(inUseLBARNs)

	for arn := range unUsedLBARNs {
		logging.FromContext(ctx).Info("deleting LoadBalancer", "ARN", arn)
		if _, err := a.cloud.ELBV2().DeleteLoadBalancerWithContext(ctx, &elbv2.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(arn),
		}); err != nil {
			return nil
		}
		logging.FromContext(ctx).Info("deleted LoadBalancer", "ARN", arn)
	}
	return nil
}

func (a *loadBalancerActuator) findLoadBalancerByTags(ctx context.Context) ([]string, error) {
	tags := a.tagProvider.TagResource(a.stack.ID, build.ResourceIDLoadBalancer, nil)

	req := &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters:          cloud.NewRGTTagFilters(tags),
		ResourceTypeFilters: aws.StringSlice([]string{cloud.ResourceTypeELBLoadBalancer}),
	}
	resources, err := a.cloud.RGT().GetResourcesAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(resources))
	for _, resource := range resources {
		lbARN := aws.StringValue(resource.ResourceARN)

		result = append(result, lbARN)
	}
	return result, nil
}

func (a *loadBalancerActuator) reconcileLoadBalancer(ctx context.Context, lb *api.LoadBalancer, lbARNs []string) error {
	var adoptableInstance *elbv2.LoadBalancer = nil
	if len(lbARNs) != 0 {
		lbInstances, err := a.cloud.ELBV2().DescribeLoadBalancersAsList(ctx, &elbv2.DescribeLoadBalancersInput{
			LoadBalancerArns: aws.StringSlice(lbARNs),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to describe LB")
		}
		for _, instance := range lbInstances {
			if a.isLBInstanceAdoptable(ctx, lb, instance) {
				adoptableInstance = instance
				break
			}
		}
	}

	var err error
	lbARN := ""
	if adoptableInstance == nil {
		if lbARN, err = a.createLBInstance(ctx, lb); err != nil {
			return err
		}
	} else {
		if lbARN, err = a.updateLBInstance(ctx, lb, adoptableInstance); err != nil {
			return err
		}
	}

	if err := a.reconcileLBInstanceAttributes(ctx, lbARN, lb.Spec.Attributes); err != nil {
		return err
	}

	if err := a.reconcileListeners(ctx, lb, lbARN); err != nil {
		return err
	}

	lb.Status.ARN = lbARN
	return nil
}

func (a *loadBalancerActuator) createLBInstance(ctx context.Context, lb *api.LoadBalancer) (string, error) {
	logging.FromContext(ctx).Info("creating LoadBalancer", "resource", lb.ObjectMeta.Name)

	sgIDs, err := a.resolveSecurityGroupReferences(ctx, lb.Spec.SecurityGroups)
	if err != nil {
		return "", err
	}

	tags := a.tagProvider.TagResource(a.stack.ID, build.ResourceIDLoadBalancer, lb.Spec.Tags)
	resp, err := a.cloud.ELBV2().CreateLoadBalancerWithContext(ctx, &elbv2.CreateLoadBalancerInput{
		Name:           aws.String(lb.Spec.LoadBalancerName),
		Type:           aws.String(lb.Spec.LoadBalancerType),
		Scheme:         aws.String(lb.Spec.Schema.String()),
		IpAddressType:  aws.String(lb.Spec.IPAddressType.String()),
		SubnetMappings: buildSubnetMapping(lb.Spec.SubnetMappings),
		SecurityGroups: aws.StringSlice(sgIDs),
		Tags:           convertToELBV2Tags(tags),
	})
	if err != nil {
		return "", err
	}

	lbARN := aws.StringValue(resp.LoadBalancers[0].LoadBalancerArn)
	lb.Status.DNSName = aws.StringValue(resp.LoadBalancers[0].DNSName)

	logging.FromContext(ctx).Info("created LoadBalancer", "resource", lb.ObjectMeta.Name, "ARN", lbARN)
	return lbARN, nil
}

func (a *loadBalancerActuator) updateLBInstance(ctx context.Context, lb *api.LoadBalancer, instance *elbv2.LoadBalancer) (string, error) {
	lbARN := aws.StringValue(instance.LoadBalancerArn)

	if !awsutil.DeepEqual(aws.StringValue(instance.IpAddressType), lb.Spec.IPAddressType.String()) {
		changeDesc := fmt.Sprintf("IpAddressType: %v => %v", aws.StringValue(instance.IpAddressType), lb.Spec.IPAddressType.String())
		logging.FromContext(ctx).Info("modifying LoadBalancer", "resource", lb.ObjectMeta.Name, "ARN", lbARN, "change", changeDesc)
		if _, err := a.cloud.ELBV2().SetIpAddressTypeWithContext(ctx, &elbv2.SetIpAddressTypeInput{
			LoadBalancerArn: aws.String(lbARN),
			IpAddressType:   aws.String(lb.Spec.IPAddressType.String()),
		}); err != nil {
			return lbARN, err
		}
		logging.FromContext(ctx).Info("modified LoadBalancer", "resource", lb.ObjectMeta.Name, "ARN", lbARN)
	}

	{
		desiredSubnets := sets.NewString()
		for _, mapping := range lb.Spec.SubnetMappings {
			desiredSubnets.Insert(mapping.SubnetID)
		}
		currentSubnets := sets.NewString()
		for _, az := range instance.AvailabilityZones {
			currentSubnets.Insert(aws.StringValue(az.SubnetId))
		}
		if !currentSubnets.Equal(desiredSubnets) {
			changeDesc := fmt.Sprintf("Subnets: %v => %v", currentSubnets.List(), desiredSubnets.List())
			logging.FromContext(ctx).Info("modifying LoadBalancer", "resource", lb.ObjectMeta.Name, "ARN", lbARN, "change", changeDesc)

			if _, err := a.cloud.ELBV2().SetSubnetsWithContext(ctx, &elbv2.SetSubnetsInput{
				LoadBalancerArn: aws.String(lbARN),
				SubnetMappings:  buildSubnetMapping(lb.Spec.SubnetMappings),
			}); err != nil {
				return lbARN, err
			}
			logging.FromContext(ctx).Info("modified LoadBalancer", "resource", lb.ObjectMeta.Name, "ARN", lbARN)
		}
	}

	{
		desiredSGIDs, err := a.resolveSecurityGroupReferences(ctx, lb.Spec.SecurityGroups)
		if err != nil {
			return lbARN, err
		}
		currentSecurityGroups := sets.NewString(aws.StringValueSlice(instance.SecurityGroups)...)
		if !currentSecurityGroups.Equal(sets.NewString(desiredSGIDs...)) {
			changeDesc := fmt.Sprintf("securityGroup: %v => %v", currentSecurityGroups.List(), desiredSGIDs)
			logging.FromContext(ctx).Info("modifying LoadBalancer", "resource", lb.ObjectMeta.Name, "ARN", lbARN, "change", changeDesc)

			if _, err := a.cloud.ELBV2().SetSecurityGroupsWithContext(ctx, &elbv2.SetSecurityGroupsInput{
				LoadBalancerArn: aws.String(lbARN),
				SecurityGroups:  aws.StringSlice(desiredSGIDs),
			}); err != nil {
				return lbARN, err
			}
			logging.FromContext(ctx).Info("modified LoadBalancer", "resource", lb.ObjectMeta.Name, "ARN", lbARN)
		}
	}

	{
		tags := a.tagProvider.TagResource(a.stack.ID, build.ResourceIDLoadBalancer, lb.Spec.Tags)
		if err := a.tagProvider.ReconcileELBV2Tags(ctx, lbARN, tags, false); err != nil {
			return lbARN, err
		}
	}

	lb.Status.DNSName = aws.StringValue(instance.DNSName)
	return lbARN, nil
}

// IsLBInstanceAdoptable checks whether an lb instance can be adopt by lb.
func (a *loadBalancerActuator) isLBInstanceAdoptable(ctx context.Context, lb *api.LoadBalancer, instance *elbv2.LoadBalancer) bool {
	return lb.Spec.Schema.String() == aws.StringValue(instance.Scheme)
}

func (a *loadBalancerActuator) resolveSecurityGroupReferences(ctx context.Context, sgRefs []api.SecurityGroupReference) ([]string, error) {
	var sgIDs []string
	for _, sgRef := range sgRefs {
		if len(sgRef.SecurityGroupID) != 0 {
			sgIDs = append(sgIDs, sgRef.SecurityGroupID)
		} else if sgRef.SecurityGroupRef.Name != "" {
			sg, exists := a.stack.FindSecurityGroup(sgRef.SecurityGroupRef.Name)
			// should never happen under current code
			if !exists || len(sg.Status.ID) == 0 {
				return nil, errors.Errorf("failed to resolve securityGroup: %v", sgRef.SecurityGroupRef.Name)
			}
			sgIDs = append(sgIDs, sg.Status.ID)
		}
	}
	return sgIDs, nil
}

func buildSubnetMapping(mappings []api.SubnetMapping) []*elbv2.SubnetMapping {
	var subnetMappings []*elbv2.SubnetMapping
	for _, mapping := range mappings {
		subnetMappings = append(subnetMappings, &elbv2.SubnetMapping{
			SubnetId: aws.String(mapping.SubnetID),
		})
	}
	return subnetMappings
}
