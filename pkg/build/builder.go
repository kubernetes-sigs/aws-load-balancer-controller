package build

import (
	"context"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/auth"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build/tls"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	DefaultSSLPolicy = "ELBSecurityPolicy-2016-08"
)

type Builder interface {
	Build(ctx context.Context, ingGroup ingress.Group) (LoadBalancingStack, error)
}

func NewBuilder(cloud cloud.Cloud, cache cache.Cache, annotationParser k8s.AnnotationParser, ingConfig ingress.Config) Builder {
	authActionBuilder := auth.NewActionBuilder(cache, annotationParser)
	return &defaultBuilder{
		cloud:             cloud,
		cache:             cache,
		annotationParser:  annotationParser,
		authActionBuilder: authActionBuilder,
		ingConfig:         ingConfig,
	}
}

type defaultBuilder struct {
	cloud             cloud.Cloud
	cache             cache.Cache
	annotationParser  k8s.AnnotationParser
	authActionBuilder auth.ActionBuilder
	ingConfig         ingress.Config
}

func (b *defaultBuilder) Build(ctx context.Context, ingGroup ingress.Group) (LoadBalancingStack, error) {
	stack := LoadBalancingStack{
		ID: ingGroup.ID.String(),
	}

	if len(ingGroup.ActiveMembers) == 0 {
		return stack, nil
	}

	lbSpec, err := b.buildLoadBalancerSpec(ctx, ingGroup)
	if err != nil {
		return LoadBalancingStack{}, err
	}
	listeners, sgs, err := b.buildListenersAndLBSG(ctx, &stack, ingGroup, lbSpec.IPAddressType)
	if err != nil {
		return LoadBalancingStack{}, err
	}
	lbSpec.Listeners = listeners
	lbSpec.SecurityGroups = sgs
	lb := &api.LoadBalancer{
		ObjectMeta: v1.ObjectMeta{
			Name: ResourceIDLoadBalancer,
		},
		Spec: lbSpec,
	}
	stack.LoadBalancer = lb
	return stack, nil
}

func (b *defaultBuilder) buildListenersAndLBSG(ctx context.Context, stack *LoadBalancingStack, ingGroup ingress.Group, ipAddressType api.IPAddressType) ([]api.Listener, []api.SecurityGroupReference, error) {
	portsByIngress := map[types.NamespacedName]sets.Int64{}
	protocolByPort := map[int64]api.Protocol{}
	tlsPolicyByPort := map[int64]string{}
	tlsCertsByPort := map[int64][]string{}
	defaultActionsByPort := map[int64][]api.ListenerAction{}
	rulesByPort := map[int64][]api.ListenerRule{}

	annoCertBuilder := tls.NewAnnotationCertificateBuilder(b.annotationParser)
	inferACMCertBuilder := tls.NewInferACMCertificateBuilder(b.cloud)
	for _, ing := range ingGroup.ActiveMembers {
		tlsPolicy := b.buildIngressTLSPolicy(ctx, ing)
		tlsCerts, err := annoCertBuilder.Build(ctx, ing)
		if err != nil {
			return nil, nil, err
		}
		listenPorts, err := b.buildIngressListenPorts(ctx, ing, len(tlsCerts) != 0)
		if err != nil {
			return nil, nil, err
		}
		portsByIngress[k8s.NamespacedName(ing)] = sets.Int64KeySet(listenPorts)

		for port, protocol := range listenPorts {
			if existingProtocol, exists := protocolByPort[port]; exists && existingProtocol != protocol {
				return nil, nil, errors.Errorf("conflicting listener protocol for port %d: %v, %v", port, existingProtocol, protocol)
			}
			protocolByPort[port] = protocol

			if protocol == api.ProtocolHTTPS {
				if tlsPolicy != "" {
					if existingTLSPolicy, exists := tlsPolicyByPort[port]; exists && existingTLSPolicy != tlsPolicy {
						return nil, nil, errors.Errorf("conflicting listener tlsPolicy for port %d: %v, %v", port, existingTLSPolicy, tlsPolicy)
					}
					tlsPolicyByPort[port] = tlsPolicy
				}

				if len(tlsCerts) == 0 {
					var err error
					tlsCerts, err = inferACMCertBuilder.Build(ctx, ing)
					if err != nil {
						return nil, nil, err
					}
				}

				// maintain original order for tlsCertsByPort[port], since we use the first cert as default listener certificate.
				existingTLSCertSet := sets.NewString(tlsCertsByPort[port]...)
				for _, cert := range tlsCerts {
					if !existingTLSCertSet.Has(cert) {
						tlsCertsByPort[port] = append(tlsCertsByPort[port], cert)
						existingTLSCertSet.Insert(cert)
					}
				}
			}

			if ing.Spec.Backend != nil {
				if _, exists := defaultActionsByPort[port]; exists {
					return nil, nil, errors.Errorf("only one Ingress should specify default backend for port %d", port)
				}
				defaultActions, err := b.buildListenerActions(ctx, stack, ingGroup.ID, ing, *ing.Spec.Backend, protocol)
				if err != nil {
					return nil, nil, err
				}
				defaultActionsByPort[port] = defaultActions
			}

			rules, err := b.buildListenerRules(ctx, stack, ingGroup.ID, ing, port, protocol)
			if err != nil {
				return nil, nil, err
			}
			rulesByPort[port] = append(rulesByPort[port], rules...)
		}
	}

	listeners := make([]api.Listener, 0, len(protocolByPort))
	for port, protocol := range protocolByPort {
		defaultActions := defaultActionsByPort[port]
		if len(defaultActions) == 0 {
			defaultActions = []api.ListenerAction{b.buildDefault404Action()}
		}

		rules := rulesByPort[port]
		ls := api.Listener{
			Port:           port,
			Protocol:       protocol,
			DefaultActions: defaultActions,
			Rules:          rules,
		}

		if protocol == api.ProtocolHTTPS {
			if tlsPolicy, exists := tlsPolicyByPort[port]; exists {
				ls.SSLPolicy = tlsPolicy
			} else {
				ls.SSLPolicy = DefaultSSLPolicy
			}
			if tlsCerts, exists := tlsCertsByPort[port]; exists && len(tlsCerts) > 0 {
				ls.Certificates = tlsCerts
			} else {
				return nil, nil, errors.Errorf("at least one TLS Certificate must be specified for port %d", port)
			}
		}
		listeners = append(listeners, ls)
	}

	lbSecurityGroups, err := b.buildLBSecurityGroups(ctx, stack, ingGroup, portsByIngress, ipAddressType)
	if err != nil {
		return nil, nil, err
	}
	return listeners, lbSecurityGroups, nil
}
