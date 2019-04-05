package deploy

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/apimachinery/pkg/util/sets"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
)

func (a *loadBalancerActuator) reconcileListeners(ctx context.Context, lb *api.LoadBalancer, lbArn string) error {
	instancesByPort, err := a.loadListenersForLoadbalancer(ctx, lbArn)
	if err != nil {
		return err
	}
	portsInUse := sets.Int64{}
	for _, ls := range lb.Spec.Listeners {
		instance, exists := instancesByPort[ls.Port]
		if exists {
			portsInUse.Insert(ls.Port)
		}
		if err := a.reconcileListener(ctx, lbArn, ls, instance); err != nil {
			return err
		}
	}

	portsUnused := sets.Int64KeySet(instancesByPort).Difference(portsInUse)
	for port := range portsUnused {
		instance := instancesByPort[port]
		logging.FromContext(ctx).Info("deleting listener", "lbArn", lbArn, "port", port)
		if _, err := a.cloud.ELBV2().DeleteListenerWithContext(ctx, &elbv2.DeleteListenerInput{
			ListenerArn: instance.ListenerArn,
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("deleted listener", "lbArn", lbArn, "port", port)
	}
	return nil
}

func (a *loadBalancerActuator) reconcileListener(ctx context.Context, lbArn string, listener api.Listener, instance *elbv2.Listener) error {
	var err error
	lsArn := ""
	if instance == nil {
		if lsArn, err = a.createLSInstance(ctx, lbArn, listener); err != nil {
			return err
		}
	} else {
		if lsArn, err = a.updateLSInstance(ctx, listener, instance); err != nil {
			return err
		}
	}
	if err := a.reconcileListenerRules(ctx, listener, lsArn); err != nil {
		return err
	}
	_, extraCerts := splitDefaultAndExtraCertificates(listener.Certificates)
	if err := a.reconcileListenerExtraCertificates(ctx, lsArn, extraCerts); err != nil {
		return err
	}
	return nil
}

func (a *loadBalancerActuator) createLSInstance(ctx context.Context, lbArn string, listener api.Listener) (string, error) {
	defaultActions, err := a.buildELBV2ListenerActions(ctx, listener.DefaultActions)
	if err != nil {
		return "", err
	}
	req := &elbv2.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Port:            aws.Int64(listener.Port),
		Protocol:        aws.String(listener.Protocol.String()),
		DefaultActions:  defaultActions,
	}
	if listener.Protocol == api.ProtocolHTTPS {
		req.SslPolicy = aws.String(listener.SSLPolicy)
		defaultCerts, _ := splitDefaultAndExtraCertificates(listener.Certificates)
		for _, cert := range defaultCerts {
			req.Certificates = append(req.Certificates, &elbv2.Certificate{
				CertificateArn: aws.String(cert),
			})
		}
	}
	logging.FromContext(ctx).Info("creating listener", "lbArn", lbArn, "port", listener.Port)
	resp, err := a.cloud.ELBV2().CreateListenerWithContext(ctx, req)
	if err != nil {
		return "", err
	}
	lsArn := aws.StringValue(resp.Listeners[0].ListenerArn)
	logging.FromContext(ctx).Info("created listener", "lbArn", lbArn, "port", listener.Port, "lsArn", lsArn)
	return lsArn, nil
}

func (a *loadBalancerActuator) updateLSInstance(ctx context.Context, listener api.Listener, instance *elbv2.Listener) (string, error) {
	lsArn := aws.StringValue(instance.ListenerArn)
	defaultActions, err := a.buildELBV2ListenerActions(ctx, listener.DefaultActions)
	if err != nil {
		return lsArn, err
	}
	defaultCerts, _ := splitDefaultAndExtraCertificates(listener.Certificates)
	if a.isLSInstanceModified(ctx, listener, instance, defaultCerts, defaultActions) {
		req := &elbv2.ModifyListenerInput{
			ListenerArn:    aws.String(lsArn),
			Port:           aws.Int64(listener.Port),
			Protocol:       aws.String(listener.Protocol.String()),
			DefaultActions: defaultActions,
		}
		if listener.Protocol == api.ProtocolHTTPS {
			req.SslPolicy = aws.String(listener.SSLPolicy)
			for _, cert := range defaultCerts {
				req.Certificates = append(req.Certificates, &elbv2.Certificate{
					CertificateArn: aws.String(cert),
				})
			}
		}
		logging.FromContext(ctx).Info("modifying listener", "port", listener.Port, "lsArn", lsArn)
		_, err := a.cloud.ELBV2().ModifyListenerWithContext(ctx, req)
		if err != nil {
			return lsArn, err
		}
		logging.FromContext(ctx).Info("modified listener", "port", listener.Port, "lsArn", lsArn)
	}
	return lsArn, nil
}

func (a *loadBalancerActuator) isLSInstanceModified(ctx context.Context, listener api.Listener, instance *elbv2.Listener, desiredDefaultCerts []string, desiredDefaultActions []*elbv2.Action) bool {
	needModification := false
	if !awsutil.DeepEqual(aws.Int64Value(instance.Port), listener.Port) {
		changeDesc := fmt.Sprintf("%v => %v", aws.Int64Value(instance.Port), listener.Port)
		logging.FromContext(ctx).V(3).Info("listener port needs modification", "listener", listener.Port, "change", changeDesc)
		needModification = true
	}

	if !awsutil.DeepEqual(aws.StringValue(instance.Protocol), listener.Protocol.String()) {
		changeDesc := fmt.Sprintf("%v => %v", aws.StringValue(instance.Protocol), listener.Protocol.String())
		logging.FromContext(ctx).V(3).Info("listener protocol needs modification", "listener", listener.Port, "change", changeDesc)
		needModification = true
	}

	if listener.Protocol == api.ProtocolHTTPS {
		if !awsutil.DeepEqual(aws.StringValue(instance.SslPolicy), listener.SSLPolicy) {
			changeDesc := fmt.Sprintf("%v => %v", aws.StringValue(instance.SslPolicy), listener.SSLPolicy)
			logging.FromContext(ctx).V(3).Info("listener ssl policy needs modification", "listener", listener.Port, "change", changeDesc)
			needModification = true
		}

		actualDefaultCerts := sets.String{}
		for _, cert := range instance.Certificates {
			actualDefaultCerts.Insert(aws.StringValue(cert.CertificateArn))
		}
		if !actualDefaultCerts.Equal(sets.NewString(desiredDefaultCerts...)) {
			changeDesc := fmt.Sprintf("%v => %v", actualDefaultCerts.List(), desiredDefaultCerts)
			logging.FromContext(ctx).V(3).Info("listener default SSL certs needs modification", "listener", listener.Port, "change", changeDesc)
			needModification = true
		}
	}

	normalizeELBV2ListenerActions(instance.DefaultActions)
	normalizeELBV2ListenerActions(desiredDefaultActions)
	if !awsutil.DeepEqual(instance.DefaultActions, desiredDefaultActions) {
		changeDesc := fmt.Sprintf("%v => %v", awsutil.Prettify(instance.DefaultActions), awsutil.Prettify(desiredDefaultActions))
		logging.FromContext(ctx).V(3).Info("listener default actions needs modification", "listener", listener.Port, "change", changeDesc)
		needModification = true
	}
	return needModification
}

func (a *loadBalancerActuator) reconcileListenerExtraCertificates(ctx context.Context, lsArn string, extraCerts []string) error {
	certificates, err := a.cloud.ELBV2().DescribeListenerCertificatesAsList(ctx, &elbv2.DescribeListenerCertificatesInput{
		ListenerArn: aws.String(lsArn),
	})
	if err != nil {
		return err
	}

	actualExtraCertificates := sets.NewString()
	for _, certificate := range certificates {
		if !aws.BoolValue(certificate.IsDefault) {
			actualExtraCertificates.Insert(aws.StringValue(certificate.CertificateArn))
		}
	}
	desiredExtraCertificates := sets.NewString(extraCerts...)

	certificatesToAdd := desiredExtraCertificates.Difference(actualExtraCertificates)
	certificatesToRemove := actualExtraCertificates.Difference(desiredExtraCertificates)
	for certARN := range certificatesToAdd {
		logging.FromContext(ctx).Info("adding certificate to listener", "lsArn", lsArn, "certArn", certARN)
		if _, err := a.cloud.ELBV2().AddListenerCertificatesWithContext(ctx, &elbv2.AddListenerCertificatesInput{
			ListenerArn: aws.String(lsArn),
			Certificates: []*elbv2.Certificate{
				{
					CertificateArn: aws.String(certARN),
				},
			},
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("added certificate to listener", "lsArn", lsArn, "certArn", certARN)
	}
	for certARN := range certificatesToRemove {
		logging.FromContext(ctx).Info("removing certificate to listener", "lsArn", lsArn, "certArn", certARN)
		if _, err := a.cloud.ELBV2().RemoveListenerCertificatesWithContext(ctx, &elbv2.RemoveListenerCertificatesInput{
			ListenerArn: aws.String(lsArn),
			Certificates: []*elbv2.Certificate{
				{
					CertificateArn: aws.String(certARN),
				},
			},
		}); err != nil {
			return err
		}
		logging.FromContext(ctx).Info("removed certificate to listener", "lsArn", lsArn, "certArn", certARN)
	}
	return nil
}

func (a *loadBalancerActuator) loadListenersForLoadbalancer(ctx context.Context, lbArn string) (map[int64]*elbv2.Listener, error) {
	instances, err := a.cloud.ELBV2().DescribeListenersAsList(ctx, &elbv2.DescribeListenersInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	if err != nil {
		return nil, err
	}

	instanceByPort := make(map[int64]*elbv2.Listener)
	for _, instance := range instances {
		instanceByPort[aws.Int64Value(instance.Port)] = instance
	}
	return instanceByPort, nil
}

func splitDefaultAndExtraCertificates(totalCertificates []string) ([]string, []string) {
	if len(totalCertificates) > 0 {
		return totalCertificates[0:1], totalCertificates[1:]
	}
	return nil, nil
}
