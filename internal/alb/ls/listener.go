package ls

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albacm"
	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/rs"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	api "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type NewDesiredListenerOptions struct {
	ExistingListener *Listener
	Port             loadbalancer.PortData
	CertificateArn   *string
	Logger           *log.Logger
	SslPolicy        *string
	Ingress          *extensions.Ingress
	Store            store.Storer
	TargetGroups     tg.TargetGroups
	IgnoreHostHeader *bool
}

// NewDesiredListener returns a new listener.Listener based on the parameters provided.
func NewDesiredListener(o *NewDesiredListenerOptions) (*Listener, error) {
	l := &elbv2.Listener{
		Port:     aws.Int64(o.Port.Port),
		Protocol: aws.String(elbv2.ProtocolEnumHttp),
		DefaultActions: []*elbv2.Action{
			{
				Type: aws.String(elbv2.ActionTypeEnumForward),
			},
		},
	}

	if o.Port.Scheme == elbv2.ProtocolEnumHttps {
		l.Protocol = aws.String(elbv2.ProtocolEnumHttps)
		certs, err := getCertificates(o.CertificateArn, o.Ingress, o.Logger)
		if err != nil {
			return nil, err
		}
		l.Certificates = certs

		if o.SslPolicy != nil {
			l.SslPolicy = o.SslPolicy
		}
	}

	listener := &Listener{
		ls:             ls{desired: l},
		logger:         o.Logger,
		defaultBackend: o.Ingress.Spec.Backend,
	}

	if o.ExistingListener != nil {
		listener.rules = o.ExistingListener.rules
	}

	var p int
	for _, rule := range o.Ingress.Spec.Rules {
		var err error

		listener.rules, p, err = rs.NewDesiredRules(&rs.NewDesiredRulesOptions{
			Ingress:          o.Ingress,
			Store:            o.Store,
			Priority:         p,
			Logger:           o.Logger,
			ListenerRules:    listener.rules,
			ListenerProtocol: listener.ls.desired.Protocol,
			ListenerPort:     o.Port,
			Rule:             &rule,
			IgnoreHostHeader: o.IgnoreHostHeader,
			TargetGroups:     o.TargetGroups,
		})
		if err != nil {
			return nil, err
		}
	}

	if o.ExistingListener != nil {
		o.ExistingListener.ls.desired = listener.ls.desired
		o.ExistingListener.rules = listener.rules
		o.ExistingListener.defaultBackend = listener.defaultBackend
		return o.ExistingListener, nil
	}

	return listener, nil
}

type NewCurrentListenerOptions struct {
	Listener     *elbv2.Listener
	Logger       *log.Logger
	TargetGroups tg.TargetGroups
}

// NewCurrentListener returns a new listener.Listener based on an elbv2.Listener.
func NewCurrentListener(o *NewCurrentListenerOptions) (*Listener, error) {
	rules, err := rs.NewCurrentRules(&rs.NewCurrentRulesOptions{
		ListenerArn:  o.Listener.ListenerArn,
		Logger:       o.Logger,
		TargetGroups: o.TargetGroups,
	})
	if err != nil {
		return nil, err
	}

	resourceTags, err := albrgt.RGTsvc.GetClusterResources()
	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS tags. Error: %s", err.Error())
	}

	var defaultBackend *extensions.IngressBackend
	if *o.Listener.DefaultActions[0].Type == elbv2.ActionTypeEnumForward {
		tgArn := *o.Listener.DefaultActions[0].TargetGroupArn

		tgTags, ok := resourceTags.TargetGroups[tgArn]
		if !ok {
			return nil, fmt.Errorf("TargetGroup %v does not exist in tag map", tgArn)
		}

		svcName, svcPort, err := tgTags.ServiceNameAndPort()
		if err != nil {
			return nil, fmt.Errorf("The Target Group %s does not have the proper tags, can't import: %s", tgArn, err.Error())
		}

		defaultBackend = &extensions.IngressBackend{
			ServiceName: svcName,
			ServicePort: svcPort,
		}
	} else {
		defaultBackend = &extensions.IngressBackend{
			ServicePort: intstr.FromString(action.UseActionAnnotation),
		}
	}

	return &Listener{
		ls:             ls{current: o.Listener},
		logger:         o.Logger,
		defaultBackend: defaultBackend,
		rules:          rules,
	}, nil
}

func (l *Listener) resolveDefaultBackend(rOpts *ReconcileOptions) (*elbv2.Action, error) {
	if action.Use(l.defaultBackend.ServicePort.String()) {
		annos, err := rOpts.Store.GetIngressAnnotations(k8s.MetaNamespaceKey(rOpts.Ingress))
		if err != nil {
			return nil, err
		}
		if annos.Action == nil {
			return nil, nil
		}
		return annos.Action.GetAction(l.defaultBackend.ServiceName)
	}

	i := rOpts.TargetGroups.LookupByBackend(*l.defaultBackend)
	if i < 0 {
		return nil, fmt.Errorf("cannot reconcile listeners, unable to find a target group for default backend %s",
			l.defaultBackend.String())
	}

	action := l.ls.desired.DefaultActions[0]
	action.TargetGroupArn = rOpts.TargetGroups[i].CurrentARN()
	return action, nil
}

// Reconcile compares the current and desired state of this Listener instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS listener to
// satisfy the ingress's current state.

func (l *Listener) Reconcile(rOpts *ReconcileOptions) (err error) {
	// If there is a desired listener, set some of the ARNs which are not available when we assemble the desired state
	if l.ls.desired != nil {
		l.ls.desired.LoadBalancerArn = rOpts.LoadBalancerArn
		l.ls.desired.DefaultActions[0], err = l.resolveDefaultBackend(rOpts)
		if err != nil {
			return err
		}
	}

	switch {
	case l.ls.desired == nil: // listener should be deleted
		if l.ls.current == nil {
			break
		}
		l.logger.Infof("Start Listener deletion.")
		if err := l.delete(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%v listener deleted", *l.ls.current.Port)
		l.logger.Infof("Completed Listener deletion.")

	case l.ls.current == nil && l.ls.desired != nil: // listener doesn't exist and should be created
		l.logger.Infof("Start Listener creation.")
		if err := l.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%v listener created", *l.ls.current.Port)
		l.logger.Infof("Completed Listener creation. ARN: %s | Port: %v | Proto: %s.",
			*l.ls.current.ListenerArn, *l.ls.current.Port,
			*l.ls.current.Protocol)

	case l.needsModification(rOpts): // current and desired diff; needs mod
		if err := l.modify(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%v listener modified", *l.ls.current.Port)
	}

	if l.ls.current != nil {
		if rs, err := l.rules.Reconcile(&rs.ReconcileOptions{
			Eventf:       rOpts.Eventf,
			ListenerArn:  l.ls.current.ListenerArn,
			TargetGroups: rOpts.TargetGroups,
		}); err != nil {
			return err
		} else {
			l.rules = rs
		}
	}

	return nil
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (l *Listener) create(rOpts *ReconcileOptions) error {
	// Attempt listener creation.
	desired := l.ls.desired
	in := &elbv2.CreateListenerInput{
		Certificates:    defaultCertificate(desired.Certificates),
		LoadBalancerArn: desired.LoadBalancerArn,
		Protocol:        desired.Protocol,
		Port:            desired.Port,
		SslPolicy:       desired.SslPolicy,
		DefaultActions:  desired.DefaultActions,
	}
	o, err := albelbv2.ELBV2svc.CreateListener(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %v listener: %s", *desired.Port, err.Error())
		return fmt.Errorf("Failed Listener creation: %s.", err.Error())
	}

	for _, cert := range otherCertificates(desired.Certificates) {
		_, err = albelbv2.ELBV2svc.AddListenerCertificates(&elbv2.AddListenerCertificatesInput{
			ListenerArn:  o.Listeners[0].ListenerArn,
			Certificates: []*elbv2.Certificate{cert},
		})
		if err != nil {
			rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error adding certificate %v to listener %v: %s", cert.CertificateArn, *desired.Port, err.Error())
			l.logger.Infof("Error adding certificate %v to listener %v: %s", cert.CertificateArn, *desired.Port, err.Error())
		}
	}

	l.ls.current = o.Listeners[0]
	return nil
}

func defaultCertificate(certs []*elbv2.Certificate) []*elbv2.Certificate {
	if len(certs) <= 1 {
		return certs
	}
	return certs[0:]
}

func otherCertificates(certs []*elbv2.Certificate) []*elbv2.Certificate {
	if len(certs) <= 1 {
		return []*elbv2.Certificate{}
	}
	return certs[1:]
}

// Modifies a listener
func (l *Listener) modify(rOpts *ReconcileOptions) error {
	desired := l.ls.desired
	in := &elbv2.ModifyListenerInput{
		ListenerArn:    l.ls.current.ListenerArn,
		Certificates:   desired.Certificates,
		Port:           desired.Port,
		Protocol:       desired.Protocol,
		SslPolicy:      desired.SslPolicy,
		DefaultActions: desired.DefaultActions,
	}

	o, err := albelbv2.ELBV2svc.ModifyListener(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying %v listener: %s", *desired.Port, err.Error())
		return fmt.Errorf("Failed Listener modification: %s", err.Error())
	}
	l.ls.current = o.Listeners[0]

	return nil
}

// delete removes a Listener from an existing ALB in AWS.
func (l *Listener) delete(rOpts *ReconcileOptions) error {
	if err := albelbv2.ELBV2svc.RemoveListener(l.ls.current.ListenerArn); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %v listener: %s", *l.ls.current.Port, err.Error())
		return fmt.Errorf("Failed Listener deletion. ARN: %s: %s", *l.ls.current.ListenerArn, err.Error())
	}

	l.deleted = true
	return nil
}

// needsModification returns true when the current and desired listener state are not the same.
// representing that a modification to the listener should be attempted.
func (l *Listener) needsModification(rOpts *ReconcileOptions) bool {
	lsc := l.ls.current
	lsd := l.ls.desired

	switch {
	case lsc == nil && lsd == nil:
		return false
	case lsc == nil:
		l.logger.Debugf("Current is nil")
		return true
	case !util.DeepEqual(lsc.Port, lsd.Port):
		l.logger.Debugf("Port needs to be changed (%v != %v)", log.Prettify(lsc.Port), log.Prettify(lsd.Port))
		return true
	case !util.DeepEqual(lsc.Protocol, lsd.Protocol):
		l.logger.Debugf("Protocol needs to be changed (%v != %v)", log.Prettify(lsc.Protocol), log.Prettify(lsd.Protocol))
		return true
	case !util.DeepEqual(lsc.Certificates, lsd.Certificates):
		l.logger.Debugf("Certificates needs to be changed (%v != %v)", log.Prettify(lsc.Certificates), log.Prettify(lsd.Certificates))
		return true
	case !util.DeepEqual(lsc.DefaultActions, lsd.DefaultActions):
		l.logger.Debugf("DefaultActions needs to be changed (%v != %v)", log.Prettify(lsc.DefaultActions), log.Prettify(lsd.DefaultActions))
		return true
	case !util.DeepEqual(lsc.SslPolicy, lsd.SslPolicy):
		l.logger.Debugf("SslPolicy needs to be changed (%v != %v)", log.Prettify(lsc.SslPolicy), log.Prettify(lsd.SslPolicy))
		return true
	}
	return false
}

// StripDesiredState removes the desired state from the listener.
func (l *Listener) StripDesiredState() {
	l.ls.desired = nil
	l.rules.StripDesiredState()
}

// stripCurrentState removes the current state from the listener.
func (l *Listener) stripCurrentState() {
	l.ls.current = nil
	l.rules.StripCurrentState()
}

func (l *Listener) GetRules() rs.Rules {
	return l.rules
}

func (l *Listener) DefaultActionArn() *string {
	if l.ls.current == nil || len(l.ls.current.DefaultActions) < 1 || l.ls.current.DefaultActions[0].Type == nil {
		return nil
	}
	if *l.ls.current.DefaultActions[0].Type == elbv2.ActionTypeEnumRedirect {
		return l.ls.current.DefaultActions[0].TargetGroupArn
	}
	return nil
}

func getCertificates(certificateArn *string, ingress *extensions.Ingress, logger *log.Logger) ([]*elbv2.Certificate, error) {
	if certificateArn != nil {
		logger.Debugf("New desired listener has certificate-arn '%v' in annotation", certificateArn)
		return []*elbv2.Certificate{
			{CertificateArn: certificateArn},
		}, nil
	}

	logger.Debugf("New desired listener wants HTTPS, but hasn't provided an certificate-arn annotation")

	var input = &acm.ListCertificatesInput{
		CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued}),

		// AWS documentation doesn't specify what the actual default is
		MaxItems: aws.Int64(500),
	}
	var certs []*elbv2.Certificate
	var seen = map[string]bool{}
	var page = 0
	var ingressHosts = uniqueHosts(ingress)
	err := albacm.ACMsvc.ListCertificatesPages(input, func(output *acm.ListCertificatesOutput, _ bool) bool {
		logger.Debugf("%d issued certificates in AWS ACM response page %d", len(output.CertificateSummaryList), page)
		for _, c := range output.CertificateSummaryList {
			for _, h := range ingressHosts {
				if domainMatchesHost(aws.StringValue(c.DomainName), h) {
					logger.Debugf("Domain name '%s', matches TLS host '%v', adding to Listener", aws.StringValue(c.DomainName), h)
					if !seen[aws.StringValue(c.CertificateArn)] {
						certs = append(certs, &elbv2.Certificate{CertificateArn: c.CertificateArn})
						seen[aws.StringValue(c.CertificateArn)] = true
					}
				} else {
					logger.Debugf("Ignoring domain name '%s', doesn't match '%s'", aws.StringValue(c.DomainName), h)
				}
			}
		}
		page++
		return true
	})

	if err != nil {
		return nil, err
	}

	return certs, nil
}

func domainMatchesHost(domainName string, tlsHost string) bool {
	if strings.HasPrefix(domainName, "*.") {
		ds := strings.Split(domainName, ".")
		hs := strings.Split(tlsHost, ".")

		if len(ds) != len(hs) {
			return false
		}

		for i, dp := range ds {
			if i == 0 && dp == "*" {
				continue
			}
			if dp != hs[i] {
				return false
			}
		}
		return true
	}

	return domainName == tlsHost
}

func uniqueHosts(ingress *extensions.Ingress) []string {
	var result []string
	seen := map[string]bool{}

	for _, r := range ingress.Spec.Rules {
		if !seen[r.Host] {
			result = append(result, r.Host)
			seen[r.Host] = true
		}
	}
	for _, t := range ingress.Spec.TLS {
		for _, h := range t.Hosts {
			if !seen[h] {
				result = append(result, h)
				seen[h] = true
			}
		}
	}

	return result
}
