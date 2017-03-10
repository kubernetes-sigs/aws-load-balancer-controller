package controller

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type LoadBalancer struct {
	id           string
	namespace    string
	hostname     string
	vpcID        string
	LoadBalancer *elbv2.LoadBalancer // current version of load balancer in AWS
	TargetGroups []*TargetGroup
	Listeners    []*Listener
}

// creates the load balancer
// albIngress is only passed along for logging
func (lb *LoadBalancer) create(a *albIngress) error {
	createLoadBalancerInput := &elbv2.CreateLoadBalancerInput{
		Name:           aws.String(lb.id),
		Subnets:        a.annotations.subnets,
		Scheme:         a.annotations.scheme,
		Tags:           a.Tags(),
		SecurityGroups: a.annotations.securityGroups,
	}

	// // Debug logger to introspect CreateLoadBalancer request
	glog.Infof("%s: Create load balancer %s request sent:\n%s", a.Name(), lb.id, createLoadBalancerInput)
	if noop {
		lb.LoadBalancer = &elbv2.LoadBalancer{
			LoadBalancerArn:       aws.String("mock/arn"),
			DNSName:               aws.String(lb.hostname),
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
	return nil
}

// Modifies the attributes of an existing ALB.
// albIngress is only passed along for logging
func (lb *LoadBalancer) modify(a *albIngress) error {
	needsModify, canModify := lb.checkModify(a)

	if !needsModify {
		return nil
	}

	glog.Infof("%s: Modifying existing load balancer %s", a.Name(), lb.id)

	if canModify {
		glog.Infof("%s: Modifying load balancer %s", a.Name(), lb.id)
		glog.Infof("%s: NOT IMPLEMENTED!!!!", a.Name())
		// TODO: Add LB modification stuff
		return nil
	}

	glog.Infof("%s: Must delete %s load balancer and recreate", a.Name(), lb.id)
	glog.Infof("%s: NOT IMPLEMENTED!!!!", a.Name())

	return nil
}

// Deletes the load balancer
func (lb *LoadBalancer) delete(a *albIngress) error {
	glog.Infof("%a: Deleting load balancer %v", a.Name(), lb.id)

	glog.Infof("%a: Delete %s load balancer", a.Name(), *lb.LoadBalancer.LoadBalancerName)
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

func (lb *LoadBalancer) loadBalancerExists(a *albIngress) (bool, *elbv2.LoadBalancer, error) {
	// glog.Infof("%s: Check if %s exists", a.Name(), lb.id)
	params := &elbv2.DescribeLoadBalancersInput{
		Names: []*string{aws.String(lb.id)},
	}
	resp, err := elbv2svc.svc.DescribeLoadBalancers(params)
	if err != nil && err.(awserr.Error).Code() != "LoadBalancerNotFound" {
		return false, nil, err
	}
	if len(resp.LoadBalancers) > 0 {
		return true, resp.LoadBalancers[0], nil
	}
	// ALB does *not* exist
	return false, nil, nil
}

func LoadBalancerID(clustername, namespace, ingressname, hostname string) string {
	hasher := md5.New()
	hasher.Write([]byte(namespace + ingressname + hostname))
	output := hex.EncodeToString(hasher.Sum(nil))
	// limit to 15 chars
	if len(output) > 15 {
		output = output[:15]
	}
	return fmt.Sprintf("%s-%s", clustername, output)
}

// checkModify returns if a LB needs to be modified and if it can be modified in place
// first parameter is true if the LB needs to be changed
// second parameter true if it can be changed in place
// TODO add more checks
func (lb *LoadBalancer) checkModify(a *albIngress) (bool, bool) {
	switch {
	case *lb.LoadBalancer.Scheme != *a.annotations.scheme:
		return true, false
	default:
		return false, false
	}
}
