package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type Listener struct {
	serviceName string
	Listener    *elbv2.Listener
	Rules       []*elbv2.Rule
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (elb *ELBV2) createListener(a *albIngress, lb *LoadBalancer, tg *TargetGroup) error {
	// Debug logger to introspect CreateListener request
	glog.Infof("%s: Create Listener for %s sent", a.Name(), lb.hostname)
	if noop {
		return nil
	}

	protocol := "HTTP"
	port := aws.Int64(80)
	certificates := []*elbv2.Certificate{}

	if *a.annotations.certificateArn != "" {
		certificate := &elbv2.Certificate{
			CertificateArn: a.annotations.certificateArn,
		}
		certificates = append(certificates, certificate)
		protocol = "HTTPS"
		port = aws.Int64(443)
	}

	// TODO tags

	createListenerInput := &elbv2.CreateListenerInput{
		Certificates:    certificates,
		LoadBalancerArn: lb.LoadBalancer.LoadBalancerArn,
		Protocol:        aws.String(protocol),
		Port:            port,
		DefaultActions: []*elbv2.Action{
			{
				Type:           aws.String("forward"),
				TargetGroupArn: tg.TargetGroup.TargetGroupArn,
			},
		},
	}

	_, err := elbv2svc.svc.CreateListener(createListenerInput)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateListener"}).Add(float64(1))
		return err
	}
	return nil
}

// Modifies the attributes of an existing ALB.
// albIngress is only passed along for logging
func (l *Listener) modify(a *albIngress, lb *LoadBalancer) error {
	needsModify := l.checkModify(a, lb)

	if !needsModify {
		return nil
	}

	glog.Infof("%s: Modifying existing %s listener %s", a.Name(), lb.id, l.serviceName)
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

func (elb *ELBV2) describeListeners(loadBalancerArn *string) ([]*elbv2.Listener, error) {
	var listeners []*elbv2.Listener
	describeListenersInput := &elbv2.DescribeListenersInput{
		LoadBalancerArn: loadBalancerArn,
		PageSize:        aws.Int64(100),
	}

	for {
		describeListenersOutput, err := elb.svc.DescribeListeners(describeListenersInput)
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

func (l *Listener) checkModify(a *albIngress, lb *LoadBalancer) bool {
	switch {
	// certificate arn changed
	case *a.annotations.certificateArn != "" && *a.annotations.certificateArn != *l.Listener.Certificates[0].CertificateArn:
		return true
		// TODO default actions changed
		// TODO port changed
		// TODO protocol changed
		// TODO ssl policy changed
	default:
		return false
	}
}
