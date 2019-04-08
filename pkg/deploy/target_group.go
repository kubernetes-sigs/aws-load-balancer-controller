package deploy

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"k8s.io/apimachinery/pkg/util/sets"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
)

func NewTargetGroupActuator(cloud cloud.Cloud, tagProvider TagProvider, stack *build.LoadBalancingStack) Actuator {
	return &targetGroupActuator{
		cloud:       cloud,
		tagProvider: tagProvider,
		stack:       stack,
	}
}

type targetGroupActuator struct {
	cloud       cloud.Cloud
	tagProvider TagProvider
	stack       *build.LoadBalancingStack

	existingTGARNs []string
}

func (a *targetGroupActuator) Initialize(ctx context.Context) error {
	tgARNsByID, err := a.findTargetGroupsByTags(ctx)
	if err != nil {
		return err
	}
	for _, tgARNs := range tgARNsByID {
		a.existingTGARNs = append(a.existingTGARNs, tgARNs...)
	}

	for _, tg := range a.stack.TargetGroups {
		if err := a.reconcileTargetGroup(ctx, tg, tgARNsByID[tg.Name]); err != nil {
			return err
		}
	}

	return nil
}

func (a *targetGroupActuator) Finalize(ctx context.Context) error {
	inUseTGARNs := sets.String{}
	for _, tg := range a.stack.TargetGroups {
		inUseTGARNs.Insert(tg.Status.ARN)
	}

	unusedTargetGroups := sets.NewString(a.existingTGARNs...).Difference(inUseTGARNs)
	for arn := range unusedTargetGroups {
		logging.FromContext(ctx).Info("deleting targetGroup", "arn", arn)
		if _, err := a.cloud.ELBV2().DeleteTargetGroupWithContext(ctx, &elbv2.DeleteTargetGroupInput{
			TargetGroupArn: aws.String(arn),
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("deleted targetGroup", "arn", arn)
	}

	return nil
}

// findTargetGroupsByTags returns an map of tgID => tgARNs.
func (a *targetGroupActuator) findTargetGroupsByTags(ctx context.Context) (map[string][]string, error) {
	tags := a.tagProvider.TagStack(a.stack.ID)
	req := &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters:          cloud.NewRGTTagFilters(tags),
		ResourceTypeFilters: aws.StringSlice([]string{cloud.ResourceTypeELBTargetGroup}),
	}
	resources, err := a.cloud.RGT().GetResourcesAsList(ctx, req)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]string, len(resources))
	for _, resource := range resources {
		tgTags := cloud.ParseRGTTags(resource.Tags)
		tgID := tgTags[TagKeyResourceID]
		tgARN := aws.StringValue(resource.ResourceARN)

		result[tgID] = append(result[tgID], tgARN)
	}
	return result, nil
}

// reconcileTargetGroup will first try to found an targetGroup from tgARNs that can be used and reconcile it.
// otherwise, it will create an new targetGroup.
func (a *targetGroupActuator) reconcileTargetGroup(ctx context.Context, tg *api.TargetGroup, tgARNs []string) error {
	var adoptableInstance *elbv2.TargetGroup = nil
	if len(tgARNs) != 0 {
		// TODO(@M00nF1sh): Optimize this, targetGroups for whole cluster can be described together.
		tgInstances, err := a.cloud.ELBV2().DescribeTargetGroupsAsList(ctx, &elbv2.DescribeTargetGroupsInput{
			TargetGroupArns: aws.StringSlice(tgARNs),
		})
		if err != nil {
			return err
		}
		for _, instance := range tgInstances {
			if a.isTGInstanceAdoptable(ctx, tg, instance) {
				adoptableInstance = instance
				break
			}
		}
	}

	var err error
	tgARN := ""
	if adoptableInstance == nil {
		if tgARN, err = a.createTGInstance(ctx, tg); err != nil {
			return err
		}
	} else {
		if tgARN, err = a.updateTGInstance(ctx, tg, adoptableInstance); err != nil {
			return err
		}
	}

	if err := a.reconcileTGInstanceAttributes(ctx, tgARN, tg.Spec.Attributes); err != nil {
		return err
	}

	tg.Status.ARN = tgARN
	return nil
}

func (a *targetGroupActuator) createTGInstance(ctx context.Context, tg *api.TargetGroup) (string, error) {
	logging.FromContext(ctx).Info("creating targetGroup", "resource", tg.ObjectMeta.Name)
	resp, err := a.cloud.ELBV2().CreateTargetGroupWithContext(ctx, &elbv2.CreateTargetGroupInput{
		Name:                       aws.String(tg.Spec.TargetGroupName),
		TargetType:                 aws.String(tg.Spec.TargetType.String()),
		Protocol:                   aws.String(tg.Spec.Protocol.String()),
		Port:                       aws.Int64(tg.Spec.Port),
		HealthCheckPath:            aws.String(tg.Spec.HealthCheckConfig.Path),
		HealthCheckPort:            aws.String(tg.Spec.HealthCheckConfig.Port.String()),
		HealthCheckProtocol:        aws.String(tg.Spec.HealthCheckConfig.Protocol.String()),
		HealthCheckIntervalSeconds: aws.Int64(tg.Spec.HealthCheckConfig.IntervalSeconds),
		HealthCheckTimeoutSeconds:  aws.Int64(tg.Spec.HealthCheckConfig.TimeoutSeconds),
		HealthyThresholdCount:      aws.Int64(tg.Spec.HealthCheckConfig.HealthyThresholdCount),
		UnhealthyThresholdCount:    aws.Int64(tg.Spec.HealthCheckConfig.UnhealthyThresholdCount),
		Matcher:                    &elbv2.Matcher{HttpCode: aws.String(tg.Spec.HealthCheckConfig.Matcher.HTTPCode)},
		VpcId:                      aws.String(a.cloud.VpcID()),
	})
	if err != nil {
		return "", err
	}
	tgArn := aws.StringValue(resp.TargetGroups[0].TargetGroupArn)
	logging.FromContext(ctx).Info("created targetGroup", "resource", tg.ObjectMeta.Name, "arn", tgArn)

	tags := a.tagProvider.TagResource(a.stack.ID, tg.Name, tg.Spec.Tags)
	if err := a.tagProvider.ReconcileELBV2Tags(ctx, tgArn, tags, true); err != nil {
		return "", err
	}
	return tgArn, nil
}

func (a *targetGroupActuator) updateTGInstance(ctx context.Context, tg *api.TargetGroup, instance *elbv2.TargetGroup) (string, error) {
	tgArn := aws.StringValue(instance.TargetGroupArn)
	if a.isTGInstanceModified(ctx, tg, instance) {
		logging.FromContext(ctx).Info("modifying targetGroup", "resource", tg.ObjectMeta.Name, "arn", tgArn)
		if _, err := a.cloud.ELBV2().ModifyTargetGroupWithContext(ctx, &elbv2.ModifyTargetGroupInput{
			TargetGroupArn:             instance.TargetGroupArn,
			HealthCheckPath:            aws.String(tg.Spec.HealthCheckConfig.Path),
			HealthCheckPort:            aws.String(tg.Spec.HealthCheckConfig.Port.String()),
			HealthCheckProtocol:        aws.String(tg.Spec.HealthCheckConfig.Protocol.String()),
			HealthCheckIntervalSeconds: aws.Int64(tg.Spec.HealthCheckConfig.IntervalSeconds),
			HealthCheckTimeoutSeconds:  aws.Int64(tg.Spec.HealthCheckConfig.TimeoutSeconds),
			HealthyThresholdCount:      aws.Int64(tg.Spec.HealthCheckConfig.HealthyThresholdCount),
			UnhealthyThresholdCount:    aws.Int64(tg.Spec.HealthCheckConfig.UnhealthyThresholdCount),
			Matcher:                    &elbv2.Matcher{HttpCode: aws.String(tg.Spec.HealthCheckConfig.Matcher.HTTPCode)},
		}); err != nil {
			return tgArn, err
		}
		logging.FromContext(ctx).Info("modified targetGroup", "resource", tg.ObjectMeta.Name, "arn", tgArn)
	}

	tags := a.tagProvider.TagResource(a.stack.ID, tg.Name, tg.Spec.Tags)
	if err := a.tagProvider.ReconcileELBV2Tags(ctx, tgArn, tags, false); err != nil {
		return tgArn, err
	}
	return tgArn, nil
}

func (a *targetGroupActuator) isTGInstanceAdoptable(ctx context.Context, tg *api.TargetGroup, instance *elbv2.TargetGroup) bool {
	if tg.Spec.TargetType.String() != aws.StringValue(instance.TargetType) {
		return false
	}
	if tg.Spec.Protocol.String() != aws.StringValue(instance.Protocol) {
		return false
	}

	return true
}

func (a *targetGroupActuator) isTGInstanceModified(ctx context.Context, tg *api.TargetGroup, instance *elbv2.TargetGroup) bool {
	needsChange := false

	if !awsutil.DeepEqual(tg.Spec.HealthCheckConfig.Path, aws.StringValue(instance.HealthCheckPath)) {
		needsChange = true
	}
	if !awsutil.DeepEqual(tg.Spec.HealthCheckConfig.Port.String(), aws.StringValue(instance.HealthCheckPort)) {
		needsChange = true
	}
	if !awsutil.DeepEqual(tg.Spec.HealthCheckConfig.Protocol.String(), aws.StringValue(instance.HealthCheckProtocol)) {
		needsChange = true
	}
	if !awsutil.DeepEqual(tg.Spec.HealthCheckConfig.IntervalSeconds, aws.Int64Value(instance.HealthCheckIntervalSeconds)) {
		needsChange = true
	}
	if !awsutil.DeepEqual(tg.Spec.HealthCheckConfig.TimeoutSeconds, aws.Int64Value(instance.HealthCheckTimeoutSeconds)) {
		needsChange = true
	}
	if !awsutil.DeepEqual(tg.Spec.HealthCheckConfig.HealthyThresholdCount, aws.Int64Value(instance.HealthyThresholdCount)) {
		needsChange = true
	}
	if !awsutil.DeepEqual(tg.Spec.HealthCheckConfig.UnhealthyThresholdCount, aws.Int64Value(instance.UnhealthyThresholdCount)) {
		needsChange = true
	}
	return needsChange
}
