package nlb

import (
	"context"
	"crypto/sha256"
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
	"regexp"
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
	healthCheckPortTrafficPort           = "traffic-port"

	resourceIDLoadBalancer = "LoadBalancer"
)

type ModelBuilder interface {
	Build(ctx context.Context, service *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, error)
}

func NewDefaultModelBuilder(clusterName string, subnetsResolver networking.SubnetsResolver, annotationParser annotations.Parser) ModelBuilder {
	return &defaultModelBuilder{
		clusterName:      clusterName,
		annotationParser: annotationParser,
		subnetsResolver:  subnetsResolver,
	}
}

var _ ModelBuilder = &defaultModelBuilder{}

type defaultModelBuilder struct {
	clusterName      string
	annotationParser annotations.Parser
	subnetsResolver  networking.SubnetsResolver
}

func (b *defaultModelBuilder) Build(ctx context.Context, service *corev1.Service) (core.Stack, *elbv2model.LoadBalancer, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(service)))
	task := &defaultModelBuildTask{
		clusterName:      b.clusterName,
		annotationParser: b.annotationParser,
		subnetsResolver:  b.subnetsResolver,

		service: service,
		stack:   stack,

		defaultAccessLogS3Enabled:            false,
		defaultAccessLogsS3Bucket:            "",
		defaultAccessLogsS3Prefix:            "",
		defaultLoadBalancingCrossZoneEnabled: false,
		defaultProxyProtocolV2Enabled:        false,
		defaultHealthCheckProtocol:           elbv2model.ProtocolTCP,
		defaultHealthCheckPort:               healthCheckPortTrafficPort,
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
	clusterName      string
	annotationParser annotations.Parser
	subnetsResolver  networking.SubnetsResolver

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
	err := t.buildModel(ctx)
	return err
}

func (t *defaultModelBuildTask) buildModel(ctx context.Context) error {
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
	spec, err := t.buildLoadBalancerSpec(ctx, scheme, ec2Subnets)
	if err != nil {
		return err
	}
	t.loadBalancer = elbv2model.NewLoadBalancer(t.stack, resourceIDLoadBalancer, spec)
	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerSpec(ctx context.Context, scheme elbv2model.LoadBalancerScheme, ec2Subnets []*ec2.Subnet) (elbv2model.LoadBalancerSpec, error) {
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
	name := t.buildLoadBalancerName(ctx, scheme)
	spec := elbv2model.LoadBalancerSpec{
		Name:                   name,
		Type:                   elbv2model.LoadBalancerTypeNetwork,
		Scheme:                 &scheme,
		IPAddressType:          &ipAddressType,
		SubnetMappings:         subnetMappings,
		LoadBalancerAttributes: lbAttributes,
		Tags:                   tags,
	}
	return spec, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerScheme(_ context.Context) (elbv2model.LoadBalancerScheme, error) {
	scheme := elbv2model.LoadBalancerSchemeInternetFacing
	internal := false
	if _, err := t.annotationParser.ParseBoolAnnotation(annotations.SvcLBSuffixInternal, &internal, t.service.Annotations); err != nil {
		return "", err
	} else if internal {
		scheme = elbv2model.LoadBalancerSchemeInternal
	}
	return scheme, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerTags(_ context.Context) (map[string]string, error) {
	tags := make(map[string]string)
	_, err := t.annotationParser.ParseStringMapAnnotation(annotations.SvcLBSuffixAdditionalTags, &tags, t.service.Annotations)
	if err != nil {
		return nil, err
	}
	return tags, nil
}

func (t *defaultModelBuildTask) buildSubnetMappings(_ context.Context, ec2Subnets []*ec2.Subnet) ([]elbv2model.SubnetMapping, error) {
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

func (t *defaultModelBuildTask) buildLoadBalancerAttributes(_ context.Context) ([]elbv2model.LoadBalancerAttribute, error) {
	var attrs []elbv2model.LoadBalancerAttribute
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

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckPort(_ context.Context) (intstr.IntOrString, error) {
	rawHealthCheckPort := t.defaultHealthCheckPort
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPort, &rawHealthCheckPort, t.service.Annotations)
	if rawHealthCheckPort == t.defaultHealthCheckPort {
		return intstr.FromString(rawHealthCheckPort), nil
	}
	var portVal int64
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCPort, &portVal, t.service.Annotations); err != nil {
		return intstr.IntOrString{}, err
	}
	return intstr.FromInt(int(portVal)), nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckProtocol(_ context.Context) (elbv2model.Protocol, error) {
	rawHealthCheckProtocol := string(t.defaultHealthCheckProtocol)
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCProtocol, &rawHealthCheckProtocol, t.service.Annotations)
	switch strings.ToUpper(rawHealthCheckProtocol) {
	case string(elbv2model.ProtocolTCP):
		return elbv2model.ProtocolTCP, nil
	case string(elbv2model.ProtocolHTTP):
		return elbv2model.ProtocolHTTP, nil
	case string(elbv2model.ProtocolHTTPS):
		return elbv2model.ProtocolHTTPS, nil
	default:
		return "", errors.Errorf("Unsupported health check protocol %v", rawHealthCheckProtocol)
	}
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckPath(_ context.Context) *string {
	healthCheckPath := t.defaultHealthCheckPath
	t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixHCPath, &healthCheckPath, t.service.Annotations)
	return &healthCheckPath
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckIntervalSeconds(_ context.Context) (int64, error) {
	intervalSeconds := t.defaultHealthCheckInterval
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCInterval, &intervalSeconds, t.service.Annotations); err != nil {
		return 0, err
	}
	return intervalSeconds, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckTimeoutSeconds(_ context.Context) (int64, error) {
	timeoutSeconds := t.defaultHealthCheckTimeout
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCTimeout, &timeoutSeconds, t.service.Annotations); err != nil {
		return 0, err
	}
	return timeoutSeconds, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckHealthyThresholdCount(_ context.Context) (int64, error) {
	healthyThresholdCount := t.defaultHealthCheckHealthyThreshold
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCHealthyThreshold, &healthyThresholdCount, t.service.Annotations); err != nil {
		return 0, err
	}
	return healthyThresholdCount, nil
}

func (t *defaultModelBuildTask) buildTargetGroupHealthCheckUnhealthyThresholdCount(_ context.Context) (int64, error) {
	unhealthyThresholdCount := t.defaultHealthCheckUnhealthyThreshold
	if _, err := t.annotationParser.ParseInt64Annotation(annotations.SvcLBSuffixHCUnhealthyThreshold, &unhealthyThresholdCount, t.service.Annotations); err != nil {
		return 0, err
	}
	return unhealthyThresholdCount, nil
}

func (t *defaultModelBuildTask) buildTargetHealthCheck(ctx context.Context) (*elbv2model.TargetGroupHealthCheckConfig, error) {
	healthCheckProtocol, err := t.buildTargetGroupHealthCheckProtocol(ctx)
	if err != nil {
		return nil, err
	}
	var healthCheckPathPtr *string
	if healthCheckProtocol != elbv2model.ProtocolTCP {
		healthCheckPathPtr = t.buildTargetGroupHealthCheckPath(ctx)
	}
	healthCheckPort, err := t.buildTargetGroupHealthCheckPort(ctx)
	if err != nil {
		return nil, err
	}
	intervalSeconds, err := t.buildTargetGroupHealthCheckIntervalSeconds(ctx)
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := t.buildTargetGroupHealthCheckTimeoutSeconds(ctx)
	if err != nil {
		return nil, err
	}
	healthyThresholdCount, err := t.buildTargetGroupHealthCheckHealthyThresholdCount(ctx)
	if err != nil {
		return nil, err
	}
	unhealthyThresholdCount, err := t.buildTargetGroupHealthCheckUnhealthyThresholdCount(ctx)
	if err != nil {
		return nil, err
	}
	return &elbv2model.TargetGroupHealthCheckConfig{
		Port:                    &healthCheckPort,
		Protocol:                &healthCheckProtocol,
		Path:                    healthCheckPathPtr,
		IntervalSeconds:         &intervalSeconds,
		TimeoutSeconds:          &timeoutSeconds,
		HealthyThresholdCount:   &healthyThresholdCount,
		UnhealthyThresholdCount: &unhealthyThresholdCount,
	}, nil
}

func (t *defaultModelBuildTask) buildTargetGroupAttributes(_ context.Context) ([]elbv2model.TargetGroupAttribute, error) {
	var attrs []elbv2model.TargetGroupAttribute
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
	tgAttrs, err := t.buildTargetGroupAttributes(ctx)
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

		svcPort := intstr.FromInt(int(port.Port))
		tgName := t.buildTargetGroupName(ctx, svcPort, elbv2model.TargetTypeIP, tgProtocol, hc)
		tgResId := t.buildTargetGroupResourceID(k8s.NamespacedName(t.service), svcPort)
		targetGroup, exists := targetGroupMap[port.TargetPort.String()]
		if !exists {
			targetGroup = elbv2model.NewTargetGroup(t.stack, tgResId, elbv2model.TargetGroupSpec{
				Name:                  tgName,
				TargetType:            elbv2model.TargetTypeIP,
				Port:                  int64(port.TargetPort.IntValue()),
				Protocol:              tgProtocol,
				HealthCheckConfig:     hc,
				TargetGroupAttributes: tgAttrs,
			})
			targetGroupMap[port.TargetPort.String()] = targetGroup
			_ = t.buildTargetGroupBinding(ctx, targetGroup, port, hc, ec2Subnets)
		}

		var sslPolicy *string = nil
		sslPolicyStr := ""
		if t.annotationParser.ParseStringAnnotation(annotations.SvcLBSuffixSSLNegotiationPolicy, &sslPolicyStr, t.service.Annotations) {
			sslPolicy = &sslPolicyStr
		}

		var certificates []elbv2model.Certificate
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

func (t *defaultModelBuildTask) buildTargetGroupBinding(ctx context.Context, targetGroup *elbv2model.TargetGroup,
	port corev1.ServicePort, hc *elbv2model.TargetGroupHealthCheckConfig, ec2Subnets []*ec2.Subnet) *elbv2model.TargetGroupBindingResource {
	targetType := elbv2api.TargetTypeIP
	tgbNetworking := t.buildTargetGroupBindingNetworking(ctx, port.TargetPort, *hc.Port, port.Protocol, ec2Subnets)

	return elbv2model.NewTargetGroupBindingResource(t.stack, targetGroup.ID(), elbv2model.TargetGroupBindingResourceSpec{
		Template: elbv2model.TargetGroupBindingTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: t.service.Namespace,
				Name:      targetGroup.Spec.Name,
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

func (t *defaultModelBuildTask) buildTargetGroupBindingNetworking(_ context.Context, tgPort intstr.IntOrString, hcPort intstr.IntOrString,
	tgProtocol corev1.Protocol, ec2Subnets []*ec2.Subnet) *elbv2model.TargetGroupBindingNetworking {
	var from []elbv2model.NetworkingPeer
	networkingProtocol := elbv2api.NetworkingProtocolTCP
	if tgProtocol == corev1.ProtocolUDP {
		networkingProtocol = elbv2api.NetworkingProtocolUDP
	}
	for _, subnet := range ec2Subnets {
		from = append(from, elbv2model.NetworkingPeer{
			IPBlock: &elbv2api.IPBlock{
				CIDR: aws.StringValue(subnet.CidrBlock),
			},
		})
	}
	ports := []elbv2api.NetworkingPort{
		{
			Port:     &tgPort,
			Protocol: &networkingProtocol,
		},
	}
	if hcPort.String() != healthCheckPortTrafficPort && hcPort.IntValue() != tgPort.IntValue() {
		networkingProtocolTCP := elbv2api.NetworkingProtocolTCP
		ports = append(ports, elbv2api.NetworkingPort{
			Port:     &hcPort,
			Protocol: &networkingProtocolTCP,
		})
	}
	tgbNetworking := &elbv2model.TargetGroupBindingNetworking{
		Ingress: []elbv2model.NetworkingIngressRule{
			{
				From:  from,
				Ports: ports,
			},
		},
	}
	return tgbNetworking
}

var invalidLoadBalancerNamePattern = regexp.MustCompile("[[:^alnum:]]")

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context, scheme elbv2model.LoadBalancerScheme) string {
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.service.UID))
	_, _ = uuidHash.Write([]byte(scheme))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidLoadBalancerNamePattern.ReplaceAllString(t.service.Namespace, "")
	sanitizedName := invalidLoadBalancerNamePattern.ReplaceAllString(t.service.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

var invalidTargetGroupNamePattern = regexp.MustCompile("[[:^alnum:]]")

func (t *defaultModelBuildTask) buildTargetGroupName(_ context.Context, port intstr.IntOrString, targetType elbv2model.TargetType, tgProtocol elbv2model.Protocol, hc *elbv2model.TargetGroupHealthCheckConfig) string {
	healthCheckProtocol := string(elbv2model.ProtocolTCP)
	healthCheckInterval := strconv.FormatInt(t.defaultHealthCheckInterval, 10)
	if hc.Protocol != nil {
		healthCheckProtocol = string(*hc.Protocol)
	}
	if hc.IntervalSeconds != nil {
		healthCheckInterval = strconv.FormatInt(*hc.IntervalSeconds, 10)
	}
	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(t.clusterName))
	_, _ = uuidHash.Write([]byte(t.service.UID))
	_, _ = uuidHash.Write([]byte(port.String()))
	_, _ = uuidHash.Write([]byte(targetType))
	_, _ = uuidHash.Write([]byte(tgProtocol))
	_, _ = uuidHash.Write([]byte(healthCheckProtocol))
	_, _ = uuidHash.Write([]byte(healthCheckInterval))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidTargetGroupNamePattern.ReplaceAllString(t.service.Namespace, "")
	sanitizedName := invalidTargetGroupNamePattern.ReplaceAllString(t.service.Name, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", sanitizedNamespace, sanitizedName, uuid)
}

func (t *defaultModelBuildTask) buildTargetGroupResourceID(svcKey types.NamespacedName, port intstr.IntOrString) string {
	return fmt.Sprintf("%s/%s:%s", svcKey.Namespace, svcKey.Name, port.String())
}
