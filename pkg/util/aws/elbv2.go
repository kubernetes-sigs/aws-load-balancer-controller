package aws

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

const (
	// Amount of time between each deletion attempt (or reattempt) for a target group
	deleteTargetGroupReattemptSleep int = 10
	// Maximum attempts should be made to delete a target group
	deleteTargetGroupReattemptMax int = 10
)

// ELBV2 is our extension to AWS's elbv2.ELBV2
type ELBV2 struct {
	elbv2iface.ELBV2API
}

// NewELBV2 returns an ELBV2 based off of the provided AWS session
func NewELBV2(awsSession *session.Session) *ELBV2 {
	elbClient := ELBV2{
		elbv2.New(awsSession),
	}
	return &elbClient
}

// RemoveListener removes a Listener from an ELBV2 (ALB) by deleting it in AWS. If the deletion
// attempt returns a elbv2.ErrCodeListenerNotFoundException, it's considered a success as the
// listener has already been removed. If removal fails for another reason, an error is returned.
func (e *ELBV2) RemoveListener(in elbv2.DeleteListenerInput) error {
	if _, err := e.DeleteListener(&in); err != nil {
		awsErr := err.(awserr.Error)
		if awsErr.Code() != elbv2.ErrCodeListenerNotFoundException {
			return err
		}
	}

	return nil
}

// RemoveTargetGroup removes a Target Group from AWS by deleting it. If the deletion fails, an error
// is returned. Often, a Listener that references the Target Group is still being deleted when this
// method is accessed. Thus, this method makes multiple attempts to delete the Target Group when it
// receives an elbv2.ErrCodeResourceInUseException.
func (e *ELBV2) RemoveTargetGroup(in elbv2.DeleteTargetGroupInput) error {
	for i := 0; i < deleteTargetGroupReattemptMax; i++ {
		_, err := e.DeleteTargetGroup(&in)
		switch {
		case err != nil && err.(awserr.Error).Code() == elbv2.ErrCodeResourceInUseException:
			time.Sleep(time.Duration(deleteTargetGroupReattemptSleep) * time.Second)
			continue
		case err != nil:
			return err
		}
	}

	return nil
}

// GetClusterLoadBalancers looks up all ELBV2 (ALB) instances in AWS that are part of the cluster.
func (e *ELBV2) GetClusterLoadBalancers(clusterName *string) ([]*elbv2.LoadBalancer, error) {
	var loadbalancers []*elbv2.LoadBalancer

	err := e.DescribeLoadBalancersPagesWithContext(context.Background(),
		&elbv2.DescribeLoadBalancersInput{},
		func(p *elbv2.DescribeLoadBalancersOutput, lastPage bool) bool {
			for _, loadBalancer := range p.LoadBalancers {
				if strings.HasPrefix(*loadBalancer.LoadBalancerName, *clusterName+"-") {
					if s := strings.Split(*loadBalancer.LoadBalancerName, "-"); len(s) == 2 {
						if s[0] == *clusterName {
							loadbalancers = append(loadbalancers, loadBalancer)
						}
					}
				}
			}
			return true
		})
	if err != nil {
		return nil, err
	}

	return loadbalancers, nil
}

// DescribeTargetGroupsForLoadBalancer looks up all ELBV2 (ALB) target groups in AWS that are part of the cluster.
func (e *ELBV2) DescribeTargetGroupsForLoadBalancer(loadBalancerArn *string) ([]*elbv2.TargetGroup, error) {
	var targetGroups []*elbv2.TargetGroup

	err := e.DescribeTargetGroupsPagesWithContext(context.Background(),
		&elbv2.DescribeTargetGroupsInput{LoadBalancerArn: loadBalancerArn},
		func(p *elbv2.DescribeTargetGroupsOutput, lastPage bool) bool {
			for _, targetGroup := range p.TargetGroups {
				targetGroups = append(targetGroups, targetGroup)
			}
			return true
		})
	if err != nil {
		return nil, err
	}

	return targetGroups, nil
}

// DescribeListenersForLoadBalancer looks up all ELBV2 (ALB) listeners in AWS that are part of the cluster.
func (e *ELBV2) DescribeListenersForLoadBalancer(loadBalancerArn *string) ([]*elbv2.Listener, error) {
	var listeners []*elbv2.Listener

	err := e.DescribeListenersPagesWithContext(context.Background(),
		&elbv2.DescribeListenersInput{LoadBalancerArn: loadBalancerArn},
		func(p *elbv2.DescribeListenersOutput, lastPage bool) bool {
			for _, listener := range p.Listeners {
				listeners = append(listeners, listener)
			}
			return true
		})
	if err != nil {
		return nil, err
	}

	return listeners, nil
}

// DescribeTagsForArn looks up all tags for a given ARN.
func (e *ELBV2) DescribeTagsForArn(arn *string) (util.Tags, error) {
	describeTags, err := e.DescribeTags(&elbv2.DescribeTagsInput{
		ResourceArns: []*string{arn},
	})

	var tags []*elbv2.Tag
	for _, tag := range describeTags.TagDescriptions[0].Tags {
		tags = append(tags, &elbv2.Tag{Key: tag.Key, Value: tag.Value})
	}

	return tags, err
}

// DescribeTargetGroupTargetsForArn looks up target group targets by an ARN.
func (e *ELBV2) DescribeTargetGroupTargetsForArn(arn *string) (util.AWSStringSlice, error) {
	var targets util.AWSStringSlice
	targetGroupHealth, err := e.DescribeTargetHealth(&elbv2.DescribeTargetHealthInput{
		TargetGroupArn: arn,
	})
	if err != nil {
		return nil, err
	}
	for _, targetHealthDescription := range targetGroupHealth.TargetHealthDescriptions {
		targets = append(targets, targetHealthDescription.Target.Id)
	}
	sort.Sort(targets)
	return targets, err
}

// UpdateTags compares the new (desired) tags against the old (current) tags. It then adds and
// removes tags as needed.
func (e *ELBV2) UpdateTags(arn *string, old util.Tags, new util.Tags) error {
	// List of tags that will be removed, if any.
	removeTags := []*string{}

	// Loop over all old (current) tags and for each tag no longer found in the new list, add it to
	// the removeTags list for deletion.
	for _, t := range old {
		found := false
		for _, nt := range new {
			if *nt.Key == *t.Key {
				found = true
				break
			}
		}
		if found == false {
			removeTags = append(removeTags, t.Key)
		}
	}

	// Adds all tags found in the new list. Tags pre-existing will be updated, tags not already
	// existent will be added, and tags where the value has not changed will remain unchanged.
	addParams := &elbv2.AddTagsInput{
		ResourceArns: []*string{arn},
		Tags:         new,
	}
	if _, err := e.AddTags(addParams); err != nil {
		return err
	}

	// When 1 or more tags were found to remove, remove them from the resource.
	if len(removeTags) > 0 {
		removeParams := &elbv2.RemoveTagsInput{
			ResourceArns: []*string{arn},
			TagKeys:      removeTags,
		}

		if _, err := e.RemoveTags(removeParams); err != nil {
			return err
		}
	}

	return nil
}
