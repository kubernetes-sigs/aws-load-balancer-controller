package awsutil

import (
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/coreos/alb-ingress-controller/controller/util"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Amount of time between each deletion attempt (or reattempt) for a target group
	deleteTargetGroupReattemptSleep int = 10
	// Maximum attempts should be made to delete a target group
	deleteTargetGroupReattemptMax int = 10
)

// ELBV2 is our extension to AWS's elbv2.ELBV2
type ELBV2 struct {
	Svc elbv2iface.ELBV2API
}

// NewELBV2 returns an ELBV2 based off of the provided AWS session
func NewELBV2(awsSession *session.Session) *ELBV2 {
	elbClient := ELBV2{
		elbv2.New(awsSession),
	}
	return &elbClient
}

// Create makes a new ELBV2 (ALB) in AWS. It returns the elbv2.LoadBalancer created on success or an
// error on failure.
func (e *ELBV2) Create(in elbv2.CreateLoadBalancerInput) (*elbv2.LoadBalancer, error) {
	o, err := e.Svc.CreateLoadBalancer(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "CreateLoadBalancer"}).Add(float64(1))
		return nil, err
	}

	return o.LoadBalancers[0], nil
}

// AddListener creates a new Listener and associates it with the ELBV2 (ALB). It returns the
// elbv2.Listener created on success or an error on failure.
func (e *ELBV2) AddListener(in elbv2.CreateListenerInput) (*elbv2.Listener, error) {
	o, err := e.Svc.CreateListener(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "CreateListener"}).Add(float64(1))
		return nil, err
	}

	return o.Listeners[0], nil
}

// AddRule creates a new Rule and associates it with the Listener. It returns the elbv2.Rule created
// on success or an error returned on failure.
func (e *ELBV2) AddRule(in elbv2.CreateRuleInput) (*elbv2.Rule, error) {
	o, err := e.Svc.CreateRule(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "CreateRule"}).Add(float64(1))
		return nil, err
	}

	return o.Rules[0], nil
}

// AddTargetGroup creates a new TargetGroup in AWS. It returns the created elbv2.TargetGroup on
// success and an error on failure.
func (e *ELBV2) AddTargetGroup(in elbv2.CreateTargetGroupInput) (*elbv2.TargetGroup, error) {
	o, err := e.Svc.CreateTargetGroup(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "CreateTargetGroup"}).Add(float64(1))
		return nil, err
	}

	return o.TargetGroups[0], nil
}

// Delete removes an ELBV2 (ALB) in AWS. It returns an error if the delete fails. Deletions of ALBs
// in AWS will also remove all listeners and rules associated with them.
func (e *ELBV2) Delete(in elbv2.DeleteLoadBalancerInput) error {
	_, err := e.Svc.DeleteLoadBalancer(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "DeleteLoadBalancer"}).Add(float64(1))
		return err
	}
	return nil
}

// RemoveListener removes a Listener from an ELBV2 (ALB) by deleting it in AWS. If the deletion
// attempt returns a elbv2.ErrCodeListenerNotFoundException, it's considered a success as the
// listener has already been removed. If removal fails for another reason, an error is returned.
func (e *ELBV2) RemoveListener(in elbv2.DeleteListenerInput) error {
	_, err := e.Svc.DeleteListener(&in)
	if err == nil {
		return nil
	}

	// If error is due to listener not existing; consider operation a success
	awsErr := err.(awserr.Error)
	if awsErr.Code() != elbv2.ErrCodeListenerNotFoundException {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "DeleteListener"}).Add(float64(1))
		return err
	}

	return nil
}

// RemoveRule removes a Rule from a listener by deleting it in AWS. If the deletion fails, an error
// is returned.
func (e *ELBV2) RemoveRule(in elbv2.DeleteRuleInput) error {
	_, err := e.Svc.DeleteRule(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "DeleteRule"}).Add(float64(1))
		return err
	}
	return nil
}

// RemoveTargetGroup removes a Target Group from AWS by deleting it. If the deletion fails, an error
// is returned. Often, a Listener that references the Target Group is still being deleted when this
// method is accessed. Thus, this method makes multiple attempts to delete the Target Group when it
// receives an elbv2.ErrCodeResourceInUseException.
func (e *ELBV2) RemoveTargetGroup(in elbv2.DeleteTargetGroupInput) error {
	for i := 0; i < deleteTargetGroupReattemptMax; i++ {
		_, err := e.Svc.DeleteTargetGroup(&in)
		switch {
		case err != nil && err.(awserr.Error).Code() == elbv2.ErrCodeResourceInUseException:
			AWSErrorCount.With(
				prometheus.Labels{"service": "ELBV2", "request": "DeleteTargetGroup"}).Add(float64(1))
			time.Sleep(time.Duration(deleteTargetGroupReattemptSleep) * time.Second)
			continue
		case err != nil:
			AWSErrorCount.With(
				prometheus.Labels{"service": "ELBV2", "request": "DeleteRule"}).Add(float64(1))
			return err
		}
	}

	return nil
}

// ModifyTargetGroup alters a Target Group in AWS. The modified elbv2.TargetGroup is returned on
// success and an error is returned on failure.
func (e *ELBV2) ModifyTargetGroup(in elbv2.ModifyTargetGroupInput) (*elbv2.TargetGroup, error) {
	o, err := e.Svc.ModifyTargetGroup(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "ModifyTargetGroup"}).Add(float64(1))
		return nil, err
	}

	return o.TargetGroups[0], nil
}

// SetSecurityGroups updates the security groups attached to an ELBV2 (ALB). It returns an error
// when unsuccessful.
func (e *ELBV2) SetSecurityGroups(in elbv2.SetSecurityGroupsInput) error {
	_, err := e.Svc.SetSecurityGroups(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "SetSecurityGroups"}).Add(float64(1))
		return err
	}
	return nil
}

// SetSubnets updates the subnets attached to an ELBV2 (ALB). It returns an error when unsuccessful.
func (e *ELBV2) SetSubnets(in elbv2.SetSubnetsInput) error {
	_, err := e.Svc.SetSubnets(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "SetSubnets"}).Add(float64(1))
		return err
	}
	return nil
}

// RegisterTargets adds EC2 instances to a Target Group. It returns an error when unsuccessful.
func (e *ELBV2) RegisterTargets(in elbv2.RegisterTargetsInput) error {
	_, err := e.Svc.RegisterTargets(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "RegisterTargets"}).Add(float64(1))
		return err
	}
	return nil
}

// DescribeLoadBalancers looks up all ELBV2 (ALB) instances in AWS that are part of the cluster.
func (e *ELBV2) DescribeLoadBalancers(clusterName *string) ([]*elbv2.LoadBalancer, error) {
	var loadbalancers []*elbv2.LoadBalancer
	describeLoadBalancersInput := &elbv2.DescribeLoadBalancersInput{
		PageSize: aws.Int64(100),
	}

	for {
		describeLoadBalancersOutput, err := e.Svc.DescribeLoadBalancers(describeLoadBalancersInput)
		if err != nil {
			return nil, err
		}

		describeLoadBalancersInput.Marker = describeLoadBalancersOutput.NextMarker

		for _, loadBalancer := range describeLoadBalancersOutput.LoadBalancers {
			if strings.HasPrefix(*loadBalancer.LoadBalancerName, *clusterName+"-") {
				if s := strings.Split(*loadBalancer.LoadBalancerName, "-"); len(s) == 2 {
					if s[0] == *clusterName {
						loadbalancers = append(loadbalancers, loadBalancer)
					}
				}
			}
		}

		if describeLoadBalancersOutput.NextMarker == nil {
			break
		}
	}
	return loadbalancers, nil
}

// DescribeTargetGroups looks up all ELBV2 (ALB) target groups in AWS that are part of the cluster.
func (e *ELBV2) DescribeTargetGroups(loadBalancerArn *string) ([]*elbv2.TargetGroup, error) {
	var targetGroups []*elbv2.TargetGroup
	describeTargetGroupsInput := &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: loadBalancerArn,
		PageSize:        aws.Int64(100),
	}

	for {
		describeTargetGroupsOutput, err := e.Svc.DescribeTargetGroups(describeTargetGroupsInput)
		if err != nil {
			return nil, err
		}

		describeTargetGroupsInput.Marker = describeTargetGroupsOutput.NextMarker

		for _, targetGroup := range describeTargetGroupsOutput.TargetGroups {
			targetGroups = append(targetGroups, targetGroup)
		}

		if describeTargetGroupsOutput.NextMarker == nil {
			break
		}
	}
	return targetGroups, nil
}

// DescribeListeners looks up all ELBV2 (ALB) listeners in AWS that are part of the cluster.
func (e *ELBV2) DescribeListeners(loadBalancerArn *string) ([]*elbv2.Listener, error) {
	var listeners []*elbv2.Listener
	describeListenersInput := &elbv2.DescribeListenersInput{
		LoadBalancerArn: loadBalancerArn,
		PageSize:        aws.Int64(100),
	}

	for {
		describeListenersOutput, err := e.Svc.DescribeListeners(describeListenersInput)
		if err != nil {
			AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DescribeListeners"}).Add(float64(1))
			return nil, err
		}

		describeListenersInput.Marker = describeListenersOutput.NextMarker

		for _, listener := range describeListenersOutput.Listeners {
			listeners = append(listeners, listener)
		}

		if describeListenersOutput.NextMarker == nil {
			break
		}
	}
	return listeners, nil
}

// DescribeTags looks up all tags for a given ARN.
func (e *ELBV2) DescribeTags(arn *string) (util.Tags, error) {
	describeTags, err := e.Svc.DescribeTags(&elbv2.DescribeTagsInput{
		ResourceArns: []*string{arn},
	})

	var tags []*elbv2.Tag
	for _, tag := range describeTags.TagDescriptions[0].Tags {
		tags = append(tags, &elbv2.Tag{Key: tag.Key, Value: tag.Value})
	}

	return tags, err
}

// DescribeTargetGroup looks up a target group by an ARN.
func (e *ELBV2) DescribeTargetGroup(arn *string) (*elbv2.TargetGroup, error) {
	targetGroups, err := e.Svc.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
		TargetGroupArns: []*string{arn},
	})
	if err != nil {
		return nil, err
	}
	return targetGroups.TargetGroups[0], nil
}

// DescribeTargetGroupTargets looks up target group targets by an ARN.
func (e *ELBV2) DescribeTargetGroupTargets(arn *string) (util.AWSStringSlice, error) {
	var targets util.AWSStringSlice
	targetGroupHealth, err := e.Svc.DescribeTargetHealth(&elbv2.DescribeTargetHealthInput{
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

// DescribeRules looks up all rules for a listener ARN.
func (e *ELBV2) DescribeRules(listenerArn *string) ([]*elbv2.Rule, error) {
	describeRulesInput := &elbv2.DescribeRulesInput{
		ListenerArn: listenerArn,
	}

	describeRulesOutput, err := e.Svc.DescribeRules(describeRulesInput)
	if err != nil {
		return nil, err
	}

	return describeRulesOutput.Rules, nil
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
	if _, err := e.Svc.AddTags(addParams); err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "AddTags"}).Add(float64(1))
		return err
	}

	// When 1 or more tags were found to remove, remove them from the resource.
	if len(removeTags) > 0 {
		removeParams := &elbv2.RemoveTagsInput{
			ResourceArns: []*string{arn},
			TagKeys:      removeTags,
		}

		if _, err := e.Svc.RemoveTags(removeParams); err != nil {
			AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "RemoveTags"}).Add(float64(1))
			return err
		}
	}

	return nil
}
