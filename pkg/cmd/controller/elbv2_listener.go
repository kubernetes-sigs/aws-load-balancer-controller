package controller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
)

type Listener struct {
	arn          *string
	port         *int64
	protocol     *string
	certificates []*elbv2.Certificate
	Listener     *elbv2.Listener
	Rules        []*elbv2.Rule
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (l *Listener) create(a *albIngress, lb *LoadBalancer, tg *TargetGroup) error {
	// Debug logger to introspect CreateListener request
	glog.Infof("%s: Create Listener for %s sent", a.Name(), lb.hostname)
	if noop {
		return nil
	}

	l.protocol = aws.String("HTTP")
	l.port = aws.Int64(80)

	l.applyAnnotations(a)

	createListenerInput := &elbv2.CreateListenerInput{
		Certificates:    l.certificates,
		LoadBalancerArn: lb.arn,
		Protocol:        l.protocol,
		Port:            l.port,
		DefaultActions: []*elbv2.Action{
			{
				Type:           aws.String("forward"),
				TargetGroupArn: tg.arn,
			},
		},
	}

	createListenerOutput, err := elbv2svc.svc.CreateListener(createListenerInput)
	if err != nil {
		AWSErrorCount.With(prometheus.Labels{"service": "ELBV2", "request": "CreateListener"}).Add(float64(1))
		return err
	}

	l.arn = createListenerOutput.Listeners[0].ListenerArn
	return nil
}

// Modifies the attributes of an existing ALB.
// albIngress is only passed along for logging
func (l *Listener) modify(a *albIngress, lb *LoadBalancer, tg *TargetGroup) error {
	needsModify := l.checkModify(a, lb, tg)

	if !needsModify {
		return nil
	}

	glog.Infof("%s: Modifying existing %s listener %s", a.Name(), *lb.id, *l.arn)
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

func (l *Listener) checkModify(a *albIngress, lb *LoadBalancer, tg *TargetGroup) bool {
	switch {
	// certificate arn changed
	case *a.annotations.certificateArn != *l.Listener.Certificates[0].CertificateArn:
		return true
	case *a.annotations.port != *l.Listener.Port:
		return true
	case *l.protocol != *l.Listener.Protocol:
		return true
		// TODO default actions changed
		// TODO ssl policy changed
	default:
		return false
	}
}

func (l *Listener) applyAnnotations(a *albIngress) {
	switch {
	case *a.annotations.certificateArn != "":
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
