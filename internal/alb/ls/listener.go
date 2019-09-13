package ls

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/pkg/errors"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/auth"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	extensions "k8s.io/api/extensions/v1beta1"
)

const (
	AnnotationSSLPolicy      = "ssl-policy"
	AnnotationCertificateARN = "certificate-arn"
)

const (
	DefaultSSLPolicy = "ELBSecurityPolicy-2016-08"
)

type ReconcileOptions struct {
	LBArn        string
	Ingress      *extensions.Ingress
	IngressAnnos *annotations.Ingress
	Port         loadbalancer.PortData
	TGGroup      tg.TargetGroupGroup

	// If instance is specified, reconcile will operate on this instance, otherwise new listener instance will be created.
	Instance *elbv2.Listener
}

type Controller interface {
	// Reconcile will make sure an AWS listener exists to satisfy requirements specified as options.
	Reconcile(ctx context.Context, options ReconcileOptions) error
}

func NewController(cloud aws.CloudAPI, authModule auth.Module) Controller {
	rulesController := NewRulesController(cloud, authModule)
	certDiscovery := NewACMCertDiscovery(cloud)
	return &defaultController{
		cloud:           cloud,
		authModule:      authModule,
		rulesController: rulesController,
		certDiscovery:   certDiscovery,
	}
}

type defaultController struct {
	cloud           aws.CloudAPI
	authModule      auth.Module
	rulesController RulesController
	certDiscovery   CertDiscovery
}

type listenerConfig struct {
	Port           *int64
	Protocol       *string
	DefaultActions []*elbv2.Action

	SslPolicy            *string
	DefaultCertificate   []*elbv2.Certificate
	ExtraCertificateARNs []string
}

func (controller *defaultController) Reconcile(ctx context.Context, options ReconcileOptions) error {
	config, err := controller.buildListenerConfig(ctx, options)
	if err != nil {
		return fmt.Errorf("failed to build listener config due to %v", err)
	}

	instance := options.Instance
	if instance == nil {
		if instance, err = controller.newLSInstance(ctx, options.LBArn, config); err != nil {
			return fmt.Errorf("failed to create listener due to %v", err)
		}
	} else {
		if instance, err = controller.reconcileLSInstance(ctx, instance, config); err != nil {
			return fmt.Errorf("failed to reconcile listener due to %v", err)
		}
	}

	if options.Port.Scheme == elbv2.ProtocolEnumHttps {
		lsArn := aws.StringValue(instance.ListenerArn)
		if err := controller.reconcileExtraCertificates(ctx, lsArn, config.ExtraCertificateARNs); err != nil {
			return errors.Wrapf(err, "failed to reconcile extra certificates on listener %v", lsArn)
		}
	}

	if err := controller.rulesController.Reconcile(ctx, instance, options.Ingress, options.IngressAnnos, options.TGGroup); err != nil {
		return fmt.Errorf("failed to reconcile rules due to %v", err)
	}
	return nil
}

func (controller *defaultController) newLSInstance(ctx context.Context, lbArn string, config listenerConfig) (*elbv2.Listener, error) {
	albctx.GetLogger(ctx).Infof("creating listener %v", aws.Int64Value(config.Port))
	resp, err := controller.cloud.CreateListenerWithContext(ctx, &elbv2.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn),
		Port:            config.Port,
		Protocol:        config.Protocol,
		Certificates:    config.DefaultCertificate,
		SslPolicy:       config.SslPolicy,
		DefaultActions:  config.DefaultActions,
	})
	if err != nil {
		return nil, err
	}

	// due to EC2's eventual consistency, sometimes we get "not found" errors when trying to interact with a listener (e.g. Describe) directly after creating it
	time.Sleep(1)

	return resp.Listeners[0], nil
}

func (controller *defaultController) reconcileLSInstance(ctx context.Context, instance *elbv2.Listener, config listenerConfig) (*elbv2.Listener, error) {
	if controller.LSInstanceNeedsModification(ctx, instance, config) {
		albctx.GetLogger(ctx).Infof("modifying listener %v, arn: %v", aws.Int64Value(config.Port), aws.StringValue(instance.ListenerArn))
		output, err := controller.cloud.ModifyListenerWithContext(ctx, &elbv2.ModifyListenerInput{
			ListenerArn:    instance.ListenerArn,
			Port:           config.Port,
			Protocol:       config.Protocol,
			Certificates:   config.DefaultCertificate,
			SslPolicy:      config.SslPolicy,
			DefaultActions: config.DefaultActions,
		})
		if err != nil {
			return instance, err
		}
		return output.Listeners[0], nil
	}
	return instance, nil
}

func (controller *defaultController) LSInstanceNeedsModification(ctx context.Context, instance *elbv2.Listener, config listenerConfig) bool {
	needModification := false
	if !util.DeepEqual(instance.Port, config.Port) {
		albctx.GetLogger(ctx).DebugLevelf(1, "listener port needs modification: %v => %v", awsutil.Prettify(instance.Port), awsutil.Prettify(config.Port))
		needModification = true
	}
	if !util.DeepEqual(instance.Protocol, config.Protocol) {
		albctx.GetLogger(ctx).DebugLevelf(1, "listener protocol needs modification: %v => %v", awsutil.Prettify(instance.Protocol), awsutil.Prettify(config.Protocol))
		needModification = true
	}
	if !util.DeepEqual(instance.Certificates, config.DefaultCertificate) {
		albctx.GetLogger(ctx).DebugLevelf(1, "listener certificates needs modification: %v => %v", awsutil.Prettify(instance.Certificates), awsutil.Prettify(config.DefaultCertificate))
		needModification = true
	}
	if !util.DeepEqual(instance.SslPolicy, config.SslPolicy) {
		albctx.GetLogger(ctx).DebugLevelf(1, "listener sslPolicy needs modification: %v => %v", awsutil.Prettify(instance.SslPolicy), awsutil.Prettify(config.SslPolicy))
		needModification = true
	}
	if !util.DeepEqual(instance.DefaultActions, config.DefaultActions) {
		albctx.GetLogger(ctx).DebugLevelf(1, "listener defaultActions needs modification: %v => %v", awsutil.Prettify(instance.DefaultActions), awsutil.Prettify(config.DefaultActions))
		needModification = true
	}
	return needModification
}

func (controller *defaultController) reconcileExtraCertificates(ctx context.Context, lsArn string, extraCertificateARNs []string) error {
	certificates, err := controller.cloud.DescribeListenerCertificates(ctx, lsArn)
	if err != nil {
		return err
	}
	actualExtraCertificateArns := sets.NewString()
	for _, certificate := range certificates {
		if !aws.BoolValue(certificate.IsDefault) {
			actualExtraCertificateArns.Insert(aws.StringValue(certificate.CertificateArn))
		}
	}
	desiredExtraCertificateArns := sets.NewString(extraCertificateARNs...)

	certificatesToAdd := desiredExtraCertificateArns.Difference(actualExtraCertificateArns)
	certificatesToRemove := actualExtraCertificateArns.Difference(desiredExtraCertificateArns)
	for certARN := range certificatesToAdd {
		albctx.GetLogger(ctx).Infof("adding certificate %v to listener %v", certARN, lsArn)
		if _, err := controller.cloud.AddListenerCertificates(ctx, &elbv2.AddListenerCertificatesInput{
			ListenerArn: aws.String(lsArn),
			Certificates: []*elbv2.Certificate{
				{
					CertificateArn: aws.String(certARN),
				},
			},
		}); err != nil {
			return err
		}
	}
	for certARN := range certificatesToRemove {
		albctx.GetLogger(ctx).Infof("removing certificate %v from listener %v", certARN, lsArn)
		if _, err := controller.cloud.RemoveListenerCertificates(ctx, &elbv2.RemoveListenerCertificatesInput{
			ListenerArn: aws.String(lsArn),
			Certificates: []*elbv2.Certificate{
				{
					CertificateArn: aws.String(certARN),
				},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (controller *defaultController) buildListenerConfig(ctx context.Context, options ReconcileOptions) (listenerConfig, error) {
	config := listenerConfig{
		Port:     aws.Int64(options.Port.Port),
		Protocol: aws.String(options.Port.Scheme),
	}
	if options.Port.Scheme == elbv2.ProtocolEnumHttps {
		sslPolicy := DefaultSSLPolicy
		_ = annotations.LoadStringAnnotation(AnnotationSSLPolicy, &sslPolicy, options.Ingress.Annotations)
		config.SslPolicy = aws.String(sslPolicy)

		var certificateARNs []string
		_ = annotations.LoadStringSliceAnnotation(AnnotationCertificateARN, &certificateARNs, options.Ingress.Annotations)
		if len(certificateARNs) == 0 {
			certs, err := controller.inferCertARNs(ctx, options.Ingress)
			if err != nil {
				return config, errors.Errorf("missing certificates annotation %v and could not auto-load certificates from ACM: %v",
					parser.GetAnnotationWithPrefix(AnnotationCertificateARN), err)
			}
			if len(certs) == 0 {
				return config, errors.Errorf("missing certificates annotation %v could not find any matching certificates from ACM to auto-load",
					parser.GetAnnotationWithPrefix(AnnotationCertificateARN))
			}

			albctx.GetLogger(ctx).Infof("Auto-detected and added %d certificates to listener", len(certs))
			certificateARNs = certs
		}
		config.DefaultCertificate = []*elbv2.Certificate{
			{
				CertificateArn: aws.String(certificateARNs[0]),
			},
		}
		config.ExtraCertificateARNs = certificateARNs[1:]
	}

	actions, err := controller.buildDefaultActions(ctx, options)
	if err != nil {
		return config, err
	}
	config.DefaultActions = actions

	return config, nil
}

func (controller *defaultController) buildDefaultActions(ctx context.Context, options ReconcileOptions) ([]*elbv2.Action, error) {
	backend := action.Default404Backend()
	if options.Ingress.Spec.Backend != nil {
		backend = *options.Ingress.Spec.Backend
	}
	authCfg, err := controller.authModule.NewConfig(ctx, options.Ingress, backend, options.Port.Scheme)
	if err != nil {
		return nil, err
	}
	return buildActions(ctx, authCfg, options.IngressAnnos, backend, options.TGGroup)
}

// inferCertARNs retrieves a set of certificates from ACM that matches the ingress' hosts list
// If multiple or none certificate were found for specific host, an error will be issued.
func (controller *defaultController) inferCertARNs(ctx context.Context, ingress *extensions.Ingress) ([]string, error) {
	var ingressHosts = uniqueHosts(ingress)
	if len(ingressHosts) == 0 {
		return nil, nil
	}
	return controller.certDiscovery.Discover(ctx, ingressHosts)
}

func uniqueHosts(ingress *extensions.Ingress) sets.String {
	hosts := sets.NewString()

	for _, r := range ingress.Spec.Rules {
		if r.Host == "" {
			continue
		}
		hosts.Insert(r.Host)
	}

	for _, t := range ingress.Spec.TLS {
		hosts.Insert(t.Hosts...)
	}

	return hosts
}
