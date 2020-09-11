package nlb

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
	"strconv"
	"strings"
)

const (
	LBAttrsAccessLogsS3Enabled           = "access_logs.s3.enabled"
	LBAttrsAccessLogsS3Bucket            = "access_logs.s3.bucket"
	LBAttrsAccessLogsS3Prefix            = "access_logs.s3.prefix"
	LBAttrsLoadBalancingCrossZoneEnabled = "load_balancing.cross_zone.enabled"
	TGAttrsProxyProtocolV2Enabled        = "proxy_protocol_v2.enabled"

	DefaultAccessLogS3Enabled            = false
	DefaultAccessLogsS3Bucket            = ""
	DefaultAccessLogsS3Prefix            = ""
	DefaultLoadBalancingCrossZoneEnabled = false
	DefaultProxyProtocolV2Enabled        = false
	DefaultHealthCheckProtocol           = elbv2.ProtocolTCP
	DefaultHealthCheckPort               = "traffic-port"
	DefaultHealthCheckPath               = "/"
	DefaultHealthCheckInterval           = 10
	DefaultHealthCheckTimeout            = 10
	DefaultHealthCheckHealthyThreshold   = 3
	DefaultHealthCheckUnhealthyThreshold = 3
)

type Builder interface {
	Build(ctx context.Context) (core.Stack, error)
}

type nlbBuilder struct {
	service          *corev1.Service
	key              types.NamespacedName
	annotationParser annotations.Parser
	subnetsResolver  networking.SubnetsResolver
}

func NewServiceBuilder(service *corev1.Service, subnetsResolver networking.SubnetsResolver, key types.NamespacedName, annotationParser annotations.Parser) Builder {
	return &nlbBuilder{
		service:          service,
		key:              key,
		annotationParser: annotationParser,
		subnetsResolver:  subnetsResolver,
	}
}

func (b *nlbBuilder) Build(ctx context.Context) (core.Stack, error) {
	stack := core.NewDefaultStack(k8s.NamespacedName(b.service).String())
	if !b.service.DeletionTimestamp.IsZero() {
		return stack, nil
	}
	err := b.buildModel(ctx, stack)
	return stack, err
}

func (b *nlbBuilder) buildModel(ctx context.Context, stack core.Stack) error {
	if !b.service.DeletionTimestamp.IsZero() {
		return nil
	}
	spec, err := b.loadBalancerSpec(ctx)
	if err != nil {
		return err
	}
	nlb := elbv2.NewLoadBalancer(stack, k8s.NamespacedName(b.service).String(), spec)
	err = b.buildListeners(ctx, stack, nlb)
	if err != nil {
		return err
	}
	return nil
}

func (b *nlbBuilder) loadBalancerSpec(ctx context.Context) (elbv2.LoadBalancerSpec, error) {
	ipAddressType := elbv2.IPAddressTypeIPV4
	var scheme elbv2.LoadBalancerScheme = elbv2.LoadBalancerSchemeInternetFacing
	internal := false
	if _, err := b.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixInternal, &internal, b.service.Annotations); err != nil {
		return elbv2.LoadBalancerSpec{}, err
	} else if internal {
		scheme = elbv2.LoadBalancerSchemeInternal
	}

	lbAttributes, err := b.buildLBAttributes(ctx)
	if err != nil {
		return elbv2.LoadBalancerSpec{}, err
	}
	tags := map[string]string{}
	b.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixAdditionalTags, &tags, b.service.Annotations)
	subnets, err := b.subnetsResolver.DiscoverSubnets(ctx, scheme)
	if err != nil {
		return elbv2.LoadBalancerSpec{}, err
	}
	subnetMappings, err := b.getSubnetMappings(subnets)
	if err != nil {
		return elbv2.LoadBalancerSpec{}, err
	}
	spec := elbv2.LoadBalancerSpec{
		Name:                   b.loadbalancerName(b.service),
		Type:                   elbv2.LoadBalancerTypeNetwork,
		Scheme:                 &scheme,
		IPAddressType:          &ipAddressType,
		SubnetMappings:         subnetMappings,
		LoadBalancerAttributes: lbAttributes,
		Tags:                   tags,
	}
	return spec, nil
}

func (b *nlbBuilder) getSubnetMappings(subnets []string) ([]elbv2.SubnetMapping, error) {
	if len(subnets) == 0 {
		return []elbv2.SubnetMapping{}, errors.Errorf("Unable to discover at least 1 subnet across availability zones")
	}
	subnetMappings := make([]elbv2.SubnetMapping, 0, len(subnets))
	for _, subnet := range subnets {
		subnetMappings = append(subnetMappings, elbv2.SubnetMapping{
			SubnetID: subnet,
		})
	}
	return subnetMappings, nil
}

func (b *nlbBuilder) buildLBAttributes(ctx context.Context) ([]elbv2.LoadBalancerAttribute, error) {
	attrs := []elbv2.LoadBalancerAttribute{}
	accessLogEnabled := DefaultAccessLogS3Enabled
	bucketName := DefaultAccessLogsS3Bucket
	bucketPrefix := DefaultAccessLogsS3Prefix
	if _, err := b.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixAccessLogEnabled, &accessLogEnabled, b.service.Annotations); err != nil {
		return attrs, err
	}
	if accessLogEnabled {
		b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixAccessLogS3BucketName, &bucketName, b.service.Annotations)
		b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixAccessLogS3BucketPrefix, &bucketPrefix, b.service.Annotations)
	}
	crossZoneEnabled := DefaultLoadBalancingCrossZoneEnabled
	if _, err := b.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixCrossZoneLoadBalancingEnabled, &crossZoneEnabled, b.service.Annotations); err != nil {
		return []elbv2.LoadBalancerAttribute{}, err
	}

	attrs = append(attrs, []elbv2.LoadBalancerAttribute{
		{
			Key:   LBAttrsAccessLogsS3Enabled,
			Value: strconv.FormatBool(accessLogEnabled),
		},
		{
			Key:   LBAttrsAccessLogsS3Bucket,
			Value: bucketName,
		},
		{
			Key:   LBAttrsAccessLogsS3Prefix,
			Value: bucketPrefix,
		},
		{
			Key:   LBAttrsLoadBalancingCrossZoneEnabled,
			Value: strconv.FormatBool(crossZoneEnabled),
		},
	}...)

	return attrs, nil
}

func (b *nlbBuilder) buildTargetHealthCheck(ctx context.Context) (*elbv2.TargetGroupHealthCheckConfig, error) {
	hc := elbv2.TargetGroupHealthCheckConfig{}
	protocol := string(DefaultHealthCheckProtocol)
	b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCProtocol, &protocol, b.service.Annotations)
	protocol = strings.ToUpper(protocol)
	hc.Protocol = (*elbv2.Protocol)(&protocol)

	path := DefaultHealthCheckPath
	b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPath, &path, b.service.Annotations)
	if protocol != string(elbv2.ProtocolTCP) {
		hc.Path = &path
	}

	healthCheckPort := intstr.FromString(DefaultHealthCheckPort)
	portAnnotationStr := DefaultHealthCheckPort
	if b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPort, &portAnnotationStr, b.service.Annotations); portAnnotationStr != DefaultHealthCheckPort {
		var portVal int64
		if _, err := b.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCPort, &portVal, b.service.Annotations); err != nil {
			return nil, err
		}
		healthCheckPort = intstr.FromInt(int(portVal))
	}
	hc.Port = &healthCheckPort

	intervalSeconds := int64(DefaultHealthCheckInterval)
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCInterval, &intervalSeconds, b.service.Annotations); err != nil {
		return nil, err
	}
	hc.IntervalSeconds = &intervalSeconds

	timeoutSeconds := int64(DefaultHealthCheckTimeout)
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCTimeout, &timeoutSeconds, b.service.Annotations); err != nil {
		return nil, err
	}
	hc.TimeoutSeconds = &timeoutSeconds

	healthyThreshold := int64(DefaultHealthCheckHealthyThreshold)
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCHealthyThreshold, &healthyThreshold, b.service.Annotations); err != nil {
		return nil, err
	}
	hc.HealthyThresholdCount = &healthyThreshold

	unhealthyThreshold := int64(DefaultHealthCheckUnhealthyThreshold)
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCUnhealthyThreshold, &unhealthyThreshold, b.service.Annotations); err != nil {
		return nil, err
	}
	hc.UnhealthyThresholdCount = &unhealthyThreshold

	return &hc, nil
}

func (b *nlbBuilder) targetGroupAttrs(ctx context.Context) ([]elbv2.TargetGroupAttribute, error) {
	attrs := []elbv2.TargetGroupAttribute{}
	proxyV2Enabled := DefaultProxyProtocolV2Enabled
	proxyV2Annotation := ""
	if b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixProxyProtocol, &proxyV2Annotation, b.service.Annotations) {
		if proxyV2Annotation != "*" {
			return []elbv2.TargetGroupAttribute{}, errors.Errorf("Invalid value %v for Load Balancer proxy protocol v2 annotation, only value currently supported is *", proxyV2Annotation)
		}
		proxyV2Enabled = true
	}
	attrs = append(attrs, elbv2.TargetGroupAttribute{
		Key:   TGAttrsProxyProtocolV2Enabled,
		Value: strconv.FormatBool(proxyV2Enabled),
	})
	return attrs, nil
}

func (b *nlbBuilder) buildListeners(ctx context.Context, stack core.Stack, lb *elbv2.LoadBalancer) error {
	tgAttrs, err := b.targetGroupAttrs(ctx)
	if err != nil {
		return errors.Wrapf(err, "Unable to build target group attributes")
	}

	var certificateARNs []string
	b.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLCertificate, &certificateARNs, b.service.Annotations)

	var sslPorts []string
	b.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLPorts, &sslPorts, b.service.Annotations)
	sslPortsSet := sets.NewString(sslPorts...)

	backendProtocol := ""
	b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixBEProtocol, &backendProtocol, b.service.Annotations)

	targetGroupMap := map[string]*elbv2.TargetGroup{}

	for _, port := range b.service.Spec.Ports {
		hc, err := b.buildTargetHealthCheck(ctx)
		tgProtocol := elbv2.Protocol(port.Protocol)
		listenerProtocol := elbv2.Protocol(port.Protocol)
		if err != nil {
			return err
		}

		if tgProtocol != elbv2.ProtocolUDP && certificateARNs != nil && (sslPortsSet.Len() == 0 || sslPortsSet.Has(port.Name) || sslPortsSet.Has(strconv.Itoa(int(port.Port)))) {
			if backendProtocol == "ssl" {
				tgProtocol = elbv2.ProtocolTLS
			}
			listenerProtocol = elbv2.ProtocolTLS
		}
		tgName := b.targetGroupName(b.service, b.key, port.TargetPort, string(tgProtocol), hc)
		targetGroup, exists := targetGroupMap[port.TargetPort.String()]
		if !exists {
			targetGroup = elbv2.NewTargetGroup(stack, tgName, elbv2.TargetGroupSpec{
				Name:                  tgName,
				TargetType:            elbv2.TargetTypeIP,
				Port:                  int64(port.TargetPort.IntVal),
				Protocol:              tgProtocol,
				HealthCheckConfig:     hc,
				TargetGroupAttributes: tgAttrs,
			})
			targetGroupMap[port.TargetPort.String()] = targetGroup

			var targetType elbv2api.TargetType = elbv2api.TargetTypeIP
			_ = elbv2.NewTargetGroupBindingResource(stack, tgName, elbv2.TargetGroupBindingResourceSpec{
				TargetGroupARN: targetGroup.TargetGroupARN(),
				Template: elbv2.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:    b.service.Namespace,
						Name: tgName,
					},
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetType: &targetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: b.service.Name,
							Port: port.TargetPort,
						},
					},
				},
			})
		}

		var sslPolicy *string = nil
		sslPolicyStr := ""
		if b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixSSLNegotiationPolicy, &sslPolicyStr, b.service.Annotations) {
			sslPolicy = &sslPolicyStr
		}

		certificates := []elbv2.Certificate{}
		if listenerProtocol == elbv2.ProtocolTLS {
			for _, cert := range certificateARNs {
				certificates = append(certificates, elbv2.Certificate{CertificateARN: &cert})
			}
		}

		_ = elbv2.NewListener(stack, strconv.Itoa(int(port.Port)), elbv2.ListenerSpec{
			LoadBalancerARN: lb.LoadBalancerARN(),
			Port:            int64(port.Port),
			Protocol:        listenerProtocol,
			Certificates:    certificates,
			SSLPolicy:       sslPolicy,
			DefaultActions: []elbv2.Action{
				{
					Type: elbv2.ActionTypeForward,
					ForwardConfig: &elbv2.ForwardActionConfig{
						TargetGroups: []elbv2.TargetGroupTuple{
							{
								TargetGroupARN: targetGroup.TargetGroupARN(),
							},
						},
					},
				},
			},
		})
	}
	return nil
}

func (b *nlbBuilder) loadbalancerName(svc *corev1.Service) string {
	name := "a" + strings.Replace(string(svc.UID), "-", "", -1)
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

func (b *nlbBuilder) targetGroupName(svc *corev1.Service, id types.NamespacedName, port intstr.IntOrString, proto string, hc *elbv2.TargetGroupHealthCheckConfig) string {
	uuidHash := md5.New()
	healthCheckProtocol := string(elbv2.ProtocolTCP)
	healthCheckInterval := strconv.FormatInt(DefaultHealthCheckInterval, 10)
	if hc.Protocol != nil {
		healthCheckProtocol = string(*hc.Protocol)
	}
	if hc.IntervalSeconds != nil {
		healthCheckInterval = strconv.FormatInt(*hc.IntervalSeconds, 10)
	}
	_, _ = uuidHash.Write([]byte(svc.UID))
	_, _ = uuidHash.Write([]byte(port.String()))
	_, _ = uuidHash.Write([]byte(proto))
	_, _ = uuidHash.Write([]byte(healthCheckProtocol))
	_, _ = uuidHash.Write([]byte(healthCheckInterval))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", id.Name, id.Namespace, uuid)
}
