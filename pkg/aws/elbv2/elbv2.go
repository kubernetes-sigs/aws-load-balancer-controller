package elbv2

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"

	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// ELBV2svc is a pointer to the awsutil ELBV2 service
var ELBV2svc ELBV2API

const (
	// Amount of time between each deletion attempt (or reattempt) for a target group
	deleteTargetGroupReattemptSleep int = 10
	// Maximum attempts should be made to delete a target group
	deleteTargetGroupReattemptMax int = 10
)

type ELBV2API interface {
	elbv2iface.ELBV2API
	ClusterLoadBalancers(clusterName *string) ([]*elbv2.LoadBalancer, error)
	SetIdleTimeout(arn *string, timeout int64) error
	UpdateTags(arn *string, old util.Tags, new util.Tags) error
	UpdateAttributes(arn *string, new []*elbv2.LoadBalancerAttribute) error
	RemoveTargetGroup(arn *string) error
	DescribeTagsForArn(arn *string) (util.Tags, error)
	DescribeTargetGroupTargetsForArn(arn *string, targets ...[]*elbv2.TargetDescription) (util.AWSStringSlice, error)
	RemoveListener(arn *string) error
	DescribeTargetGroupsForLoadBalancer(loadBalancerArn *string) ([]*elbv2.TargetGroup, error)
	DescribeListenersForLoadBalancer(loadBalancerArn *string) ([]*elbv2.Listener, error)
	Status() func() error
}

type LoadBalancerAttributes []*elbv2.LoadBalancerAttribute

func (a LoadBalancerAttributes) Sorted() LoadBalancerAttributes {
	sort.Slice(a, func(i, j int) bool {
		return *a[i].Key < *a[j].Key
	})
	return a
}

type TargetGroupAttributes []*elbv2.TargetGroupAttribute

func (a TargetGroupAttributes) Sorted() TargetGroupAttributes {
	sort.Slice(a, func(i, j int) bool {
		return *a[i].Key < *a[j].Key
	})
	return a
}

func (a *TargetGroupAttributes) Set(k, v string) {
	t := *a
	for i := range t {
		if *t[i].Key == k {
			t[i].Value = aws.String(v)
			return
		}
	}

	*a = append(*a, &elbv2.TargetGroupAttribute{Key: aws.String(k), Value: aws.String(v)})
}

// ELBV2 is our extension to AWS's elbv2.ELBV2
type ELBV2 struct {
	elbv2iface.ELBV2API
}

// NewELBV2 returns an ELBV2 based off of the provided AWS session
func NewELBV2(awsSession *session.Session) {
	ELBV2svc = &ELBV2{
		elbv2.New(awsSession),
	}
	return
}

// RemoveListener removes a Listener from an ELBV2 (ALB) by deleting it in AWS. If the deletion
// attempt returns a elbv2.ErrCodeListenerNotFoundException, it's considered a success as the
// listener has already been removed. If removal fails for another reason, an error is returned.
func (e *ELBV2) RemoveListener(arn *string) error {
	in := elbv2.DeleteListenerInput{
		ListenerArn: arn,
	}

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
func (e *ELBV2) RemoveTargetGroup(arn *string) error {
	in := &elbv2.DeleteTargetGroupInput{
		TargetGroupArn: arn,
	}
	for i := 0; i < deleteTargetGroupReattemptMax; i++ {
		_, err := e.DeleteTargetGroup(in)
		if err == nil {
			break
		}

		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elbv2.ErrCodeResourceInUseException:
				time.Sleep(time.Duration(deleteTargetGroupReattemptSleep) * time.Second)
			default:
				return aerr
			}
		} else {
			return aerr
		}
	}

	return nil
}

// ClusterLoadBalancers looks up all ELBV2 (ALB) instances in AWS that are part of the cluster.
func (e *ELBV2) ClusterLoadBalancers(clusterName *string) ([]*elbv2.LoadBalancer, error) {
	var loadbalancers []*elbv2.LoadBalancer

	err := e.DescribeLoadBalancersPagesWithContext(context.Background(),
		&elbv2.DescribeLoadBalancersInput{},
		func(p *elbv2.DescribeLoadBalancersOutput, lastPage bool) bool {
			for _, loadBalancer := range p.LoadBalancers {
				if strings.HasPrefix(*loadBalancer.LoadBalancerName, *clusterName+"-") {
					if s := strings.Split(*loadBalancer.LoadBalancerName, "-"); len(s) >= 2 {
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
	if len(describeTags.TagDescriptions) == 0 {
		return tags, err
	}

	for _, tag := range describeTags.TagDescriptions[0].Tags {
		tags = append(tags, &elbv2.Tag{Key: tag.Key, Value: tag.Value})
	}

	return tags, err
}

// DescribeTargetGroupTargetsForArn looks up target group targets by an ARN.
func (e *ELBV2) DescribeTargetGroupTargetsForArn(arn *string, targets ...[]*elbv2.TargetDescription) (result util.AWSStringSlice, err error) {
	var targetHealth *elbv2.DescribeTargetHealthOutput
	opts := &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: arn,
	}
	for _, target := range targets {
		opts.Targets = append(opts.Targets, target...)
	}
	targetHealth, err = e.DescribeTargetHealth(opts)
	if err != nil {
		return
	}
	for _, targetHealthDescription := range targetHealth.TargetHealthDescriptions {
		switch aws.StringValue(targetHealthDescription.TargetHealth.State) {
		case elbv2.TargetHealthStateEnumDraining:
			// We don't need to count this instance
		default:
			result = append(result, targetHealthDescription.Target.Id)
		}
	}
	sort.Sort(result)
	return
}

// SetIdleTimeout attempts to update an ELBV2's connection idle timeout setting. It must
// be passed a timeout in the range of 1-3600. If it fails to update, // an error will be returned.
func (e *ELBV2) SetIdleTimeout(arn *string, timeout int64) error {
	// aws only accepts a range of 1-3600 seconds
	if timeout < 1 || timeout > 3600 {
		return fmt.Errorf("Invalid set idle timeout provided. Must be within 1-3600 seconds. No modification will be attempted. Was: %d", timeout)
	}

	in := &elbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: arn,
		Attributes: []*elbv2.LoadBalancerAttribute{
			{
				Key:   aws.String(util.IdleTimeoutKey),
				Value: aws.String(strconv.Itoa(int(timeout)))},
		},
	}

	if _, err := e.ModifyLoadBalancerAttributes(in); err != nil {
		return fmt.Errorf("Failed to create ELBV2 (ALB): %s", err.Error())
	}

	return nil
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

// Status validates ELBV2 connectivity
func (e *ELBV2) Status() func() error {
	return func() error {
		in := &elbv2.DescribeLoadBalancersInput{}
		in.SetPageSize(1)

		if _, err := e.DescribeLoadBalancers(in); err != nil {
			return err
		}
		return nil
	}
}

// Update Attributes adds attributes to the loadbalancer.
func (e *ELBV2) UpdateAttributes(arn *string, attributes []*elbv2.LoadBalancerAttribute) error {
	newAttributes := &elbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: arn,
		Attributes:      attributes,
	}
	_, err := e.ModifyLoadBalancerAttributes(newAttributes)
	return err
}
