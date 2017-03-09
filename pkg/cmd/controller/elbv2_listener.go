package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (elb *ELBV2) createListener(a *albIngress) error {
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

	createListenerInput := &elbv2.CreateListenerInput{
		Certificates:    certificates,
		LoadBalancerArn: aws.String(a.loadBalancerArn),
		Protocol:        aws.String(protocol),
		Port:            port,
		DefaultActions: []*elbv2.Action{
			{
				Type:           aws.String("forward"),
				TargetGroupArn: aws.String(a.targetGroupArn),
			},
		},
	}

	// Debug logger to introspect CreateListener request
	glog.Infof("Create Listener request sent:\n%s", createListenerInput)
	if !noop {
		_, err := elb.CreateListener(createListenerInput)
		if err != nil {
			AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateListener"}).Add(float64(1))
			return err
		}
		return nil
	}
	return nil
}

// Deletes a Listener from an existing ALB in AWS.
func (elb *ELBV2) deleteListeners(a *albIngress) error {
	describeListenersInput := &elbv2.DescribeListenersInput{
		LoadBalancerArn: aws.String(a.loadBalancerArn),
	}

	// Debug logger to introspect DeleteListener request
	glog.Infof("Describe Listeners request sent:\n%s", describeListenersInput)
	describeListenersOutput, err := elb.DescribeListeners(describeListenersInput)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DescribeListeners"}).Add(float64(1))
		return err
	}

	for _, listener := range describeListenersOutput.Listeners {
		err := elb.deleteListener(listener)
		if err != nil {
			glog.Info("Unable to delete %v: %v", listener.ListenerArn, err)
		}
	}
	return nil
}

// Deletes a Listener from an existing ALB in AWS.
func (elb *ELBV2) deleteListener(listener *elbv2.Listener) error {
	deleteListenerInput := &elbv2.DeleteListenerInput{
		ListenerArn: listener.ListenerArn,
	}

	// Debug logger to introspect DeleteListener request
	glog.Infof("Delete Listener request sent:\n%s", deleteListenerInput)
	if !noop {
		_, err := elb.DeleteListener(deleteListenerInput)
		if err != nil {
			AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "DeleteListener"}).Add(float64(1))
			return err
		}
	}
	return nil
}
