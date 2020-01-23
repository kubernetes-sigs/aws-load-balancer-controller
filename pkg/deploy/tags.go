package deploy

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
)

const (
	TagKeyClusterID  = "ingress.k8s.aws/cluster"
	TagKeyStackID    = "ingress.k8s.aws/stack"
	TagKeyResourceID = "ingress.k8s.aws/resource"
)

type TagProvider interface {
	TagStack(stackID string) map[string]string
	TagResource(stackID string, resourceID string, extraTags map[string]string) map[string]string

	ReconcileELBV2Tags(ctx context.Context, arn string, desiredTags map[string]string, isNewResource bool) error
	ReconcileEC2Tags(ctx context.Context, ec2ResID string, desiredTags map[string]string, actualTags []*ec2.Tag) error
}

func NewTagProvider(cloud cloud.Cloud) TagProvider {
	return &defaultTagProvider{
		cloud: cloud,
	}
}

type defaultTagProvider struct {
	cloud cloud.Cloud
}

func (p *defaultTagProvider) TagStack(stackID string) map[string]string {
	return map[string]string{
		TagKeyClusterID: p.cloud.ClusterName(),
		TagKeyStackID:   stackID,
	}
}

func (p *defaultTagProvider) TagResource(stackID string, resourceID string, extraTags map[string]string) map[string]string {
	tags := map[string]string{TagKeyResourceID: resourceID}
	stackTags := p.TagStack(stackID)
	for k, v := range stackTags {
		tags[k] = v
	}
	for k, v := range extraTags {
		tags[k] = v
	}
	return tags
}

func (p *defaultTagProvider) ReconcileELBV2Tags(ctx context.Context, arn string, desiredTags map[string]string, isNewResource bool) error {
	curTags := map[string]string{}
	if !isNewResource {
		var err error
		curTags, err = p.getCurrentELBV2Tags(ctx, arn)
		if err != nil {
			return err
		}
	}

	modify, remove := computeTagChangeSet(curTags, desiredTags)
	if len(modify) > 0 {
		logging.FromContext(ctx).Info("modifying tags", "arn", arn, "changes", awsutil.Prettify(modify))
		if _, err := p.cloud.ELBV2().AddTagsWithContext(ctx, &elbv2.AddTagsInput{
			ResourceArns: []*string{aws.String(arn)},
			Tags:         convertToELBV2Tags(modify),
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("modified tags", "arn", arn)
	}
	if len(remove) > 0 {
		logging.FromContext(ctx).Info("removing tags", "arn", arn, "changes", awsutil.Prettify(remove))
		tagKeys := sets.StringKeySet(remove).List()
		if _, err := p.cloud.ELBV2().RemoveTagsWithContext(ctx, &elbv2.RemoveTagsInput{
			ResourceArns: []*string{aws.String(arn)},
			TagKeys:      aws.StringSlice(tagKeys),
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("removed tags", "arn", arn)
	}
	return nil
}

func (p *defaultTagProvider) ReconcileEC2Tags(ctx context.Context, ec2ResID string, desiredTags map[string]string, actualTags []*ec2.Tag) error {
	curTags := convertFromEC2Tags(actualTags)
	modify, remove := computeTagChangeSet(curTags, desiredTags)
	if len(modify) > 0 {
		logging.FromContext(ctx).Info("modifying tags", "ID", ec2ResID, "changes", awsutil.Prettify(modify))

		if err := wait.PollImmediateUntil(2*time.Second, func() (done bool, err error) {
			if _, err := p.cloud.EC2().CreateTagsWithContext(ctx, &ec2.CreateTagsInput{
				Resources: []*string{aws.String(ec2ResID)},
				Tags:      convertToEC2Tags(modify),
			}); err != nil {
				if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "InvalidGroup.NotFound" {
					return false, nil
				}
				return false, err
			}

			return true, nil
		}, ctx.Done()); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("modified tags", "ID", ec2ResID)
	}
	if len(remove) > 0 {
		logging.FromContext(ctx).Info("removing tags", "ID", ec2ResID, "changes", awsutil.Prettify(remove))
		if _, err := p.cloud.EC2().DeleteTagsWithContext(ctx, &ec2.DeleteTagsInput{
			Resources: []*string{aws.String(ec2ResID)},
			Tags:      convertToEC2Tags(remove),
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("removing tags", "ID", ec2ResID)
	}
	return nil
}

func (c *defaultTagProvider) getCurrentELBV2Tags(ctx context.Context, arn string) (map[string]string, error) {
	resp, err := c.cloud.ELBV2().DescribeTagsWithContext(ctx, &elbv2.DescribeTagsInput{
		ResourceArns: aws.StringSlice([]string{arn}),
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

// computeTagChangeSet compares current with desired, return the add/change and remove tags to reach desired from current.
func computeTagChangeSet(current map[string]string, desired map[string]string) (map[string]string, map[string]string) {
	modify := make(map[string]string)
	remove := make(map[string]string)

	for key, desiredVal := range desired {
		currentVal, ok := current[key]
		if !ok || currentVal != desiredVal {
			modify[key] = desiredVal
		}
	}
	for key, currentVal := range current {
		if _, ok := desired[key]; !ok {
			remove[key] = currentVal
		}
	}

	return modify, remove
}

// convertToELBV2Tags will convert tags to ELBV2 Tags
func convertToELBV2Tags(tags map[string]string) []*elbv2.Tag {
	output := make([]*elbv2.Tag, 0, len(tags))
	for k, v := range tags {
		output = append(output, &elbv2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return output
}

// convertToEC2Tags will convert tags to EC2 tags
func convertToEC2Tags(tags map[string]string) []*ec2.Tag {
	output := make([]*ec2.Tag, 0, len(tags))
	for k, v := range tags {
		output = append(output, &ec2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return output
}

// convertFromEC2Tags will converts tags from EC2 tags
func convertFromEC2Tags(tags []*ec2.Tag) map[string]string {
	output := map[string]string{}
	for _, tag := range tags {
		output[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
	}
	return output
}
