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

type Listener struct {
	port         *int64
	protocol     *string
	deleted      bool
	certificates []*elbv2.Certificate
	Listener     *elbv2.Listener
	Rules        []*elbv2.Rule
}

type Listeners []*Listener

func NewListener(a *albIngress) *Listener {
	listener := &Listener{
		port:     aws.Int64(80),
		protocol: aws.String("HTTP"),
	}
	listener.applyAnnotations(a)

	return listener
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (l *Listener) create(a *albIngress, lb *LoadBalancer, tg *TargetGroup) error {
	// Debug logger to introspect CreateListener request
	glog.Infof("%s: Create Listener for %s sent", a.Name(), *lb.hostname)
	if noop {
		return nil
	}

	createListenerInput := &elbv2.CreateListenerInput{
		Certificates:    l.certificates,
		LoadBalancerArn: lb.LoadBalancer.LoadBalancerArn,
		Protocol:        l.protocol,
		Port:            l.port,
		DefaultActions: []*elbv2.Action{
			{
				Type:           aws.String("forward"),
				TargetGroupArn: tg.TargetGroup.TargetGroupArn,
			},
		},
	}

	createListenerOutput, err := elbv2svc.svc.CreateListener(createListenerInput)
	if err != nil && err.(awserr.Error).Code() != "TargetGroupAssociationLimit" {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateListener"}).Add(float64(1))
		return err
	} else if err != nil && err.(awserr.Error).Code() == "TargetGroupAssociationLimit" {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateListener"}).Add(float64(1))
		// Something strange happening here, the original Listener doesnt have the LoadBalancerArn but a describe will return a Listener with the ARN
		// l, _ := elbv2svc.describeListeners(lb.LoadBalancer.LoadBalancerArn)
		return err
	}

	l.Listener = createListenerOutput.Listeners[0]
	return nil
}

// Modifies a listener
func (l *Listener) modify(a *albIngress, lb *LoadBalancer, tg *TargetGroup) error {
	if l.Listener == nil {
		// not a modify, a create

		return l.create(a, lb, tg)
	}

	newListener := NewListener(a)
	if newListener.Hash() == l.Hash() {
		return nil
	}

	glog.Infof("%s: Modifying existing %s listener %s", a.Name(), *lb.id, *l.Listener.ListenerArn)
	glog.Infof("%s: NOT IMPLEMENTED!!!!", a.Name())

	return nil
}

// Deletes a Listener from an existing ALB in AWS.
func (l *Listener) delete(a *albIngress) error {
	deleteListenerInput := &elbv2.DeleteListenerInput{
		ListenerArn: l.Listener.ListenerArn,
	}

	// Debug logger to introspect DeleteListener request
	glog.Infof("%s: Delete listener %s", a.Name(), *l.Listener.ListenerArn)
	if !noop {
		_, err := elbv2svc.svc.DeleteListener(deleteListenerInput)
		if err != nil {
			AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteListener"}).Add(float64(1))
			return err
		}
	}
	return nil
}

func (l *Listener) applyAnnotations(a *albIngress) {
	switch {
	case a.annotations.certificateArn != nil:
		l.certificates = []*elbv2.Certificate{
			&elbv2.Certificate{
				CertificateArn: a.annotations.certificateArn,
			},
		}
		l.protocol = aws.String("HTTPS")
		l.port = aws.Int64(443)
	case a.annotations.port != nil:
		l.port = a.annotations.port
	}
}

func (l *Listener) Hash() string {
	hasher := md5.New()
	// TODO: include certificates []*elbv2.Certificate

	hasher.Write([]byte(fmt.Sprintf("%v%v", *l.port, *l.protocol)))
	output := hex.EncodeToString(hasher.Sum(nil))
	return output
}

func (l Listeners) find(listener *Listener) int {
	for p, v := range l {
		if listener.Hash() == v.Hash() {
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
		if *listener.Listener.DefaultActions[0].TargetGroupArn != *arn {
			listeners = append(listeners, listener)
		}
	}
	return listeners
}

func (l Listeners) modify(a *albIngress, lb *LoadBalancer) error {
	var li Listeners
	for _, targetGroup := range lb.TargetGroups {
		for _, listener := range lb.Listeners {
			if listener.deleted {
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
				*listener.Listener.ListenerArn,
				err)
		}
	}
	if errors {
		return fmt.Errorf("There were errors deleting listeners")
	}
	return nil
}
