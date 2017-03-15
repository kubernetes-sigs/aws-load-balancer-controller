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
	id           *string
	namespace    *string
	hostname     *string
	vpcID        *string
	LoadBalancer *elbv2.LoadBalancer // current version of load balancer in AWS
	TargetGroups TargetGroups
	Listeners    Listeners
	Tags         []*elbv2.Tag
}

type LoadBalancerChange uint

const (
	SecurityGroups LoadBalancerChange = 1 << iota
	Subnets
)

// creates the load balancer
// albIngress is only passed along for logging
func (lb *LoadBalancer) create(a *albIngress) error {
	tags := a.Tags()

	createLoadBalancerInput := &elbv2.CreateLoadBalancerInput{
		Name:           lb.id,
		Subnets:        a.annotations.subnets,
		Scheme:         a.annotations.scheme,
		Tags:           tags,
		SecurityGroups: a.annotations.securityGroups,
	}

	// // Debug logger to introspect CreateLoadBalancer request
	glog.Infof("%s: Create load balancer %s", a.Name(), *lb.id)
	if noop {
		lb.LoadBalancer = &elbv2.LoadBalancer{
			LoadBalancerArn:       aws.String("mock/arn"),
			DNSName:               lb.hostname,
			Scheme:                createLoadBalancerInput.Scheme,
			CanonicalHostedZoneId: aws.String("loadbalancerzoneid"),
		}
		return nil
	}

	createLoadBalancerOutput, err := elbv2svc.svc.CreateLoadBalancer(createLoadBalancerInput)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateLoadBalancer"}).Add(float64(1))
		return err
	}

	lb.LoadBalancer = createLoadBalancerOutput.LoadBalancers[0]
	lb.Tags = tags
	return nil
}

// Modifies the attributes of an existing ALB.
// albIngress is only passed along for logging
func (lb *LoadBalancer) modify(a *albIngress) error {
	needsModification, canModify := lb.needsModification(a)

	if needsModification == 0 {
		return nil
	}

	glog.Infof("%s: Modifying existing load balancer %s", a.Name(), *lb.id)

	if canModify {
		glog.Infof("%s: Modifying load balancer %s", a.Name(), *lb.id)

		if needsModification&SecurityGroups != 0 {
			params := &elbv2.SetSecurityGroupsInput{
				LoadBalancerArn: lb.LoadBalancer.LoadBalancerArn,
				SecurityGroups:  a.annotations.securityGroups,
			}
			_, err := elbv2svc.svc.SetSecurityGroups(params)
			if err != nil {
				return fmt.Errorf("Failure Setting ALB Security Groups: %s", err)
			}
		}

		if needsModification&Subnets != 0 {
			params := &elbv2.SetSubnetsInput{
				LoadBalancerArn: lb.LoadBalancer.LoadBalancerArn,
				Subnets:         a.annotations.subnets,
			}
			_, err := elbv2svc.svc.SetSubnets(params)
			if err != nil {
				return fmt.Errorf("Failure Setting ALB Subnets: %s", err)
			}
		}

		return nil
	}

	glog.Infof("%s: Must delete %s load balancer and recreate", a.Name(), *lb.id)
	lb.delete(a)
	lb.create(a)
	// TODO: Update TargetGroups & rules

	return nil
}

// Deletes the load balancer
func (lb *LoadBalancer) delete(a *albIngress) error {
	glog.Infof("%s: Deleting load balancer %v", a.Name(), *lb.id)
	if noop {
		return nil
	}

	deleteParams := &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: lb.LoadBalancer.LoadBalancerArn,
	}

	_, err := elbv2svc.svc.DeleteLoadBalancer(deleteParams)
	if err != nil {
		return err
	}

	return nil
}

func LoadBalancerID(clustername, namespace, ingressname, hostname string) *string {
	hasher := md5.New()
	hasher.Write([]byte(namespace + ingressname + hostname))
	output := hex.EncodeToString(hasher.Sum(nil))
	// limit to 15 chars
	if len(output) > 15 {
		output = output[:15]
	}

	name := fmt.Sprintf("%s-%s", clustername, output)
	return aws.String(name)
}

// needsModification returns if a LB needs to be modified and if it can be modified in place
// first parameter is true if the LB needs to be changed
// second parameter true if it can be changed in place
// TODO test tags
func (lb *LoadBalancer) needsModification(a *albIngress) (LoadBalancerChange, bool) {
	var (
		changes LoadBalancerChange
	)

	if lb.LoadBalancer == nil {
		return changes, true
	}

	subnets := lb.subnets()
	sort.Sort(subnets)

	securityGroups := AwsStringSlice(lb.LoadBalancer.SecurityGroups)
	sort.Sort(securityGroups)

	switch {
	case *lb.LoadBalancer.Scheme != *a.annotations.scheme:
		return changes, false
	case awsutil.Prettify(securityGroups) != awsutil.Prettify(a.annotations.securityGroups):
		changes |= SecurityGroups
	case awsutil.Prettify(subnets) != awsutil.Prettify(a.annotations.subnets):
		changes |= Subnets
	}
	return changes, true
}

func (lb *LoadBalancer) subnets() AwsStringSlice {
	var out AwsStringSlice

	for _, az := range lb.LoadBalancer.AvailabilityZones {
		out = append(out, az.SubnetId)
	}
	return out
}
