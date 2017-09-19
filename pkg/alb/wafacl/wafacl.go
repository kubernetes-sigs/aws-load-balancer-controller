package wafacl

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/annotations"
	"github.com/coreos/alb-ingress-controller/pkg/aws/waf"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

var logger *log.Logger

func init() {
	logger = log.New("wafacl")
}

// WafAcl contains the relevant ID
type WafAcl struct {
	IngressID       *string
	LoadBalancerArn *string
	CurrentWafAclId *string
	DesiredWafAclId *string
}

// NewWafAcl returns a WAF ACL
func NewWafAcl(annotations *annotations.Annotations, loadBalancer *elbv2.LoadBalancer, ingressID *string) *WafAcl {
	record := &WafAcl{
		IngressID:       ingressID,
		DesiredWafAclId: annotations.WafAclId,
	}

	if loadBalancer != nil {
		record.LoadBalancerArn = loadBalancer.LoadBalancerArn
	}

	return record
}

// Reconcile compares the current and desired WAF ACL of Load Balancer. Comparison
// results in no action, the association, the disassociation, or the modification of WAF ACL
// record set to satisfy the ingress's current state.
func (w *WafAcl) Reconcile(lb *elbv2.LoadBalancer) error {
	switch {
	case w.DesiredWafAclId == nil: // should be deassociated
		if w.CurrentWafAclId == nil {
			break
		}
		logger.Infof("Start WAF ACL disassociation.", *w.IngressID)
		if err := w.disassociate(); err != nil {
			return err
		}
		logger.Infof("Completed WAF ACL disassociation.", *w.IngressID)

	case w.CurrentWafAclId == nil: // should be associated
		logger.Infof("Start WAF ACL association.", *w.IngressID)
		if err := w.associate(); err != nil {
			return err
		}
		logger.Infof("Completed WAF ACL association.", *w.IngressID)

	default: // check for diff between current and desired acl; mod if needed
		if *w.CurrentWafAclId != *w.DesiredWafAclId {
			logger.Infof("Start WAF ACL modification.", *w.IngressID)
			if err := w.associate(); err != nil {
				return err
			}
			logger.Infof("Completed WAF ACL modification.", *w.IngressID)
		} else {
			logger.Debugf("No modification of WAF ACL required.", *w.IngressID)
		}
	}

	return nil
}

func (w *WafAcl) associate() error {
	if w.LoadBalancerArn == nil {
		return nil
	}

	if _, err := waf.WAFRegionalsvc.Associate(w.LoadBalancerArn, w.DesiredWafAclId); err != nil {
		logger.Errorf("Failed associate WAF ACL | Error: %s", *w.IngressID, err.Error())
		return err
	}

	w.CurrentWafAclId = w.DesiredWafAclId

	return nil
}

func (w *WafAcl) disassociate() error {
	if w.LoadBalancerArn == nil {
		return nil
	}

	if _, err := waf.WAFRegionalsvc.Disassociate(w.LoadBalancerArn); err != nil {
		logger.Errorf("Failed disassociate WAF ACL | Error: %s", *w.IngressID, err.Error())
		return err
	}

	w.CurrentWafAclId = nil

	return nil
}
