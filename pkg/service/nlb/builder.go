package nlb

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
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
	DefaultHealthCheckProtocol           = elbv2model.ProtocolTCP
	DefaultHealthCheckPort               = "traffic-port"
	DefaultHealthCheckPath               = "/"
	DefaultHealthCheckInterval           = 10
	DefaultHealthCheckTimeout            = 10
	DefaultHealthCheckHealthyThreshold   = 3
	DefaultHealthCheckUnhealthyThreshold = 3
)

type ModelBuilder interface {
	Build(ctx context.Context) (core.Stack, *elbv2model.LoadBalancer, error)
}

type nlbBuilder struct {
	service          *corev1.Service
	key              types.NamespacedName
	annotationParser annotations.Parser
	subnetsResolver  networking.SubnetsResolver
}

func NewServiceBuilder(service *corev1.Service, subnetsResolver networking.SubnetsResolver, annotationParser annotations.Parser) ModelBuilder {
	return &nlbBuilder{
		service:          service,
		key:              k8s.NamespacedName(service),
		annotationParser: annotationParser,
		subnetsResolver:  subnetsResolver,
	}
}

func (b *nlbBuilder) Build(ctx context.Context) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack := core.NewDefaultStack(k8s.NamespacedName(b.service).String())
	if !b.service.DeletionTimestamp.IsZero() {
		return stack, nil, nil
	}
	lb, err := b.buildModel(ctx, stack)
	return stack, lb, err
}

func (b *nlbBuilder) buildModel(ctx context.Context, stack core.Stack) (*elbv2model.LoadBalancer, error) {
	if !b.service.DeletionTimestamp.IsZero() {
		return nil, nil
	}
	scheme, err := b.buildLoadBalancerScheme(ctx)
	if err != nil {
		return nil, err
	}
	ec2Subnets, err := b.subnetsResolver.DiscoverSubnets(ctx, scheme)
	if err != nil {
		return nil, err
	}

	nlb, err := b.buildLoadBalancer(ctx, stack, ec2Subnets)
	if err != nil {
		return nil, err
	}
	err = b.buildListeners(ctx, stack, nlb, ec2Subnets)
	if err != nil {
		return nil, err
	}
	return nlb, nil
}

func (b *nlbBuilder) buildLoadBalancer(ctx context.Context, stack core.Stack, ec2Subnets []*ec2.Subnet) (*elbv2model.LoadBalancer, error) {
	spec, err := b.loadBalancerSpec(ctx, ec2Subnets)
	if err != nil {
		return nil, err
	}
	return elbv2model.NewLoadBalancer(stack, k8s.NamespacedName(b.service).String(), spec), nil
}

func (b *nlbBuilder) loadBalancerSpec(ctx context.Context, ec2Subnets []*ec2.Subnet) (elbv2model.LoadBalancerSpec, error) {
	ipAddressType := elbv2model.IPAddressTypeIPV4
	scheme, err := b.buildLoadBalancerScheme(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	lbAttributes, err := b.buildLoadBalancerAttributes(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	tags, err := b.buildLoadBalancerTags(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	subnetMappings, err := b.buildSubnetMappings(ctx, ec2Subnets)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	spec := elbv2model.LoadBalancerSpec{
		Name:                   b.loadbalancerName(b.service),
		Type:                   elbv2model.LoadBalancerTypeNetwork,
		Scheme:                 &scheme,
		IPAddressType:          &ipAddressType,
		SubnetMappings:         subnetMappings,
		LoadBalancerAttributes: lbAttributes,
		Tags:                   tags,
	}
	return spec, nil
}

func (b *nlbBuilder) buildLoadBalancerScheme(ctx context.Context) (elbv2model.LoadBalancerScheme, error) {
	scheme := elbv2model.LoadBalancerSchemeInternetFacing
	internal := false
	if _, err := b.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixInternal, &internal, b.service.Annotations); err != nil {
		return "", err
	} else if internal {
		scheme = elbv2model.LoadBalancerSchemeInternal
	}
	return scheme, nil
}

func (b *nlbBuilder) buildLoadBalancerTags(ctx context.Context) (map[string]string, error) {
	tags := make(map[string]string)
	_, err := b.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixAdditionalTags, &tags, b.service.Annotations)
	if err != nil {
		return nil, err
	}
	return tags, nil
}

func (b *nlbBuilder) buildSubnetMappings(ctx context.Context, ec2Subnets []*ec2.Subnet) ([]elbv2model.SubnetMapping, error) {
	if len(ec2Subnets) == 0 {
		return []elbv2model.SubnetMapping{}, errors.New("Unable to discover at least one subnet across availability zones")
	}
	var eipAllocation []string
	eipConfigured := b.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixEIPAllocations, &eipAllocation, b.service.Annotations)
	if eipConfigured && len(eipAllocation) != len(ec2Subnets) {
		return []elbv2model.SubnetMapping{}, errors.Errorf("Error creating load balancer, number of EIP allocations (%d) and subnets (%d) must match", len(eipAllocation), len(ec2Subnets))
	}
	subnetMappings := make([]elbv2model.SubnetMapping, 0, len(ec2Subnets))
	for idx, subnet := range ec2Subnets {
		mapping := elbv2model.SubnetMapping{
			SubnetID: aws.StringValue(subnet.SubnetId),
		}
		if idx < len(eipAllocation) {
			mapping.AllocationID = aws.String(eipAllocation[idx])
		}
		subnetMappings = append(subnetMappings, mapping)
	}
	return subnetMappings, nil
}

func (b *nlbBuilder) buildLoadBalancerAttributes(ctx context.Context) ([]elbv2model.LoadBalancerAttribute, error) {
	attrs := []elbv2model.LoadBalancerAttribute{}
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
		return []elbv2model.LoadBalancerAttribute{}, err
	}

	attrs = append(attrs, []elbv2model.LoadBalancerAttribute{
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

func (b *nlbBuilder) buildTargetHealthCheck(ctx context.Context) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	hc := elbv2model.TargetGroupHealthCheckConfig{}
	protocol := string(DefaultHealthCheckProtocol)
	b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCProtocol, &protocol, b.service.Annotations)
	protocol = strings.ToUpper(protocol)
	hc.Protocol = (*elbv2model.Protocol)(&protocol)

	path := DefaultHealthCheckPath
	b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPath, &path, b.service.Annotations)
	if protocol != string(elbv2model.ProtocolTCP) {
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

func (b *nlbBuilder) targetGroupAttrs(ctx context.Context) ([]elbv2model.TargetGroupAttribute, error) {
	attrs := []elbv2model.TargetGroupAttribute{}
	proxyV2Enabled := DefaultProxyProtocolV2Enabled
	proxyV2Annotation := ""
	if b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixProxyProtocol, &proxyV2Annotation, b.service.Annotations) {
		if proxyV2Annotation != "*" {
			return []elbv2model.TargetGroupAttribute{}, errors.Errorf("Invalid value %v for Load Balancer proxy protocol v2 annotation, only value currently supported is *", proxyV2Annotation)
		}
		proxyV2Enabled = true
	}
	attrs = append(attrs, elbv2model.TargetGroupAttribute{
		Key:   TGAttrsProxyProtocolV2Enabled,
		Value: strconv.FormatBool(proxyV2Enabled),
	})
	return attrs, nil
}

func (b *nlbBuilder) buildListeners(ctx context.Context, stack core.Stack, lb *elbv2model.LoadBalancer, ec2Subnets []*ec2.Subnet) error {
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

	targetGroupMap := map[string]*elbv2model.TargetGroup{}

	for _, port := range b.service.Spec.Ports {
		hc, err := b.buildTargetHealthCheck(ctx)
		tgProtocol := elbv2model.Protocol(port.Protocol)
		listenerProtocol := elbv2model.Protocol(port.Protocol)
		if err != nil {
			return err
		}

		if tgProtocol != elbv2model.ProtocolUDP && certificateARNs != nil && (sslPortsSet.Len() == 0 || sslPortsSet.Has(port.Name) || sslPortsSet.Has(strconv.Itoa(int(port.Port)))) {
			if backendProtocol == "ssl" {
				tgProtocol = elbv2model.ProtocolTLS
			}
			listenerProtocol = elbv2model.ProtocolTLS
		}
		tgName := b.targetGroupName(b.service, b.key, port.TargetPort, string(tgProtocol), hc)
		targetGroup, exists := targetGroupMap[port.TargetPort.String()]
		if !exists {
			targetGroup = elbv2model.NewTargetGroup(stack, tgName, elbv2model.TargetGroupSpec{
				Name:                  tgName,
				TargetType:            elbv2model.TargetTypeIP,
				Port:                  int64(port.TargetPort.IntValue()),
				Protocol:              tgProtocol,
				HealthCheckConfig:     hc,
				TargetGroupAttributes: tgAttrs,
			})
			targetGroupMap[port.TargetPort.String()] = targetGroup

			var targetType elbv2api.TargetType = elbv2api.TargetTypeIP
			tgbNetworking := b.buildTargetGroupBindingNetworking(ctx, port.TargetPort, tgProtocol, ec2Subnets)
			if err != nil {
				return err
			}
			_ = elbv2model.NewTargetGroupBindingResource(stack, tgName, elbv2model.TargetGroupBindingResourceSpec{
				Template: elbv2model.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: b.service.Namespace,
						Name:      tgName,
					},
					Spec: elbv2model.TargetGroupBindingSpec{
						TargetGroupARN: targetGroup.TargetGroupARN(),
						TargetType:     &targetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: b.service.Name,
							Port: intstr.FromInt(int(port.Port)),
						},
						Networking: tgbNetworking,
					},
				},
			})
		}

		var sslPolicy *string = nil
		sslPolicyStr := ""
		if b.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixSSLNegotiationPolicy, &sslPolicyStr, b.service.Annotations) {
			sslPolicy = &sslPolicyStr
		}

		certificates := []elbv2model.Certificate{}
		if listenerProtocol == elbv2model.ProtocolTLS {
			for _, cert := range certificateARNs {
				certificates = append(certificates, elbv2model.Certificate{CertificateARN: &cert})
			}
		}

		_ = elbv2model.NewListener(stack, strconv.Itoa(int(port.Port)), elbv2model.ListenerSpec{
			LoadBalancerARN: lb.LoadBalancerARN(),
			Port:            int64(port.Port),
			Protocol:        listenerProtocol,
			Certificates:    certificates,
			SSLPolicy:       sslPolicy,
			DefaultActions: []elbv2model.Action{
				{
					Type: elbv2model.ActionTypeForward,
					ForwardConfig: &elbv2model.ForwardActionConfig{
						TargetGroups: []elbv2model.TargetGroupTuple{
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

func (b *nlbBuilder) buildTargetGroupBindingNetworking(_ context.Context, tgPort intstr.IntOrString, tgProtocol elbv2model.Protocol, ec2Subnets []*ec2.Subnet) *elbv2model.TargetGroupBindingNetworking {
	var from []elbv2model.NetworkingPeer
	for _, subnet := range ec2Subnets {
		from = append(from, elbv2model.NetworkingPeer{
			IPBlock: &elbv2api.IPBlock{
				CIDR: aws.StringValue(subnet.CidrBlock),
			},
		})
	}
	tgbNetworking := &elbv2model.TargetGroupBindingNetworking{
		Ingress: []elbv2model.NetworkingIngressRule{
			{
				From: from,
				Ports: []elbv2api.NetworkingPort{{
					Port: &tgPort,
				}},
			},
		},
	}
	return tgbNetworking
}

func (b *nlbBuilder) loadbalancerName(svc *corev1.Service) string {
	name := "a" + strings.Replace(string(svc.UID), "-", "", -1)
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

func (b *nlbBuilder) targetGroupName(svc *corev1.Service, id types.NamespacedName, port intstr.IntOrString, proto string, hc *elbv2model.TargetGroupHealthCheckConfig) string {
	uuidHash := md5.New()
	healthCheckProtocol := string(elbv2model.ProtocolTCP)
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
