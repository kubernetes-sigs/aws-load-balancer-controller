package tags

import (
	"context"

	"github.com/aws/aws-sdk-go/service/ec2"
	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// Controller manages tags on a resource
type Controller interface {
	// ReconcileELB ensures the tag for ELB resources denoted by arn have specified tags.
	ReconcileELB(ctx context.Context, arn string, desiredTags map[string]string) error

	// ReconcileEC2WithCurTags ensures the tag for EC2 resources denoted by resourceID have specified tags by reconcile from curTags.
	ReconcileEC2WithCurTags(ctx context.Context, resourceID string, desiredTags map[string]string, curTags map[string]string) error
}

// NewController constructs a new tags controller
func NewController(cloud aws.CloudAPI) Controller {
	return &controller{
		cloud: cloud,
	}
}

type controller struct {
	cloud aws.CloudAPI
}

func (c *controller) ReconcileELB(ctx context.Context, arn string, desiredTags map[string]string) error {
	curTags, err := c.getCurrentELBTags(ctx, arn)
	if err != nil {
		return err
	}
	modify, remove := changeSets(curTags, desiredTags)
	if len(modify) > 0 {
		albctx.GetLogger(ctx).Infof("modifying tags %v on %v", log.Prettify(modify), arn)
		if _, err := c.cloud.AddELBV2TagsWithContext(ctx, &elbv2.AddTagsInput{
			ResourceArns: []*string{aws.String(arn)},
			Tags:         ConvertToELBV2(modify),
		}); err != nil {
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "error tagging %s due to %s", arn, err)
			return err
		}
	}
	if len(remove) > 0 {
		albctx.GetLogger(ctx).Infof("removing tags %v on %v", log.Prettify(remove), arn)
		tagKeys := sets.StringKeySet(remove).List()
		if _, err := c.cloud.RemoveELBV2TagsWithContext(ctx, &elbv2.RemoveTagsInput{
			ResourceArns: []*string{aws.String(arn)},
			TagKeys:      aws.StringSlice(tagKeys),
		}); err != nil {
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "error tagging %s due to %s", arn, err)
			return err
		}
	}
	return nil
}

func (c *controller) ReconcileEC2WithCurTags(ctx context.Context, resourceID string, desiredTags map[string]string, curTags map[string]string) error {
	modify, remove := changeSets(curTags, desiredTags)
	if len(modify) > 0 {
		albctx.GetLogger(ctx).Infof("modifying tags %v on %v", log.Prettify(modify), resourceID)
		if _, err := c.cloud.CreateEC2TagsWithContext(ctx, &ec2.CreateTagsInput{
			Resources: []*string{aws.String(resourceID)},
			Tags:      ConvertToEC2(modify),
		}); err != nil {
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "error tagging %s due to %s", resourceID, err)
			return err
		}
	}
	if len(remove) > 0 {
		albctx.GetLogger(ctx).Infof("removing tags %v on %v", log.Prettify(remove), resourceID)
		if _, err := c.cloud.DeleteEC2TagsWithContext(ctx, &ec2.DeleteTagsInput{
			Resources: []*string{aws.String(resourceID)},
			Tags:      ConvertToEC2(remove),
		}); err != nil {
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "error tagging %s due to %s", resourceID, err)
			return err
		}
	}
	return nil
}

func (c *controller) getCurrentELBTags(ctx context.Context, arn string) (map[string]string, error) {
	resp, err := c.cloud.DescribeELBV2TagsWithContext(ctx, &elbv2.DescribeTagsInput{
		ResourceArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return nil, err
	}
	tags := make(map[string]string)
	for _, tagDescription := range resp.TagDescriptions {
		for _, tag := range tagDescription.Tags {
			tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
		}
	}
	return tags, nil
}

// changeSets compares source with target, return the add/change and remove tags to reach target from source.
func changeSets(source, target map[string]string) (map[string]string, map[string]string) {
	modify := make(map[string]string)
	remove := make(map[string]string)

	for key, targetVal := range target {
		sourceVal, ok := source[key]
		if !ok || sourceVal != targetVal {
			modify[key] = targetVal
		}
	}
	for key, sourceVal := range source {
		if _, ok := target[key]; !ok {
			remove[key] = sourceVal
		}
	}

	return modify, remove
}

// ConvertToELBV2 will convert tags to ELBV2 Tags
func ConvertToELBV2(tags map[string]string) []*elbv2.Tag {
	output := make([]*elbv2.Tag, 0, len(tags))
	for k, v := range tags {
		output = append(output, &elbv2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return output
}

// ConvertToEC2 will convert tags to EC2 tags
func ConvertToEC2(tags map[string]string) []*ec2.Tag {
	output := make([]*ec2.Tag, 0, len(tags))
	for k, v := range tags {
		output = append(output, &ec2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return output
}
