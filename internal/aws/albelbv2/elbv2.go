package albelbv2

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
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
	RemoveTargetGroup(arn *string) error
	RemoveListener(arn *string) error
	DescribeListenersForLoadBalancer(loadBalancerArn *string) ([]*elbv2.Listener, error)
	Status() func() error
	SetField(string, interface{})

	GetRules(string) ([]*elbv2.Rule, error)
	GetLoadBalancerByArn(string) (*elbv2.LoadBalancer, error)
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

func (e *ELBV2) GetRules(listenerArn string) ([]*elbv2.Rule, error) {
	var rules []*elbv2.Rule

	p := request.Pagination{
		EndPageOnSameToken: true,
		NewRequest: func() (*request.Request, error) {
			req, _ := e.DescribeRulesRequest(&elbv2.DescribeRulesInput{ListenerArn: aws.String(listenerArn)})
			return req, nil
		},
	}
	for p.Next() {
		page := p.Page().(*elbv2.DescribeRulesOutput)
		for _, rule := range page.Rules {
			rules = append(rules, rule)
		}
	}
	return rules, p.Err()
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
