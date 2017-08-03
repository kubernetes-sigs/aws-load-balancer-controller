package alb

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/awsutil"
	"github.com/coreos/alb-ingress-controller/controller/config"
	"github.com/coreos/alb-ingress-controller/controller/util"
	"github.com/coreos/alb-ingress-controller/log"
	"github.com/golang/glog"
)

// Listener contains the relevant ID, Rules, and current/desired Listeners
type Listener struct {
	IngressID       *string
	CurrentListener *elbv2.Listener
	DesiredListener *elbv2.Listener
	Rules           Rules
	deleted         bool
}

// NewListener returns a new alb.Listener based on the parameters provided.
func NewListener(annotations *config.Annotations, ingressID *string) []*Listener {
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
			IngressID:       ingressID,
		}

		listeners = append(listeners, listenerT)
	}

	return listeners
}

// Reconcile compares the current and desired state of this Listener instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS listener to
// satisfy the ingress's current state.
func (l *Listener) Reconcile(lb *LoadBalancer) error {
	switch {

	case l.DesiredListener == nil: // listener should be deleted
		if l.CurrentListener == nil {
			break
		}
		log.Infof("Start Listener deletion.", *l.IngressID)
		if err := l.delete(lb); err != nil {
			return err
		}
		log.Infof("Completed Listener deletion.", *l.IngressID)

	case l.CurrentListener == nil: // listener doesn't exist and should be created
		log.Infof("Start Listener creation.", *l.IngressID)
		if err := l.create(lb); err != nil {
			return err
		}
		log.Infof("Completed Listener creation. ARN: %s | Port: %s | Proto: %s.",
			*l.IngressID, *l.CurrentListener.ListenerArn, *l.CurrentListener.Port,
			*l.CurrentListener.Protocol)

	case l.needsModification(l.DesiredListener): // current and desired diff; needs mod
		log.Infof("Start Listener modification.", *l.IngressID)
		if err := l.modify(lb); err != nil {
			return err
		}

	default:
		log.Debugf("No listener modification required.", *l.IngressID)
	}

	return nil
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (l *Listener) create(lb *LoadBalancer) error {
	l.DesiredListener.LoadBalancerArn = lb.CurrentLoadBalancer.LoadBalancerArn

	// TODO: If we couldn't resolve default, we 'default' to the first targetgroup known.
	// Questionable approach.
	l.DesiredListener.DefaultActions[0].TargetGroupArn = lb.TargetGroups[0].CurrentTargetGroup.TargetGroupArn

	// Look for the default rule in the list of rules known to the Listener. If the default is found,
	// use the Kubernetes service name attached to that.
	for _, rule := range l.Rules {
		if *rule.DesiredRule.IsDefault {
			log.Infof("Located default rule. Rule: %s", *l.IngressID, log.Prettify(rule.DesiredRule))
			tgIndex := lb.TargetGroups.LookupBySvc(rule.SvcName)
			if tgIndex < 0 {
				log.Errorf("Failed to locate TargetGroup related to this service. Defaulting to first Target Group. SVC: %s",
					*l.IngressID, rule.SvcName)
			} else {
				ctg := lb.TargetGroups[tgIndex].CurrentTargetGroup
				l.DesiredListener.DefaultActions[0].TargetGroupArn = ctg.TargetGroupArn
			}
		}
	}

	// Attempt listener creation.
	in := elbv2.CreateListenerInput{
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
	o, err := awsutil.ALBsvc.AddListener(in)
	if err != nil {
		log.Errorf("Failed Listener creation. Error: %s.", *l.IngressID, err.Error())
		return err
	}

	l.CurrentListener = o
	return nil
}

// Modifies a listener
// TODO: Determine if this needs to be implemented and if so, implement it.
func (l *Listener) modify(lb *LoadBalancer) error {
	if l.CurrentListener == nil {
		// not a modify, a create
		return l.create(lb)
	}

	glog.Infof("Modifying existing %s listener %s", *lb.ID, *l.CurrentListener.ListenerArn)
	glog.Info("NOT IMPLEMENTED!!!!")

	log.Infof("Completed Listener modification. ARN: %s | Port: %s | Proto: %s.",
		*l.IngressID, *l.CurrentListener.ListenerArn, *l.CurrentListener.Port, *l.CurrentListener.Protocol)
	return nil
}

// delete adds a Listener from an existing ALB in AWS.
func (l *Listener) delete(lb *LoadBalancer) error {
	in := elbv2.DeleteListenerInput{
		ListenerArn: l.CurrentListener.ListenerArn,
	}

	if err := awsutil.ALBsvc.RemoveListener(in); err != nil {
		log.Errorf("Failed Listener deletion. ARN: %s | Error: %s", *l.IngressID,
			*l.CurrentListener.ListenerArn, err.Error())
		return err
	}

	l.deleted = true
	return nil
}

func (l *Listener) needsModification(target *elbv2.Listener) bool {
	switch {
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
