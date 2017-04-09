package alb

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/awsutil"
	"github.com/coreos/alb-ingress-controller/controller/config"
	"github.com/coreos/alb-ingress-controller/controller/util"
	"github.com/coreos/alb-ingress-controller/log"
	"github.com/prometheus/client_golang/prometheus"
)

// LoadBalancer contains the overarching configuration for the ALB
type LoadBalancer struct {
	ID                  *string
	IngressID           *string // Same Id as ingress object this comes from.
	Hostname            *string
	CurrentLoadBalancer *elbv2.LoadBalancer // current version of load balancer in AWS
	DesiredLoadBalancer *elbv2.LoadBalancer // current version of load balancer in AWS
	ResourceRecordSet   *ResourceRecordSet
	TargetGroups        TargetGroups
	Listeners           Listeners
	CurrentTags         util.Tags
	DesiredTags         util.Tags
	Deleted             bool // flag representing the LoadBalancer instance was fully deleted.
	LastRulePriority    int64
}

type loadBalancerChange uint

const (
	securityGroupsModified loadBalancerChange = 1 << iota
	subnetsModified
	tagsModified
	schemeModified
)

// NewLoadBalancer returns a new alb.LoadBalancer based on the parameters provided.
func NewLoadBalancer(clustername, namespace, ingressname, hostname string, ingressID *string, annotations *config.AnnotationsT, tags util.Tags) *LoadBalancer {
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

	vpcID, err := awsutil.Ec2svc.GetVPCID(annotations.Subnets)
	if err != nil {
		log.Errorf("Failed to fetch VPC subnets. Subnets: %v | Error: %v",
			ingressname, awsutil.Prettify(annotations.Subnets), err.Error())
		return nil
	}

	lb := &LoadBalancer{
		ID:          aws.String(name),
		IngressID:   ingressID,
		Hostname:    aws.String(hostname),
		DesiredTags: tags,
		DesiredLoadBalancer: &elbv2.LoadBalancer{
			AvailabilityZones: annotations.Subnets.AsAvailabilityZones(),
			LoadBalancerName:  aws.String(name),
			Scheme:            annotations.Scheme,
			SecurityGroups:    annotations.SecurityGroups,
			VpcId:             vpcID,
		},
		LastRulePriority: 1,
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
		log.Infof("Start ELBV2 (ALB) deletion.", *lb.IngressID)
		lb.delete()

	// No CurrentState means load balancer doesn't exist in AWS and should be created.
	case lb.CurrentLoadBalancer == nil:
		log.Infof("Start ELBV2 (ALB) creation.", *lb.IngressID)
		lb.create()

	// Current and Desired exist and need for modification should be evaluated.
	default:
		needsModification, _ := lb.needsModification()
		if needsModification == 0 {
			log.Debugf("No modification of ELBV2 (ALB) required.", *lb.IngressID)
			return lb
		}

		log.Infof("Start ELBV2 (ALB) modification.", *lb.IngressID)
		lb.modify()
	}

	return lb
}

// create requests a new ELBV2 (ALB) is created in AWS.
func (lb *LoadBalancer) create() error {
	createLoadBalancerInput := &elbv2.CreateLoadBalancerInput{
		Name:           lb.DesiredLoadBalancer.LoadBalancerName,
		Subnets:        util.AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets(),
		Scheme:         lb.DesiredLoadBalancer.Scheme,
		Tags:           lb.DesiredTags,
		SecurityGroups: lb.DesiredLoadBalancer.SecurityGroups,
	}

	createLoadBalancerOutput, err := awsutil.Elbv2svc.Svc.CreateLoadBalancer(createLoadBalancerInput)
	if err != nil {
		awsutil.AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateLoadBalancer"}).Add(float64(1))
		log.Errorf("Failed to create ELBV2 (ALB). Error: %s", err.Error())
		return err
	}

	lb.CurrentLoadBalancer = createLoadBalancerOutput.LoadBalancers[0]
	log.Infof("Completed ELBV2 (ALB) creation. Name: %s | ARN: %s",
		*lb.IngressID, *lb.CurrentLoadBalancer.LoadBalancerName, *lb.CurrentLoadBalancer.LoadBalancerArn)
	return nil
}

// modify modifies the attributes of an existing ALB in AWS.
func (lb *LoadBalancer) modify() error {
	needsModification, canModify := lb.needsModification()

	if canModify {

		// Modify Security Groups
		if needsModification&securityGroupsModified != 0 {
			log.Infof("Start ELBV2 security groups modification.", *lb.IngressID)
			params := &elbv2.SetSecurityGroupsInput{
				LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
				SecurityGroups:  lb.DesiredLoadBalancer.SecurityGroups,
			}
			_, err := awsutil.Elbv2svc.Svc.SetSecurityGroups(params)
			if err != nil {
				log.Errorf("Failed ELBV2 security groups modification. Error: %s", err.Error())
				return err
			}
			lb.CurrentLoadBalancer.SecurityGroups = lb.DesiredLoadBalancer.SecurityGroups
			log.Infof("Completed ELBV2 security groups modification. SGs: %s",
				*lb.IngressID, log.Prettify(lb.CurrentLoadBalancer.SecurityGroups))
		}

		// Modify Subnets
		if needsModification&subnetsModified != 0 {
			log.Infof("Start subnets modification.", *lb.IngressID)
			params := &elbv2.SetSubnetsInput{
				LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
				Subnets:         util.AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets(),
			}
			setSubnetsOutput, err := awsutil.Elbv2svc.Svc.SetSubnets(params)
			if err != nil {
				return fmt.Errorf("Failure Setting ALB Subnets: %s", err)
			}
			log.Infof("Completed subnets modification. Subnets are %s.", *lb.IngressID, log.Prettify(setSubnetsOutput.AvailabilityZones))
		}

		// Modify Tags
		if needsModification&tagsModified != 0 {
			log.Infof("Start ELBV2 tag modification.", *lb.IngressID)
			if err := awsutil.Elbv2svc.SetTags(lb.CurrentLoadBalancer.LoadBalancerArn, lb.CurrentTags, lb.DesiredTags); err != nil {
				log.Errorf("Failed ELBV2 (ALB) tag modification. Error: %s", err.Error())
			}
			lb.CurrentTags = lb.DesiredTags
			log.Infof("Completed ELBV2 tag modification. Tags are %s.", *lb.IngressID, log.Prettify(lb.CurrentTags))
		}
		return nil
	}

	log.Infof("Start ELBV2 full modification (delete and create).", *lb.IngressID)
	lb.delete()
	// Since listeners and rules are deleted during lb deletion, ensure their current state is removed
	// as they'll no longer exist.
	lb.Listeners.StripCurrentState()
	lb.create()
	log.Infof("Completed ELBV2 full modification (delete and create). Name: %s | ARN: %s",
		*lb.IngressID, *lb.CurrentLoadBalancer.LoadBalancerName, *lb.CurrentLoadBalancer.LoadBalancerArn)

	return nil
}

// delete Deletes the load balancer from AWS.
func (lb *LoadBalancer) delete() error {

	deleteParams := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
	}

	_, err := awsutil.Elbv2svc.Svc.DeleteLoadBalancer(deleteParams)
	if err != nil {
		log.Errorf("Failed deletion of ELBV2 (ALB). Error: %s.", *lb.IngressID, err.Error())
		return err
	}

	log.Infof("Completed ELBV2 (ALB) deletion. Name: %s | ARN: %s",
		*lb.IngressID, *lb.CurrentLoadBalancer.LoadBalancerName, *lb.CurrentLoadBalancer.LoadBalancerArn)
	return nil
}

// needsModification returns if a LB needs to be modified and if it can be modified in place
// first parameter is true if the LB needs to be changed
// second parameter true if it can be changed in place
func (lb *LoadBalancer) needsModification() (loadBalancerChange, bool) {
	var changes loadBalancerChange

	// In the case that the LB does not exist yet
	if lb.CurrentLoadBalancer == nil {
		return changes, true
	}

	if *lb.CurrentLoadBalancer.Scheme != *lb.DesiredLoadBalancer.Scheme {
		changes |= schemeModified
		return changes, false
	}

	currentSubnets := util.AvailabilityZones(lb.CurrentLoadBalancer.AvailabilityZones).AsSubnets()
	desiredSubnets := util.AvailabilityZones(lb.DesiredLoadBalancer.AvailabilityZones).AsSubnets()
	sort.Sort(currentSubnets)
	sort.Sort(desiredSubnets)
	if awsutil.Prettify(currentSubnets) != awsutil.Prettify(desiredSubnets) {
		changes |= subnetsModified
	}

	currentSecurityGroups := util.AWSStringSlice(lb.CurrentLoadBalancer.SecurityGroups)
	desiredSecurityGroups := util.AWSStringSlice(lb.DesiredLoadBalancer.SecurityGroups)
	sort.Sort(currentSecurityGroups)
	sort.Sort(desiredSecurityGroups)
	if awsutil.Prettify(currentSecurityGroups) != awsutil.Prettify(desiredSecurityGroups) {
		changes |= securityGroupsModified
	}

	sort.Sort(lb.CurrentTags)
	sort.Sort(lb.DesiredTags)
	if awsutil.Prettify(lb.CurrentTags) != awsutil.Prettify(lb.DesiredTags) {
		changes |= tagsModified
	}

	return changes, true
}
