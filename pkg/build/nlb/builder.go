package nlb

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/build"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sort"
	"strconv"
	"strings"
)

type Builder interface {
	Build(ctx context.Context) (build.LoadBalancingStack, error)
}

type ServiceBuilder struct {
	cloud            cloud.Cloud
	cache            cache.Cache
	service          *corev1.Service
	key              types.NamespacedName
	annotationParser k8s.AnnotationParser
}

func NewServiceBuilder(cloud cloud.Cloud, cache cache.Cache, service *corev1.Service, key types.NamespacedName, annotationParser k8s.AnnotationParser) Builder {
	return &ServiceBuilder{
		cloud:            cloud,
		cache:            cache,
		service:          service,
		key:              key,
		annotationParser: annotationParser,
	}
}

func (b *ServiceBuilder) Build(ctx context.Context) (build.LoadBalancingStack, error) {
	lbType := ""
	_ = b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerType, &lbType, b.service.Annotations)
	stack := build.LoadBalancingStack{
		ID: b.key.String(),
	}
	if !b.service.DeletionTimestamp.IsZero() {
		return stack, nil
	}
	lbSpec, err := b.loadBalancerSpec(ctx, b.service)
	if err != nil {
		return build.LoadBalancingStack{}, err
	}

	listeners, err := b.loadBalancerListeners(ctx, &stack)
	if err != nil {
		return build.LoadBalancingStack{}, err
	}

	lbSpec.Listeners = listeners

	lb := &api.LoadBalancer{
		ObjectMeta: v1.ObjectMeta{
			Name: build.ResourceIDLoadBalancer,
		},
		Spec: lbSpec,
	}

	stack.LoadBalancer = lb
	return stack, nil
}

func (b *ServiceBuilder) loadBalancerSpec(ctx context.Context, svc *corev1.Service) (api.LoadBalancerSpec, error) {
	ipAddressType := api.IPAddressTypeIPV4
	schema := api.LoadBalancerSchemaInternetFacing
	internal := "false"
	if b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerInternal, &internal, svc.Annotations) {
		if val, err := strconv.ParseBool(internal); err != nil {
			return api.LoadBalancerSpec{}, errors.Errorf("Invalid annotation value %v=%v", k8s.ServiceAnnotationLoadBalancerInternal, internal)
		} else if val {
			schema = string(api.LoadBalancerSchemaInternal)
		}
	}
	lbAttributes, err := b.buildLBAttributes(ctx, svc)
	if err != nil {
		return api.LoadBalancerSpec{}, err
	}
	subnets, err := b.discoverSubnets(ctx, api.LoadBalancerSchema(schema))
	if err != nil {
		return api.LoadBalancerSpec{}, err
	}
	tags := map[string]string{}
	b.annotationParser.ParseStringMapAnnotation(k8s.ServiceAnnotationLoadBalancerAdditionalTags, &tags, svc.Annotations)
	lbSpec := api.LoadBalancerSpec{
		LoadBalancerName: b.loadbalancerName(svc),
		LoadBalancerType: elbv2.LoadBalancerTypeEnumNetwork,
		IPAddressType:    ipAddressType,
		Schema:           api.LoadBalancerSchema(schema),
		SubnetMappings:   buildSubnetMappingFromSubnets(subnets),
		Attributes:       lbAttributes,
		Tags:             tags,
	}
	return lbSpec, nil
}

func (b *ServiceBuilder) buildLBAttributes(ctx context.Context, svc *corev1.Service) (api.LoadBalancerAttributes, error) {
	attrs := map[string]string{}
	enabled := ""
	if b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerAccessLogEnabled, &enabled, svc.Annotations) {
		if val, err := strconv.ParseBool(enabled); err != nil {
			return api.LoadBalancerAttributes{}, fmt.Errorf("Invalid value %v=%v", k8s.ServiceAnnotationLoadBalancerAccessLogEnabled, enabled)
		} else if val {
			bucketName := ""
			bucketPrefix := ""
			b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerAccessLogS3BucketName, &bucketName, svc.Annotations)
			b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerAccessLogS3BucketPrefix, &bucketPrefix, svc.Annotations)
			attrs[build.AccessLogsS3EnabledKey] = enabled
			attrs[build.AccessLogsS3BucketKey] = bucketName
			attrs[build.AccessLogsS3PrefixKey] = bucketPrefix
		}

	}
	if b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerCrossZoneLoadBalancingEnabled, &enabled, svc.Annotations) {
		if _, err := strconv.ParseBool(enabled); err != nil {
			return api.LoadBalancerAttributes{}, fmt.Errorf("Invalid value %v=%v", k8s.ServiceAnnotationLoadBalancerCrossZoneLoadBalancingEnabled, enabled)
		}
		attrs[build.CrossZoneLoadBalancing] = enabled
	}
	elbv2Attrs := make([]*elbv2.LoadBalancerAttribute, 0, len(attrs))
	for k, v := range attrs {
		elbv2Attrs = append(elbv2Attrs, &elbv2.LoadBalancerAttribute{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	lbAttributes, unknown, err := build.ParseLoadBalancerAttributes(elbv2Attrs)
	if err != nil {
		return api.LoadBalancerAttributes{}, err
	}
	if len(unknown) != 0 {
		return api.LoadBalancerAttributes{}, errors.Errorf("unknown load balancer attributes: %v", unknown)
	}
	return lbAttributes, nil
}

func (b *ServiceBuilder) buildTGHealthCheck(ctx context.Context, port intstr.IntOrString, annotations map[string]string) (api.HealthCheckConfig, error) {
	protocol := "TCP"
	b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerHealthCheckProtocol, &protocol, annotations)
	var portVal int64
	if val, err := b.annotationParser.ParseInt64Annotation(k8s.ServiceAnnotationLoadBalancerHealthCheckPort, &portVal, annotations); err != nil {
		return api.HealthCheckConfig{}, err
	} else if val {
		port.IntVal = int32(portVal)
	}
	path := "/"
	b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerHealthCheckPath, &path, annotations)
	if protocol == "TCP" {
		path = ""
	}
	intervalSeconds := int64(10)
	if _, err := b.annotationParser.ParseInt64Annotation(k8s.ServiceAnnotationLoadBalancerHCInterval, &intervalSeconds, annotations); err != nil {
		return api.HealthCheckConfig{}, nil
	}
	timeoutSeconds := int64(10)
	if _, err := b.annotationParser.ParseInt64Annotation(k8s.ServiceAnnotationLoadBalancerHCTimeout, &timeoutSeconds, annotations); err != nil {
		return api.HealthCheckConfig{}, nil
	}
	healthyThreshold := int64(3)
	if _, err := b.annotationParser.ParseInt64Annotation(k8s.ServiceAnnotationLoadBalancerHCHealthyThreshold, &healthyThreshold, annotations); err != nil {
		return api.HealthCheckConfig{}, nil
	}
	unhealthyThreshold := int64(3)
	if _, err := b.annotationParser.ParseInt64Annotation(k8s.ServiceAnnotationLoadBalancerHCUnhealthyThreshold, &unhealthyThreshold, annotations); err != nil {
		return api.HealthCheckConfig{}, nil
	}

	return api.HealthCheckConfig{
		Port:                    port,
		Protocol:                api.Protocol(protocol),
		Path:                    path,
		IntervalSeconds:         intervalSeconds,
		TimeoutSeconds:          timeoutSeconds,
		HealthyThresholdCount:   healthyThreshold,
		UnhealthyThresholdCount: unhealthyThreshold,
	}, nil
}

func (b *ServiceBuilder) loadBalancerListeners(ctx context.Context, stack *build.LoadBalancingStack) ([]api.Listener, error) {
	listeners := make([]api.Listener, 0, len(b.service.Spec.Ports))
	proxyProtocolV2 := false
	proxyV2Value := ""
	if b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerProxyProtocol, &proxyV2Value, b.service.Annotations) {
		if proxyV2Value != "*" {
			return []api.Listener{}, errors.Errorf("Invalid value %v for %v, only value currently supported is *", proxyV2Value, k8s.ServiceAnnotationLoadBalancerProxyProtocol)
		}
		proxyProtocolV2 = true
	}
	var sslPorts []string
	b.annotationParser.ParseStringSliceAnnotation(k8s.ServiceAnnotationLoadBalancerSSLPorts, &sslPorts, b.service.Annotations)
	sslPortsSet := sets.NewString(sslPorts...)
	var certificateARNs []string
	b.annotationParser.ParseStringSliceAnnotation(k8s.ServiceAnnotationLoadBalancerCertificate, &certificateARNs, b.service.Annotations)
	for _, port := range b.service.Spec.Ports {
		// Build target group, then add it to the listener
		// TODO: Search stack for existing target group
		hc, err := b.buildTGHealthCheck(ctx, port.TargetPort, b.service.Annotations)
		tgProtocol := api.ProtocolTCP
		listenerProtocol := api.ProtocolTCP
		if err != nil {
			return []api.Listener{}, err
		}
		backendProtocol := ""
		b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerBEProtocol, &backendProtocol, b.service.Annotations)
		if certificateARNs != nil && (sslPortsSet == nil || sslPortsSet.Has(port.Name) || sslPortsSet.Has(strconv.Itoa(int(port.Port)))) {
			if backendProtocol == "ssl" {
				tgProtocol = api.ProtocolTLS
			}
			listenerProtocol = api.ProtocolTLS
		}
		tgName := b.targetGroupName(b.service, b.key, port.TargetPort, tgProtocol, hc)
		tgSpec := api.TargetGroupSpec{
			TargetGroupName:   tgName,
			TargetType:        elbv2.TargetTypeEnumIp,
			Port:              int64(port.TargetPort.IntVal),
			Protocol:          api.Protocol(tgProtocol),
			HealthCheckConfig: hc,
			Attributes: api.TargetGroupAttributes{
				Stickiness:      api.TargetGroupStickinessAttributes{Enabled: false, Type: api.TargetGroupStickinessTypeSourceIP},
				ProxyProtocolV2: api.TargetGroupProxyProtocolV2{Enabled: proxyProtocolV2},
			},
		}
		tg := &api.TargetGroup{
			ObjectMeta: v1.ObjectMeta{
				Name: tgName,
			},
			Spec: tgSpec,
		}
		stack.AddTargetGroup(tg)

		eb := &api.EndpointBinding{
			ObjectMeta: v1.ObjectMeta{
				Name: stack.ID + ":" + tgName,
			},
			Spec: api.EndpointBindingSpec{
				TargetGroup: api.TargetGroupReference{
					TargetGroupRef: k8s.LocalObjectReference(tg),
				},
				TargetType:  tgSpec.TargetType,
				ServiceRef:  k8s.ObjectReference(b.service),
				ServicePort: port.TargetPort,
			},
		}
		stack.AddEndpointBinding(eb)

		defaultActions := []api.ListenerAction{b.buildForwardAction(api.TargetGroupReference{TargetGroupRef: k8s.LocalObjectReference(tg)})}

		ls := api.Listener{
			Port:           int64(port.TargetPort.IntVal),
			Protocol:       api.Protocol(listenerProtocol),
			DefaultActions: defaultActions,
		}
		if listenerProtocol == api.ProtocolTLS {
			sslPolicy := build.DefaultSSLPolicy
			b.annotationParser.ParseStringAnnotation(k8s.ServiceAnnotationLoadBalancerSSLNegotiationPolicy, &sslPolicy, b.service.Annotations)
			ls.SSLPolicy = sslPolicy
			ls.Certificates = certificateARNs
		}
		listeners = append(listeners, ls)
	}

	return listeners, nil
}

func (b *ServiceBuilder) loadbalancerName(svc *corev1.Service) string {
	name := "a" + strings.Replace(string(svc.UID), "-", "", -1)
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

func (b *ServiceBuilder) targetGroupName(svc *corev1.Service, id types.NamespacedName, port intstr.IntOrString, proto string, hc api.HealthCheckConfig) string {
	uuidHash := md5.New()
	_, _ = uuidHash.Write([]byte(svc.UID))
	_, _ = uuidHash.Write([]byte(port.String()))
	_, _ = uuidHash.Write([]byte(proto))
	_, _ = uuidHash.Write([]byte(hc.Protocol))
	_, _ = uuidHash.Write([]byte(strconv.FormatInt(hc.IntervalSeconds, 10)))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))
	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", id.Name, id.Namespace, uuid)
}

func (b *ServiceBuilder) buildForwardAction(tgRef api.TargetGroupReference) api.ListenerAction {
	return api.ListenerAction{
		Type: api.ListenerActionTypeForward,
		Forward: &api.ForwardConfig{
			TargetGroup: tgRef,
		},
	}
}

func (b *ServiceBuilder) discoverSubnets(ctx context.Context, schema api.LoadBalancerSchema) ([]string, error) {
	subnetRoleTagKey := ""
	switch schema {
	case api.LoadBalancerSchemaInternetFacing:
		subnetRoleTagKey = build.TagKeySubnetPublicELB
	case api.LoadBalancerSchemaInternal:
		subnetRoleTagKey = build.TagKeySubnetInternalELB
	}
	clusterResourceTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", b.cloud.ClusterName())
	tagFilters := map[string][]string{subnetRoleTagKey: {"", "1"}, clusterResourceTagKey: {"owned", "shared"}}
	resources, err := b.cloud.RGT().GetResourcesAsList(ctx, &resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: aws.StringSlice([]string{cloud.ResourceTypeEC2Subnet}),
		TagFilters:          cloud.NewRGTTagFiltersV2(tagFilters),
	})
	if err != nil {
		return nil, err
	}

	subnetIDs := make([]string, 0, len(resources))
	for _, resource := range resources {
		subnetARN, _ := arn.Parse(aws.StringValue(resource.ResourceARN))
		parts := strings.Split(subnetARN.Resource, "/")
		subnetID := parts[len(parts)-1]
		subnetIDs = append(subnetIDs, subnetID)
	}
	subnetIDs, err = b.chooseSubnets(ctx, subnetIDs)
	if err != nil {
		logging.FromContext(ctx).Info("failed to discover subnets", "tagFilters", tagFilters)
		return nil, errors.Wrapf(err, "failed to discover subnets")
	}

	return subnetIDs, nil
}

func (b *ServiceBuilder) chooseSubnets(ctx context.Context, subnetIDOrNames []string) ([]string, error) {
	subnets, err := b.cloud.EC2().GetSubnetsByNameOrID(ctx, subnetIDOrNames)
	if err != nil {
		return nil, err
	}
	subnetsByAZ := map[string][]string{}
	for _, subnet := range subnets {
		subnetVpcID := aws.StringValue(subnet.VpcId)
		clusterVpcID := b.cloud.VpcID()
		if subnetVpcID != clusterVpcID {
			logging.FromContext(ctx).Info("ignore subnet", "subnetVPC", subnetVpcID, "clusterVPC", clusterVpcID)
			continue
		}
		subnetAZ := aws.StringValue(subnet.AvailabilityZone)
		subnetsByAZ[subnetAZ] = append(subnetsByAZ[subnetAZ], aws.StringValue(subnet.SubnetId))
	}

	chosenSubnets := make([]string, 0, len(subnetsByAZ))
	for az, subnets := range subnetsByAZ {
		if len(subnets) == 1 {
			chosenSubnets = append(chosenSubnets, subnets[0])
		} else if len(subnets) > 1 {
			sort.Strings(subnets)
			logging.FromContext(ctx).Info("multiple subnet same AvailabilityZone", "AvailabilityZone", az,
				"chosen", subnets[0], "ignored", subnets[1:])
			chosenSubnets = append(chosenSubnets, subnets[0])
		}
	}
	if len(chosenSubnets) < 2 {
		return nil, errors.Errorf("requires at least 2 subnet in different AvailabilityZone")
	}

	return chosenSubnets, nil
}

func buildSubnetMappingFromSubnets(subnets []string) []api.SubnetMapping {
	var subnetMappings []api.SubnetMapping
	for _, subnet := range subnets {
		subnetMappings = append(subnetMappings, api.SubnetMapping{
			SubnetID: subnet,
		})
	}
	return subnetMappings
}
