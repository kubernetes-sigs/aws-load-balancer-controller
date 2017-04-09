package awsutil

import (
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/coreos/alb-ingress-controller/controller/util"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// ELBV2 is our extension to AWS's elbv2.ELBV2
type ELBV2 struct {
	Svc elbv2iface.ELBV2API
}

func NewELBV2(awsconfig *aws.Config) *ELBV2 {
	awsSession, err := session.NewSession(awsconfig)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "NewSession"}).Add(float64(1))
		glog.Errorf("Failed to create AWS session. Error: %s.", err.Error())
		return nil
	}

	awsSession.Handlers.Send.PushFront(func(r *request.Request) {
		AWSRequest.With(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name}).Add(float64(1))
		if AWSDebug {
			glog.Infof("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation, r.Params)
		}
	})

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
	newLb := o.LoadBalancers[0]
	return newLb, nil
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
	newL := o.Listeners[0]
	return newL, nil
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

// RemoveListener removes a Listener from an ELBV2 (ALB) in AWS. If the deletion attempt returns a
// elbv2.ErrCodeListenerNotFoundException, it's considered a success as the listener has already
// been removed. If removal fails for another reason, an error is returned.
func (e *ELBV2) RemoveListener(in elbv2.DeleteListenerInput) error {
	_, err := e.Svc.DeleteListener(&in)
	awsErr := err.(awserr.Error)
	if err != nil && awsErr.Code() != elbv2.ErrCodeListenerNotFoundException {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "DeleteListener"}).Add(float64(1))
		return err
	}
	return nil
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

// SetSubnets updates the subnets attached to an ELBV2 (ALB). It returns an error when unsuccesful.
func (e *ELBV2) SetSubnets(in elbv2.SetSubnetsInput) error {
	_, err := e.Svc.SetSubnets(&in)
	if err != nil {
		AWSErrorCount.With(
			prometheus.Labels{"service": "ELBV2", "request": "SetSubnets"}).Add(float64(1))
		return err
	}
	return nil
}

// DescribeLoadBalancers looks up all ELBV2 (ALB) instances in AWS that are part of the cluster.
func (elb *ELBV2) DescribeLoadBalancers(clusterName *string) ([]*elbv2.LoadBalancer, error) {
	var loadbalancers []*elbv2.LoadBalancer
	describeLoadBalancersInput := &elbv2.DescribeLoadBalancersInput{
		PageSize: aws.Int64(100),
	}

	for {
		describeLoadBalancersOutput, err := elb.Svc.DescribeLoadBalancers(describeLoadBalancersInput)
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

func (elb *ELBV2) DescribeTargetGroups(loadBalancerArn *string) ([]*elbv2.TargetGroup, error) {
	var targetGroups []*elbv2.TargetGroup
	describeTargetGroupsInput := &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: loadBalancerArn,
		PageSize:        aws.Int64(100),
	}

	for {
		describeTargetGroupsOutput, err := elb.Svc.DescribeTargetGroups(describeTargetGroupsInput)
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

func (elb *ELBV2) DescribeListeners(loadBalancerArn *string) ([]*elbv2.Listener, error) {
	var listeners []*elbv2.Listener
	describeListenersInput := &elbv2.DescribeListenersInput{
		LoadBalancerArn: loadBalancerArn,
		PageSize:        aws.Int64(100),
	}

	for {
		describeListenersOutput, err := elb.Svc.DescribeListeners(describeListenersInput)
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

func (elb *ELBV2) DescribeTags(arn *string) (util.Tags, error) {
	describeTags, err := elb.Svc.DescribeTags(&elbv2.DescribeTagsInput{
		ResourceArns: []*string{arn},
	})

	var tags []*elbv2.Tag
	for _, tag := range describeTags.TagDescriptions[0].Tags {
		tags = append(tags, &elbv2.Tag{Key: tag.Key, Value: tag.Value})
	}

	return tags, err
}

func (elb *ELBV2) DescribeTargetGroup(arn *string) (*elbv2.TargetGroup, error) {
	targetGroups, err := ALBsvc.Svc.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
		TargetGroupArns: []*string{arn},
	})
	if err != nil {
		return nil, err
	}
	return targetGroups.TargetGroups[0], nil
}

func (elb *ELBV2) DescribeTargetGroupTargets(arn *string) (util.AWSStringSlice, error) {
	var targets util.AWSStringSlice
	targetGroupHealth, err := ALBsvc.Svc.DescribeTargetHealth(&elbv2.DescribeTargetHealthInput{
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

func (elb *ELBV2) DescribeRules(listenerArn *string) ([]*elbv2.Rule, error) {
	describeRulesInput := &elbv2.DescribeRulesInput{
		ListenerArn: listenerArn,
	}

	describeRulesOutput, err := elb.Svc.DescribeRules(describeRulesInput)
	if err != nil {
		return nil, err
	}

	return describeRulesOutput.Rules, nil
}

// Update tags compares the new (desired) tags against the old (current) tags. It then adds and
// removes tags as needed..
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
