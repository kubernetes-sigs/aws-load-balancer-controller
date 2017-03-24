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
}

type LoadBalancerChange uint

const (
	SecurityGroupsModified LoadBalancerChange = 1 << iota
	SubnetsModified
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

func (lb *LoadBalancer) SyncState() *LoadBalancer {
	if lb.DesiredLoadBalancer == nil {
		if err := lb.delete(); err != nil {
			glog.Errorf("Error deleting load balancer %s: %s", *lb.id, err)
			return lb
		}
		return nil
	} else if lb.CurrentLoadBalancer == nil {
		if err := lb.create(); err != nil {
			glog.Errorf("Error creating load balancer %s: %s", *lb.id, err)
			return lb
		}
	} else {
		needsModification, _ := lb.needsModification()
		if needsModification == 0 {
			return lb
		}
		lb.modify()
	}
	return lb
}

// creates the load balancer
// ALBIngress is only passed along for logging
func (lb *LoadBalancer) create() error {
	createLoadBalancerInput := &elbv2.CreateLoadBalancerInput{
		Name:           lb.DesiredLoadBalancer.LoadBalancerName,
		Subnets:        AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets(),
		Scheme:         lb.DesiredLoadBalancer.Scheme,
		Tags:           lb.DesiredTags,
		SecurityGroups: lb.DesiredLoadBalancer.SecurityGroups,
	}

	// // Debug logger to introspect CreateLoadBalancer request
	glog.Infof("Create load balancer %s", *lb.id)

	createLoadBalancerOutput, err := elbv2svc.svc.CreateLoadBalancer(createLoadBalancerInput)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateLoadBalancer"}).Add(float64(1))
		return err
	}

	lb.CurrentLoadBalancer = createLoadBalancerOutput.LoadBalancers[0]
	return nil
}

// Modifies the attributes of an existing ALB.
// ALBIngress is only passed along for logging
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

		return nil
	}

	glog.Infof("Must delete %s load balancer and recreate", *lb.id)
	lb.delete()
	lb.create()

	return nil
}

// Deletes the load balancer
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

	return changes, true
}
