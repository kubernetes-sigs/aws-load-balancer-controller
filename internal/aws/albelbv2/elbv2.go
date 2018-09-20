package albelbv2

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"

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
	ClusterLoadBalancers() ([]*elbv2.LoadBalancer, error)
	ClusterTargetGroups() (map[string][]*elbv2.TargetGroup, error)
	UpdateTags(arn *string, old util.ELBv2Tags, new util.ELBv2Tags) error
	RemoveTargetGroup(arn *string) error
	DescribeTargetGroupTargetsForArn(arn *string) (TargetDescriptions, error)
	RemoveListener(arn *string) error
	DescribeListenersForLoadBalancer(loadBalancerArn *string) ([]*elbv2.Listener, error)
	Status() func() error
	DescribeLoadBalancerAttributesFiltered(*string) (LoadBalancerAttributes, error)
	DescribeTargetGroupAttributesFiltered(*string) (TargetGroupAttributes, error)
	SetField(string, interface{})

	GetLoadBalancerByArn(string) (*elbv2.LoadBalancer, error)
}

type LoadBalancerAttributes []*elbv2.LoadBalancerAttribute

func (a LoadBalancerAttributes) Sorted() LoadBalancerAttributes {
	sort.Slice(a, func(i, j int) bool {
		return *a[i].Key < *a[j].Key
	})
	return a
}

func (a *LoadBalancerAttributes) Set(k, v string) {
	t := *a
	for i := range t {
		if *t[i].Key == k {
			t[i].Value = aws.String(v)
			return
		}
	}

	*a = append(*a, &elbv2.LoadBalancerAttribute{Key: aws.String(k), Value: aws.String(v)})
}

// Filtered returns the attributes that have been changed from defaults
func (a *LoadBalancerAttributes) Filtered() LoadBalancerAttributes {
	var out LoadBalancerAttributes

	// Defaults from https://github.com/aws/aws-sdk-go/blob/b05c59e7c774a2958fe2ea6dd7ccfef338d493e1/service/elbv2/api.go#L6240-L6278
	for _, attr := range *a {
		switch *attr.Key {
		case "routing.http2.enabled":
			if *attr.Value != "true" {
				out = append(out, attr)
			}
		case "deletion_protection.enabled":
			if *attr.Value != "false" {
				out = append(out, attr)
			}
		case "access_logs.s3.bucket":
			if *attr.Value != "" {
				out = append(out, attr)
			}
		case "idle_timeout.timeout_seconds":
			if *attr.Value != "60" {
				out = append(out, attr)
			}
		case "access_logs.s3.prefix":
			if *attr.Value != "" {
				out = append(out, attr)
			}
		case "access_logs.s3.enabled":
			if *attr.Value != "false" {
				out = append(out, attr)
			}
		}
	}
	return out
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

// Filtered returns the attributes that have been changed from defaults
func (a *TargetGroupAttributes) Filtered() TargetGroupAttributes {
	var out TargetGroupAttributes

	// Defaults from https://github.com/aws/aws-sdk-go/blob/b05c59e7c774a2958fe2ea6dd7ccfef338d493e1/service/elbv2/api.go#L8027-L8068
	for _, attr := range *a {
		switch *attr.Key {
		case "deregistration_delay.timeout_seconds":
			if *attr.Value != "300" {
				out = append(out, attr)
			}
		case "slow_start.duration_seconds":
			if *attr.Value != "0" {
				out = append(out, attr)
			}
		case "stickiness.enabled":
			if *attr.Value != "false" {
				out = append(out, attr)
			}
		case "stickiness.type":
			if *attr.Value != "lb_cookie" {
				out = append(out, attr)
			}
		case "stickiness.lb_cookie.duration_seconds":
			if *attr.Value != "86400" {
				out = append(out, attr)
			}
		}
	}
	return out
}

type TargetDescriptions []*elbv2.TargetDescription

func idPort(t *elbv2.TargetDescription) string {
	k := *t.Id
	if t.Port != nil {
		k = k + fmt.Sprintf(":%v", *t.Port)
	}
	return k
}

func (a TargetDescriptions) InstanceIds(r resolver.Resolver) []*string {
	var out []*string
	for _, x := range a {
		if strings.HasPrefix(*x.Id, "i-") {
			out = append(out, x.Id)
		} else {
			if instanceID, err := r.GetInstanceIDFromPodIP(*x.Id); err == nil {
				out = append(out, aws.String(instanceID))
			} else {
				glog.Errorf("Unable to locate a node for pod ip %v.", *x.Id)
			}
		}
	}
	return out
}

func (a TargetDescriptions) Sorted() TargetDescriptions {
	sort.Slice(a, func(i, j int) bool {
		return idPort(a[i]) < idPort(a[j])
	})
	return a
}

func (a TargetDescriptions) Difference(b TargetDescriptions) (ab TargetDescriptions) {
	mb := map[string]bool{}
	for _, x := range b {
		mb[idPort(x)] = true
	}
	for _, x := range a {
		if _, ok := mb[idPort(x)]; !ok {
			ab = append(ab, x)
		}
	}
	return
}

func (a TargetDescriptions) String() string {
	var s []string
	for i := range a {
		var n []string
		if a[i].AvailabilityZone != nil {
			n = append(n, *a[i].AvailabilityZone)
		}
		n = append(n, *a[i].Id)
		if a[i].Port != nil {
			n = append(n, fmt.Sprintf("%v", *a[i].Port))
		}
		s = append(s, strings.Join(n, ":"))
	}
	return strings.Join(s, ", ")
}

// Hash returns a hash representing security group names
func (a TargetDescriptions) Hash() string {
	sorted := a.Sorted()
	hasher := md5.New()
	for _, x := range sorted {
		hasher.Write([]byte(idPort(x)))
	}
	output := hex.EncodeToString(hasher.Sum(nil))
	return output
}

func (a TargetDescriptions) PopulateAZ() error {
	vpcID, err := albec2.EC2svc.GetVPCID()
	if err != nil {
		return err
	}

	vpc, err := albec2.EC2svc.GetVPC(vpcID)
	if err != nil {
		return err
	}

	// Parse all CIDR blocks associated with the VPC
	var ipv4Nets []*net.IPNet
	for _, cblock := range vpc.CidrBlockAssociationSet {
		_, parsed, err := net.ParseCIDR(*cblock.CidrBlock)
		if err != nil {
			return err
		}
		ipv4Nets = append(ipv4Nets, parsed)
	}

	// Check if endpoints are in any of the blocks. If not the IP is outside the VPC
	for i := range a {
		found := false
		aNet := net.ParseIP(*a[i].Id)
		for _, ipv4Net := range ipv4Nets {
			if ipv4Net.Contains(aNet) {
				found = true
				break
			}
		}
		if !found {
			a[i].AvailabilityZone = aws.String("all")
		}
	}
	return nil
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
			return nil
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

	return fmt.Errorf("Timed out trying to delete target group %s", *arn)
}

// ClusterLoadBalancers looks up all ELBV2 (ALB) instances in AWS that are part of the cluster.
func (e *ELBV2) ClusterLoadBalancers() ([]*elbv2.LoadBalancer, error) {
	var loadbalancers []*elbv2.LoadBalancer

	// BUG?: Does not filter based on ingress-class, should it?
	rgt, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	err = e.DescribeLoadBalancersPages(&elbv2.DescribeLoadBalancersInput{}, func(page *elbv2.DescribeLoadBalancersOutput, _ bool) bool {
		for _, loadBalancer := range page.LoadBalancers {
			if _, ok := rgt.LoadBalancers[*loadBalancer.LoadBalancerArn]; ok {
				loadbalancers = append(loadbalancers, loadBalancer)
			}
		}
		return true
	})

	return loadbalancers, err
}

// ClusterTargetGroups fetches all target groups that are part of the cluster.
func (e *ELBV2) ClusterTargetGroups() (map[string][]*elbv2.TargetGroup, error) {
	output := make(map[string][]*elbv2.TargetGroup)

	rgt, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	err = e.DescribeTargetGroupsPages(&elbv2.DescribeTargetGroupsInput{}, func(page *elbv2.DescribeTargetGroupsOutput, _ bool) bool {
		for _, targetGroup := range page.TargetGroups {
			for _, lbarn := range targetGroup.LoadBalancerArns {
				if _, ok := rgt.LoadBalancers[*lbarn]; ok {
					output[*lbarn] = append(output[*lbarn], targetGroup)
				}
			}
		}
		return true
	})

	return output, err
}

// DescribeLoadBalancerAttributesFiltered returns the non-default load balancer attributes
func (e *ELBV2) DescribeLoadBalancerAttributesFiltered(loadBalancerArn *string) (LoadBalancerAttributes, error) {
	attrs, err := e.DescribeLoadBalancerAttributes(&elbv2.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: loadBalancerArn,
	})
	if err != nil {
		return nil, err
	}

	out := LoadBalancerAttributes(attrs.Attributes)
	return out.Filtered(), nil
}

// DescribeTargetGroupAttributesFiltered returns the non-default target group attributes
func (e *ELBV2) DescribeTargetGroupAttributesFiltered(tgArn *string) (TargetGroupAttributes, error) {
	attrs, err := e.DescribeTargetGroupAttributes(&elbv2.DescribeTargetGroupAttributesInput{
		TargetGroupArn: tgArn,
	})
	if err != nil {
		return nil, err
	}

	out := TargetGroupAttributes(attrs.Attributes)
	return out.Filtered(), nil
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

// DescribeTargetGroupTargetsForArn looks up target group targets by an ARN.
func (e *ELBV2) DescribeTargetGroupTargetsForArn(arn *string) (result TargetDescriptions, err error) {
	var targetHealth *elbv2.DescribeTargetHealthOutput
	opts := &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: arn,
	}

	targetHealth, err = e.DescribeTargetHealth(opts)
	if err != nil {
		return
	}
	for _, targetHealthDescription := range targetHealth.TargetHealthDescriptions {
		result = append(result, targetHealthDescription.Target)
	}
	result = result.Sorted()

	return
}

// UpdateTags compares the new (desired) tags against the old (current) tags. It then adds and
// removes tags as needed.
func (e *ELBV2) UpdateTags(arn *string, old util.ELBv2Tags, new util.ELBv2Tags) error {
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
			return fmt.Errorf("[elasticloadbalancer.DescribeLoadBalancers]: %v", err)
		}
		return nil
	}
}

func (e *ELBV2) SetField(field string, v interface{}) {
}

// GetLoadBalancerByArn retrives loadbalancer instance by arn
func (e *ELBV2) GetLoadBalancerByArn(arn string) (*elbv2.LoadBalancer, error) {
	loadBalancers, err := e.describeLoadBalancersHelper(&elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return nil, err
	}
	if len(loadBalancers) == 0 {
		return nil, nil
	}
	return loadBalancers[0], nil
}

// describeLoadBalancersHelper is an helper to handle pagination in describeLoadBalancers call
func (e *ELBV2) describeLoadBalancersHelper(input *elbv2.DescribeLoadBalancersInput) (result []*elbv2.LoadBalancer, err error) {
	err = e.DescribeLoadBalancersPages(input, func(output *elbv2.DescribeLoadBalancersOutput, _ bool) bool {
		result = append(result, output.LoadBalancers...)
		return true
	})
	return result, err
}
