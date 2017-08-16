package alb

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/config"
	awsutil "github.com/coreos/alb-ingress-controller/pkg/util/aws"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
	api "k8s.io/api/core/v1"
)

// Listener contains the relevant ID, Rules, and current/desired Listeners
type Listener struct {
	CurrentListener *elbv2.Listener
	DesiredListener *elbv2.Listener
	Rules           Rules
	deleted         bool
	logger          *log.Logger
}

// NewListener returns a new alb.Listener based on the parameters provided.
func NewListener(annotations *config.Annotations, logger *log.Logger) []*Listener {
	listeners := []*Listener{}

	// Creates a listener per port:protocol combination.
	for _, port := range annotations.Ports {

		listener := &elbv2.Listener{
			Port:     aws.Int64(port.Port),
			Protocol: aws.String("HTTP"),
			DefaultActions: []*elbv2.Action{
				{
					Type: aws.String("forward"),
				},
			},
		}

		if port.HTTPS {
			listener.Certificates = []*elbv2.Certificate{
				{CertificateArn: annotations.CertificateArn},
			}
			listener.Protocol = aws.String("HTTPS")
		}

		listenerT := &Listener{
			DesiredListener: listener,
			logger:          logger,
		}

		listeners = append(listeners, listenerT)
	}

	return listeners
}

// NewListenerFromAWSListener returns a new alb.Listener based on an elbv2.Listener.
func NewListenerFromAWSListener(listener *elbv2.Listener, logger *log.Logger) *Listener {
	listenerT := &Listener{
		CurrentListener: listener,
		logger:          logger,
	}

	return listenerT
}

// Reconcile compares the current and desired state of this Listener instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS listener to
// satisfy the ingress's current state.
func (l *Listener) Reconcile(rOpts *ReconcileOptions) error {
	switch {

	case l.DesiredListener == nil: // listener should be deleted
		if l.CurrentListener == nil {
			break
		}
		l.logger.Infof("Start Listener deletion.")
		if err := l.delete(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%v listener deleted", *l.CurrentListener.Port)
		l.logger.Infof("Completed Listener deletion.")

	case l.CurrentListener == nil: // listener doesn't exist and should be created
		l.logger.Infof("Start Listener creation.")
		if err := l.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%v listener created", *l.CurrentListener.Port)
		l.logger.Infof("Completed Listener creation. ARN: %s | Port: %v | Proto: %s.",
			*l.CurrentListener.ListenerArn, *l.CurrentListener.Port,
			*l.CurrentListener.Protocol)

	case l.needsModification(l.DesiredListener): // current and desired diff; needs mod
		l.logger.Infof("Start Listener modification.")
		if err := l.modify(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%v listener modified", *l.CurrentListener.Port)
		l.logger.Infof("Completed Listener modification. ARN: %s | Port: %s | Proto: %s.",
			*l.CurrentListener.ListenerArn, *l.CurrentListener.Port, *l.CurrentListener.Protocol)

	default:
		l.logger.Debugf("No listener modification required.")
	}

	return nil
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (l *Listener) create(rOpts *ReconcileOptions) error {
	lb := rOpts.loadbalancer
	l.DesiredListener.LoadBalancerArn = lb.CurrentLoadBalancer.LoadBalancerArn

	// Set the listener default action to the targetgroup from the default rule.
	for _, rule := range l.Rules {
		if *rule.DesiredRule.IsDefault {
			l.DesiredListener.DefaultActions[0].TargetGroupArn = rule.targetGroupArn(lb.TargetGroups)
		}
	}

	// Attempt listener creation.
	in := &elbv2.CreateListenerInput{
		Certificates:    l.DesiredListener.Certificates,
		LoadBalancerArn: l.DesiredListener.LoadBalancerArn,
		Protocol:        l.DesiredListener.Protocol,
		Port:            l.DesiredListener.Port,
		DefaultActions: []*elbv2.Action{
			{
				Type:           l.DesiredListener.DefaultActions[0].Type,
				TargetGroupArn: l.DesiredListener.DefaultActions[0].TargetGroupArn,
			},
		},
	}
	o, err := awsutil.ALBsvc.CreateListener(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %v listener: %s", *l.DesiredListener.Port, err.Error())
		l.logger.Errorf("Failed Listener creation: %s.", err.Error())
		return err
	}

	l.CurrentListener = o.Listeners[0]
	return nil
}

// Modifies a listener
// TODO: Determine if this needs to be implemented and if so, implement it.
func (l *Listener) modify(rOpts *ReconcileOptions) error {
	if l.CurrentListener == nil {
		// not a modify, a create
		return l.create(rOpts)
	}

	l.logger.Infof("Modifying existing %s listener %s", *rOpts.loadbalancer.ID, *l.CurrentListener.ListenerArn)
	l.logger.Infof("NOT IMPLEMENTED!!!!")

	return nil
}

// delete adds a Listener from an existing ALB in AWS.
func (l *Listener) delete(rOpts *ReconcileOptions) error {
	in := elbv2.DeleteListenerInput{
		ListenerArn: l.CurrentListener.ListenerArn,
	}

	if err := awsutil.ALBsvc.RemoveListener(in); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %v listener: %s", *l.CurrentListener.Port, err.Error())
		l.logger.Errorf("Failed Listener deletion. ARN: %s: %s",
			*l.CurrentListener.ListenerArn, err.Error())
		return err
	}

	l.deleted = true
	return nil
}

func (l *Listener) needsModification(target *elbv2.Listener) bool {
	switch {
	case l.CurrentListener == nil && l.DesiredListener == nil:
		return false
	case l.CurrentListener == nil:
		return true
	case !util.DeepEqual(l.CurrentListener.Port, target.Port):
		return true
	case !util.DeepEqual(l.CurrentListener.Protocol, target.Protocol):
		return true
	case !util.DeepEqual(l.CurrentListener.Certificates, target.Certificates):
		return true
	}
	return false
}
