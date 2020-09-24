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
)

type ModelBuilder interface {
	Build(ctx context.Context, service *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, error)
}

func NewDefaultModelBuilder(subnetsResolver networking.SubnetsResolver, annotationParser annotations.Parser) ModelBuilder {
	return &defaultModelBuilder{
		annotationParser: annotationParser,
		subnetsResolver:  subnetsResolver,
	}
}

var _ ModelBuilder = &defaultModelBuilder{}

type defaultModelBuilder struct {
	annotationParser annotations.Parser
	subnetsResolver  networking.SubnetsResolver
}

func (b *defaultModelBuilder) Build(ctx context.Context, service *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack := core.NewDefaultStack(k8s.NamespacedName(service).String())
	task := &defaultModelBuildTask{
		annotationParser: b.annotationParser,
		subnetsResolver:  b.subnetsResolver,
		key:              k8s.NamespacedName(service),
		service:          service,
		stack:            stack,

		defaultAccessLogS3Enabled:            false,
		defaultAccessLogsS3Bucket:            "",
		defaultAccessLogsS3Prefix:            "",
		defaultLoadBalancingCrossZoneEnabled: false,
		defaultProxyProtocolV2Enabled:        false,
		defaultHealthCheckProtocol:           elbv2model.ProtocolTCP,
		defaultHealthCheckPort:               "traffic-port",
		defaultHealthCheckPath:               "/",
		defaultHealthCheckInterval:           10,
		defaultHealthCheckTimeout:            10,
		defaultHealthCheckHealthyThreshold:   3,
		defaultHealthCheckUnhealthyThreshold: 3,
	}
	if err := task.run(ctx); err != nil {
		return nil, nil, err
	}
	return task.stack, task.loadBalancer, nil
}

type defaultModelBuildTask struct {
	annotationParser annotations.Parser
	subnetsResolver  networking.SubnetsResolver

	key     types.NamespacedName
	service *corev1.Service

	stack        core.Stack
	loadBalancer *elbv2model.LoadBalancer

	defaultAccessLogS3Enabled            bool
	defaultAccessLogsS3Bucket            string
	defaultAccessLogsS3Prefix            string
	defaultLoadBalancingCrossZoneEnabled bool
	defaultProxyProtocolV2Enabled        bool
	defaultHealthCheckProtocol           elbv2model.Protocol
	defaultHealthCheckPort               string
	defaultHealthCheckPath               string
	defaultHealthCheckInterval           int64
	defaultHealthCheckTimeout            int64
	defaultHealthCheckHealthyThreshold   int64
	defaultHealthCheckUnhealthyThreshold int64
}

func (t *defaultModelBuildTask) run(ctx context.Context) error {
	if !t.service.DeletionTimestamp.IsZero() {
		return nil
	}
	err := t.buildModel(ctx, t.stack)
	return err
}

func (t *defaultModelBuildTask) buildModel(ctx context.Context, stack core.Stack) error {
	scheme, err := t.buildLoadBalancerScheme(ctx)
	if err != nil {
		return err
	}
	ec2Subnets, err := t.subnetsResolver.DiscoverSubnets(ctx, scheme)
	if err != nil {
		return err
	}
	err = t.buildLoadBalancer(ctx, scheme, ec2Subnets)
	if err != nil {
		return err
	}
	err = t.buildListeners(ctx, ec2Subnets)
	if err != nil {
		return err
	}
	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancer(ctx context.Context, scheme elbv2model.LoadBalancerScheme, ec2Subnets []*ec2.Subnet) error {
	spec, err := t.loadBalancerSpec(ctx, scheme, ec2Subnets)
	if err != nil {
		return err
	}
	t.loadBalancer = elbv2model.NewLoadBalancer(t.stack, k8s.NamespacedName(t.service).String(), spec)
	return nil
}

func (t *defaultModelBuildTask) loadBalancerSpec(ctx context.Context, scheme elbv2model.LoadBalancerScheme, ec2Subnets []*ec2.Subnet) (elbv2model.LoadBalancerSpec, error) {
	ipAddressType := elbv2model.IPAddressTypeIPV4
	lbAttributes, err := t.buildLoadBalancerAttributes(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	tags, err := t.buildLoadBalancerTags(ctx)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	subnetMappings, err := t.buildSubnetMappings(ctx, ec2Subnets)
	if err != nil {
		return elbv2model.LoadBalancerSpec{}, err
	}
	spec := elbv2model.LoadBalancerSpec{
		Name:                   t.loadbalancerName(t.service),
		Type:                   elbv2model.LoadBalancerTypeNetwork,
		Scheme:                 &scheme,
		IPAddressType:          &ipAddressType,
		SubnetMappings:         subnetMappings,
		LoadBalancerAttributes: lbAttributes,
		Tags:                   tags,
	}
	return spec, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerScheme(ctx context.Context) (elbv2model.LoadBalancerScheme, error) {
	scheme := elbv2model.LoadBalancerSchemeInternetFacing
	internal := false
	if _, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixInternal, &internal, t.service.Annotations); err != nil {
		return "", err
	} else if internal {
		scheme = elbv2model.LoadBalancerSchemeInternal
	}
	return scheme, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerTags(ctx context.Context) (map[string]string, error) {
	tags := make(map[string]string)
	_, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixAdditionalTags, &tags, t.service.Annotations)
	if err != nil {
		return nil, err
	}
	return tags, nil
}

func (t *defaultModelBuildTask) buildSubnetMappings(ctx context.Context, ec2Subnets []*ec2.Subnet) ([]elbv2model.SubnetMapping, error) {
	if len(ec2Subnets) == 0 {
		return []elbv2model.SubnetMapping{}, errors.New("Unable to discover at least one subnet across availability zones")
	}
	var eipAllocation []string
	eipConfigured := t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixEIPAllocations, &eipAllocation, t.service.Annotations)
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

func (t *defaultModelBuildTask) buildLoadBalancerAttributes(ctx context.Context) ([]elbv2model.LoadBalancerAttribute, error) {
	attrs := []elbv2model.LoadBalancerAttribute{}
	accessLogEnabled := t.defaultAccessLogS3Enabled
	bucketName := t.defaultAccessLogsS3Bucket
	bucketPrefix := t.defaultAccessLogsS3Prefix
	if _, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixAccessLogEnabled, &accessLogEnabled, t.service.Annotations); err != nil {
		return attrs, err
	}
	if accessLogEnabled {
		t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixAccessLogS3BucketName, &bucketName, t.service.Annotations)
		t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixAccessLogS3BucketPrefix, &bucketPrefix, t.service.Annotations)
	}
	crossZoneEnabled := t.defaultLoadBalancingCrossZoneEnabled
	if _, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixCrossZoneLoadBalancingEnabled, &crossZoneEnabled, t.service.Annotations); err != nil {
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

func (t *defaultModelBuildTask) buildTargetHealthCheck(ctx context.Context) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	hc := elbv2model.TargetGroupHealthCheckConfig{}
	protocol := string(t.defaultHealthCheckProtocol)
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCProtocol, &protocol, t.service.Annotations)
	protocol = strings.ToUpper(protocol)
	hc.Protocol = (*elbv2model.Protocol)(&protocol)

	path := t.defaultHealthCheckPath
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPath, &path, t.service.Annotations)
	if protocol != string(elbv2model.ProtocolTCP) {
		hc.Path = &path
	}

	healthCheckPort := intstr.FromString(t.defaultHealthCheckPort)
	portAnnotationStr := t.defaultHealthCheckPort
	if t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPort, &portAnnotationStr, t.service.Annotations); portAnnotationStr != t.defaultHealthCheckPort {
		var portVal int64
		if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCPort, &portVal, t.service.Annotations); err != nil {
			return nil, err
		}
		healthCheckPort = intstr.FromInt(int(portVal))
	}
	hc.Port = &healthCheckPort

	intervalSeconds := int64(t.defaultHealthCheckInterval)
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCInterval, &intervalSeconds, t.service.Annotations); err != nil {
		return nil, err
	}
	hc.IntervalSeconds = &intervalSeconds

	timeoutSeconds := int64(t.defaultHealthCheckTimeout)
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCTimeout, &timeoutSeconds, t.service.Annotations); err != nil {
		return nil, err
	}
	hc.TimeoutSeconds = &timeoutSeconds

	healthyThreshold := int64(t.defaultHealthCheckHealthyThreshold)
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCHealthyThreshold, &healthyThreshold, t.service.Annotations); err != nil {
		return nil, err
	}
	hc.HealthyThresholdCount = &healthyThreshold

	unhealthyThreshold := int64(t.defaultHealthCheckUnhealthyThreshold)
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCUnhealthyThreshold, &unhealthyThreshold, t.service.Annotations); err != nil {
		return nil, err
	}
	hc.UnhealthyThresholdCount = &unhealthyThreshold

	return &hc, nil
}

func (t *defaultModelBuildTask) targetGroupAttrs(ctx context.Context) ([]elbv2model.TargetGroupAttribute, error) {
	attrs := []elbv2model.TargetGroupAttribute{}
	proxyV2Enabled := t.defaultProxyProtocolV2Enabled
	proxyV2Annotation := ""
	if t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixProxyProtocol, &proxyV2Annotation, t.service.Annotations) {
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

func (t *defaultModelBuildTask) buildListeners(ctx context.Context, ec2Subnets []*ec2.Subnet) error {
	tgAttrs, err := t.targetGroupAttrs(ctx)
	if err != nil {
		return errors.Wrapf(err, "Unable to build target group attributes")
	}

	var certificateARNs []string
	t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLCertificate, &certificateARNs, t.service.Annotations)

	var sslPorts []string
	t.annotationParser.ParseStringSliceAnnotation(annotations.SvcLBSuffixSSLPorts, &sslPorts, t.service.Annotations)
	sslPortsSet := sets.NewString(sslPorts...)

	backendProtocol := ""
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixBEProtocol, &backendProtocol, t.service.Annotations)

	targetGroupMap := map[string]*elbv2model.TargetGroup{}

	for _, port := range t.service.Spec.Ports {
		hc, err := t.buildTargetHealthCheck(ctx)
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
		tgName := t.targetGroupName(t.service, t.key, port.TargetPort, string(tgProtocol), hc)
		targetGroup, exists := targetGroupMap[port.TargetPort.String()]
		if !exists {
			targetGroup = elbv2model.NewTargetGroup(t.stack, tgName, elbv2model.TargetGroupSpec{
				Name:                  tgName,
				TargetType:            elbv2model.TargetTypeIP,
				Port:                  int64(port.TargetPort.IntValue()),
				Protocol:              tgProtocol,
				HealthCheckConfig:     hc,
				TargetGroupAttributes: tgAttrs,
			})
			targetGroupMap[port.TargetPort.String()] = targetGroup

			var targetType elbv2api.TargetType = elbv2api.TargetTypeIP
			tgbNetworking := t.buildTargetGroupBindingNetworking(ctx, port.TargetPort, tgProtocol, ec2Subnets)
			if err != nil {
				return err
			}
			_ = elbv2model.NewTargetGroupBindingResource(t.stack, tgName, elbv2model.TargetGroupBindingResourceSpec{
				Template: elbv2model.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: t.service.Namespace,
						Name:      tgName,
					},
					Spec: elbv2model.TargetGroupBindingSpec{
						TargetGroupARN: targetGroup.TargetGroupARN(),
						TargetType:     &targetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: t.service.Name,
							Port: intstr.FromInt(int(port.Port)),
						},
						Networking: tgbNetworking,
					},
				},
			})
		}

		var sslPolicy *string = nil
		sslPolicyStr := ""
		if t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixSSLNegotiationPolicy, &sslPolicyStr, t.service.Annotations) {
			sslPolicy = &sslPolicyStr
		}

		certificates := []elbv2model.Certificate{}
		if listenerProtocol == elbv2model.ProtocolTLS {
			for _, cert := range certificateARNs {
				certificates = append(certificates, elbv2model.Certificate{CertificateARN: &cert})
			}
		}

		_ = elbv2model.NewListener(t.stack, strconv.Itoa(int(port.Port)), elbv2model.ListenerSpec{
			LoadBalancerARN: t.loadBalancer.LoadBalancerARN(),
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

func (t *defaultModelBuildTask) buildTargetGroupBindingNetworking(_ context.Context, tgPort intstr.IntOrString, tgProtocol elbv2model.Protocol, ec2Subnets []*ec2.Subnet) *elbv2model.TargetGroupBindingNetworking {
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

func (t *defaultModelBuildTask) loadbalancerName(svc *corev1.Service) string {
	name := "a" + strings.Replace(string(svc.UID), "-", "", -1)
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

func (t *defaultModelBuildTask) targetGroupName(svc *corev1.Service, id types.NamespacedName, port intstr.IntOrString, proto string, hc *elbv2model.TargetGroupHealthCheckConfig) string {
	uuidHash := md5.New()
	healthCheckProtocol := string(elbv2model.ProtocolTCP)
	healthCheckInterval := strconv.FormatInt(t.defaultHealthCheckInterval, 10)
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
