package controller

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type Listener struct {
	CurrentListener *elbv2.Listener
	DesiredListener *elbv2.Listener
}

type Listeners []*Listener

func NewListener(annotations *annotationsT) *Listener {
	listener := &elbv2.Listener{
		Port:     aws.Int64(80),
		Protocol: aws.String("HTTP"),
	}

	if annotations.certificateArn != nil {
		listener.Certificates = []*elbv2.Certificate{
			&elbv2.Certificate{
				CertificateArn: annotations.certificateArn,
			},
		}
		listener.Protocol = aws.String("HTTPS")
		listener.Port = aws.Int64(443)
	}

	if annotations.port != nil {
		listener.Port = annotations.port
	}

	return &Listener{DesiredListener: listener}
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (l *Listener) create(a *albIngress, lb *LoadBalancer, tg *TargetGroup) error {
	// Debug logger to introspect CreateListener request
	glog.Infof("%s: Create Listener for %s sent", a.Name(), *lb.CurrentLoadBalancer.DNSName)

	createListenerInput := &elbv2.CreateListenerInput{
		Certificates:    l.DesiredListener.Certificates,
		LoadBalancerArn: lb.CurrentLoadBalancer.LoadBalancerArn,
		Protocol:        l.DesiredListener.Protocol,
		Port:            l.DesiredListener.Port,
		DefaultActions: []*elbv2.Action{
			{
				Type:           aws.String("forward"),
				TargetGroupArn: tg.CurrentTargetGroup.TargetGroupArn,
			},
		},
	}

	createListenerOutput, err := elbv2svc.svc.CreateListener(createListenerInput)
	if err != nil && err.(awserr.Error).Code() != "TargetGroupAssociationLimit" {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateListener"}).Add(float64(1))
		return err
	} else if err != nil && err.(awserr.Error).Code() == "TargetGroupAssociationLimit" {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateListener"}).Add(float64(1))
		glog.Errorf("%a: Received a TargetGroupAssociationLimit error", a.Name())
		// Something strange happening here, the original Listener doesnt have the LoadBalancerArn but a describe will return a Listener with the ARN
		// l, _ := elbv2svc.describeListeners(lb.LoadBalancer.LoadBalancerArn)
		return err
	}

	l.CurrentListener = createListenerOutput.Listeners[0]
	return nil
}

// Modifies a listener
func (l *Listener) modify(a *albIngress, lb *LoadBalancer, tg *TargetGroup) error {
	if l.CurrentListener == nil {
		// not a modify, a create
		return l.create(a, lb, tg)
	}

	if l.Equals(l.DesiredListener) {
		return nil
	}

	glog.Infof("%s: Modifying existing %s listener %s", a.Name(), *lb.id, *l.CurrentListener.ListenerArn)
	glog.Infof("%s: Have %v, want %v", a.Name(), l.CurrentListener, l.DesiredListener)
	glog.Infof("%s: NOT IMPLEMENTED!!!!", a.Name())

	return nil
}

// Deletes a Listener from an existing ALB in AWS.
func (l *Listener) delete(a *albIngress) error {
	deleteListenerInput := &elbv2.DeleteListenerInput{
		ListenerArn: l.CurrentListener.ListenerArn,
	}

	// Debug logger to introspect DeleteListener request
	glog.Infof("%s: Delete listener %s", a.Name(), *l.CurrentListener.ListenerArn)
	_, err := elbv2svc.svc.DeleteListener(deleteListenerInput)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteListener"}).Add(float64(1))
		return err
	}
	return nil
}

func (l *Listener) Equals(target *elbv2.Listener) bool {
	switch {
	case l.CurrentListener == nil:
		return false
	case !awsutil.DeepEqual(l.CurrentListener.Port, target.Port):
		return false
	case !awsutil.DeepEqual(l.CurrentListener.Protocol, target.Protocol):
		return false
	case !awsutil.DeepEqual(l.CurrentListener.Certificates, target.Certificates):
		return false
	}
	return true
}

func (l Listeners) find(listener *Listener) int {
	for p, v := range l {
		if listener.Equals(v.CurrentListener) {
			return p
		}
	}
	return -1
}

// Meant to be called when we delete a targetgroup and just need to lose references to our listeners
func (l Listeners) purgeTargetGroupArn(a *albIngress, arn *string) Listeners {
	var listeners Listeners
	for _, listener := range l {
		// TODO: do we ever have more default actions?
		if *listener.CurrentListener.DefaultActions[0].TargetGroupArn != *arn {
			listeners = append(listeners, listener)
		}
	}
	return listeners
}

func (l Listeners) modify(a *albIngress, lb *LoadBalancer) error {
	var li Listeners
	for _, targetGroup := range lb.TargetGroups {
		for _, listener := range lb.Listeners {
			if listener.DesiredListener == nil {
				listener.delete(a)
				continue
			}
			if err := listener.modify(a, lb, targetGroup); err != nil {
				return err
			}
			li = append(li, listener)
		}
	}
	lb.Listeners = li
	return nil
}

func (l Listeners) delete(a *albIngress) error {
	errors := false
	for _, listener := range l {
		if err := listener.delete(a); err != nil {
			glog.Infof("%s: Unable to delete listener %s: %s",
				a.Name(),
				*listener.CurrentListener.ListenerArn,
				err)
		}
	}
	if errors {
		return fmt.Errorf("There were errors deleting listeners")
	}
	return nil
}
