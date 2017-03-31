package controller

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type LoadBalancer struct {
	id                  *string
	hostname            *string
	CurrentLoadBalancer *elbv2.LoadBalancer // current version of load balancer in AWS
	DesiredLoadBalancer *elbv2.LoadBalancer // current version of load balancer in AWS
	ResourceRecordSet   *ResourceRecordSet
	TargetGroups        TargetGroups
	Listeners           Listeners
	CurrentTags         Tags
	DesiredTags         Tags
	Deleted             bool // flag representing the LoadBalancer instance was fully deleted.
}

type LoadBalancerChange uint

const (
	SecurityGroupsModified LoadBalancerChange = 1 << iota
	SubnetsModified
	TagsModified
)

func NewLoadBalancer(clustername, namespace, ingressname, hostname string, annotations *annotationsT, tags Tags) *LoadBalancer {
	hasher := md5.New()
	hasher.Write([]byte(namespace + ingressname + hostname))
	output := hex.EncodeToString(hasher.Sum(nil))

	if len(output) > 15 {
		output = output[:15]
	}

	name := fmt.Sprintf("%s-%s", clustername, output)

	tags = append(tags, &elbv2.Tag{
		Key:   aws.String("Hostname"),
		Value: aws.String(hostname),
	})

	vpcID, err := ec2svc.getVPCID(annotations.subnets)
	if err != nil {
		glog.Errorf("Error fetching VPC for subnets %v: %v", awsutil.Prettify(annotations.subnets), err)
		return nil
	}

	lb := &LoadBalancer{
		id:          aws.String(name),
		hostname:    aws.String(hostname),
		DesiredTags: tags,
		DesiredLoadBalancer: &elbv2.LoadBalancer{
			AvailabilityZones: annotations.subnets.AsAvailabilityZones(),
			LoadBalancerName:  aws.String(name),
			Scheme:            annotations.scheme,
			SecurityGroups:    annotations.securityGroups,
			VpcId:             vpcID,
		},
	}

	return lb
}

// SyncState compares the current and desired state of this LoadBalancer instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS ELBV2 (ALB) to
// satisfy the ingress's current state.
func (lb *LoadBalancer) SyncState() *LoadBalancer {
	switch {
	// No DesiredState means load balancer should be deleted.
	case lb.DesiredLoadBalancer == nil:
		if err := lb.delete(); err != nil {
			glog.Errorf("Error deleting load balancer %s: %s", *lb.id, err)
			return lb
		}

	// No CurrentState means load balancer doesn't exist in AWS and should be created.
	case lb.CurrentLoadBalancer == nil:
		if err := lb.create(); err != nil {
			glog.Errorf("Error creating load balancer %s: %s", *lb.id, err)
			return lb
		}

	// Current and Desired exist and need for modification should be evaluated.
	default:
		needsModification, _ := lb.needsModification()
		if needsModification == 0 {
			return lb
		}
		lb.modify()
	}

	return lb
}

// create requests a new ELBV2 (ALB) is created in AWS.
func (lb *LoadBalancer) create() error {
	createLoadBalancerInput := &elbv2.CreateLoadBalancerInput{
		Name:           lb.DesiredLoadBalancer.LoadBalancerName,
		Subnets:        AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets(),
		Scheme:         lb.DesiredLoadBalancer.Scheme,
		Tags:           lb.DesiredTags,
		SecurityGroups: lb.DesiredLoadBalancer.SecurityGroups,
	}

	createLoadBalancerOutput, err := elbv2svc.svc.CreateLoadBalancer(createLoadBalancerInput)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateLoadBalancer"}).Add(float64(1))
		return err
	}

	lb.CurrentLoadBalancer = createLoadBalancerOutput.LoadBalancers[0]
	return nil
}

// modify modifies the attributes of an existing ALB in AWS.
func (lb *LoadBalancer) modify() error {
	needsModification, canModify := lb.needsModification()

	glog.Infof("Modifying existing load balancer %s", *lb.id)

	if canModify {
		glog.Infof("Modifying load balancer %s", *lb.id)

		if needsModification&SecurityGroupsModified != 0 {
			params := &elbv2.SetSecurityGroupsInput{
				LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
				SecurityGroups:  lb.DesiredLoadBalancer.SecurityGroups,
			}
			_, err := elbv2svc.svc.SetSecurityGroups(params)
			if err != nil {
				return fmt.Errorf("Failure Setting ALB Security Groups: %s", err)
			}
		}

		if needsModification&SubnetsModified != 0 {
			params := &elbv2.SetSubnetsInput{
				LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
				Subnets:         AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets(),
			}
			_, err := elbv2svc.svc.SetSubnets(params)
			if err != nil {
				return fmt.Errorf("Failure Setting ALB Subnets: %s", err)
			}
		}

		if needsModification&TagsModified != 0 {
			glog.Infof("%s: Modifying %s tags", *lb.id)
			if err := elbv2svc.setTags(lb.CurrentLoadBalancer.LoadBalancerArn, lb.DesiredTags); err != nil {
				glog.Errorf("Error setting tags on %s: %s", *lb.id, err)
			}
			lb.CurrentTags = lb.DesiredTags
		}
		return nil
	}

	glog.Infof("Must delete %s load balancer and recreate", *lb.id)
	lb.delete()
	lb.create()

	return nil
}

// delete Deletes the load balancer from AWS.
func (lb *LoadBalancer) delete() error {
	glog.Infof("Deleting load balancer %v", *lb.id)

	deleteParams := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
	}

	_, err := elbv2svc.svc.DeleteLoadBalancer(deleteParams)
	if err != nil {
		return err
	}

	return nil
}

// needsModification returns if a LB needs to be modified and if it can be modified in place
// first parameter is true if the LB needs to be changed
// second parameter true if it can be changed in place
func (lb *LoadBalancer) needsModification() (LoadBalancerChange, bool) {
	var changes LoadBalancerChange

	// In the case that the LB does not exist yet
	if lb.CurrentLoadBalancer == nil {
		return changes, true
	}

	if *lb.CurrentLoadBalancer.Scheme != *lb.DesiredLoadBalancer.Scheme {
		return changes, false
	}

	currentSubnets := AvailabilityZones(lb.CurrentLoadBalancer.AvailabilityZones).AsSubnets()
	desiredSubnets := AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets()
	sort.Sort(currentSubnets)
	sort.Sort(desiredSubnets)
	if awsutil.Prettify(currentSubnets) != awsutil.Prettify(desiredSubnets) {
		changes |= SubnetsModified
	}

	currentSecurityGroups := AwsStringSlice(lb.CurrentLoadBalancer.SecurityGroups)
	desiredSecurityGroups := AwsStringSlice(lb.DesiredLoadBalancer.SecurityGroups)
	sort.Sort(currentSecurityGroups)
	sort.Sort(desiredSecurityGroups)
	if awsutil.Prettify(currentSecurityGroups) != awsutil.Prettify(desiredSecurityGroups) {
		changes |= SecurityGroupsModified
	}

	sort.Sort(lb.CurrentTags)
	sort.Sort(lb.DesiredTags)
	if awsutil.Prettify(lb.CurrentTags) != awsutil.Prettify(lb.DesiredTags) {
		changes |= TagsModified
	}

	return changes, true
}
