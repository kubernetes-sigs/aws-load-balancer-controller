package gateway

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

func Test_defaultModelBuilderTask_targetGroupAttrs(t *testing.T) {
	tests := []struct {
		testName  string
		gateway   *v1beta1.Gateway
		wantError bool
		wantValue []elbv2.TargetGroupAttribute
	}{
		{
			testName: "Default values",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			wantError: false,
			wantValue: []elbv2.TargetGroupAttribute{
				{
					Key:   tgAttrsProxyProtocolV2Enabled,
					Value: "false",
				},
			},
		},
		{
			testName: "Proxy V2 enabled",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-proxy-protocol": "*",
					},
				},
			},
			wantError: false,
			wantValue: []elbv2.TargetGroupAttribute{
				{
					Key:   tgAttrsProxyProtocolV2Enabled,
					Value: "true",
				},
			},
		},
		{
			testName: "Invalid value",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-proxy-protocol": "v2",
					},
				},
			},
			wantError: true,
		},
		{
			testName: "target group attributes",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": "target.group-attr-1=80, t2.enabled=false, preserve_client_ip.enabled=true",
					},
				},
			},
			wantValue: []elbv2.TargetGroupAttribute{
				{
					Key:   tgAttrsProxyProtocolV2Enabled,
					Value: "false",
				},
				{
					Key:   tgAttrsPreserveClientIPEnabled,
					Value: "true",
				},
				{
					Key:   "target.group-attr-1",
					Value: "80",
				},
				{
					Key:   "t2.enabled",
					Value: "false",
				},
			},
			wantError: false,
		},
		{
			testName: "target group proxy v2 override",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": tgAttrsProxyProtocolV2Enabled + "=false",
						"service.beta.kubernetes.io/aws-load-balancer-proxy-protocol":          "*",
					},
				},
			},
			wantValue: []elbv2.TargetGroupAttribute{
				{
					Key:   tgAttrsProxyProtocolV2Enabled,
					Value: "true",
				},
			},
		},
		{
			testName: "target group attr parse error",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": "k1=v1, malformed",
					},
				},
			},
			wantError: true,
		},
		{
			testName: "IP enabled attribute parse error",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": tgAttrsPreserveClientIPEnabled + "= FalSe",
					},
				},
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				gateway:          tt.gateway,
				annotationParser: parser,
			}
			tgAttrs, err := builder.buildTargetGroupAttributes(context.Background())
			if tt.wantError {
				assert.Error(t, err)
			} else {
				sort.Slice(tt.wantValue, func(i, j int) bool {
					return tt.wantValue[i].Key < tt.wantValue[j].Key
				})
				sort.Slice(tgAttrs, func(i, j int) bool {
					return tgAttrs[i].Key < tgAttrs[j].Key
				})
				assert.Equal(t, tt.wantValue, tgAttrs)
			}
		})
	}
}

func Test_defaultModelBuilderTask_buildTargetHealthCheck(t *testing.T) {
	trafficPort := intstr.FromString(healthCheckPortTrafficPort)
	port8888 := intstr.FromInt(8888)
	port31223 := intstr.FromInt(31223)
	tests := []struct {
		testName   string
		gateway    *v1beta1.Gateway
		targetType elbv2.TargetType
		wantError  bool
		wantValue  *elbv2.TargetGroupHealthCheckConfig
	}{
		{
			testName: "Default config",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			wantError: false,
			wantValue: &elbv2.TargetGroupHealthCheckConfig{
				Port:                    &trafficPort,
				Protocol:                (*elbv2.Protocol)(aws.String(string(elbv2.ProtocolTCP))),
				IntervalSeconds:         aws.Int64(10),
				TimeoutSeconds:          aws.Int64(10),
				HealthyThresholdCount:   aws.Int64(3),
				UnhealthyThresholdCount: aws.Int64(3),
			},
			targetType: elbv2.TargetTypeIP,
		},
		{
			testName: "With annotations",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":            "HTTP",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                "8888",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-path":                "/healthz",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "10",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "30",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "2",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "2",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-success-codes":       "200-220,231,250-300,301,302",
					},
				},
			},
			wantError: false,
			wantValue: &elbv2.TargetGroupHealthCheckConfig{
				Port:                    &port8888,
				Protocol:                (*elbv2.Protocol)(aws.String("HTTP")),
				Path:                    aws.String("/healthz"),
				IntervalSeconds:         aws.Int64(10),
				TimeoutSeconds:          aws.Int64(30),
				HealthyThresholdCount:   aws.Int64(2),
				UnhealthyThresholdCount: aws.Int64(2),
				Matcher: &elbv2.HealthCheckMatcher{
					HTTPCode: aws.String("200-220,231,250-300,301,302"),
				},
			},
			targetType: elbv2.TargetTypeInstance,
		},
		{
			testName: "default path and matcher code",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol": "HTTP",
					},
				},
			},
			wantError: false,
			wantValue: &elbv2.TargetGroupHealthCheckConfig{
				Port:                    &trafficPort,
				Protocol:                (*elbv2.Protocol)(aws.String("HTTP")),
				Path:                    aws.String("/"),
				IntervalSeconds:         aws.Int64(10),
				TimeoutSeconds:          aws.Int64(10),
				HealthyThresholdCount:   aws.Int64(3),
				UnhealthyThresholdCount: aws.Int64(3),
				Matcher: &elbv2.HealthCheckMatcher{
					HTTPCode: aws.String("200-399"),
				},
			},
			targetType: elbv2.TargetTypeIP,
		},
		{
			testName: "invalid values",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":            "HTTP",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                "invalid",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "10",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "30",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "2",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "2",
					},
				},
			},
			targetType: elbv2.TargetTypeIP,
			wantError:  true,
		},
		{
			testName: "invalid values target type local, instance mode",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":            "HTTP",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                "invalid",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "10",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "30",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "2",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "2",
					},
				},
				Spec: v1beta1.GatewaySpec{},
			},
			targetType: elbv2.TargetTypeInstance,
			wantError:  true,
		},
		{
			testName: "traffic policy local, target type IP, default healthcheck",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{},
			},
			wantError: false,
			wantValue: &elbv2.TargetGroupHealthCheckConfig{
				Port:                    &trafficPort,
				Protocol:                (*elbv2.Protocol)(aws.String(string(elbv2.ProtocolTCP))),
				IntervalSeconds:         aws.Int64(10),
				TimeoutSeconds:          aws.Int64(10),
				HealthyThresholdCount:   aws.Int64(3),
				UnhealthyThresholdCount: aws.Int64(3),
			},
			targetType: elbv2.TargetTypeIP,
		},
		{
			testName: "traffic policy local, target type Instance, default healthcheck",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{},
			},
			wantError: false,
			wantValue: &elbv2.TargetGroupHealthCheckConfig{
				Port:                    &port31223,
				Protocol:                (*elbv2.Protocol)(aws.String(string(elbv2.ProtocolHTTP))),
				Path:                    aws.String("/healthz"),
				IntervalSeconds:         aws.Int64(10),
				TimeoutSeconds:          aws.Int64(6),
				HealthyThresholdCount:   aws.Int64(2),
				UnhealthyThresholdCount: aws.Int64(2),
				Matcher: &elbv2.HealthCheckMatcher{
					HTTPCode: aws.String("200-399"),
				},
			},
			targetType: elbv2.TargetTypeInstance,
		},
		{
			testName: "traffic policy local, target type Instance, override default",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":            "TCP",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                "8888",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-path":                "/healthz",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "10",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "30",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "5",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "5",
					},
				},
				Spec: v1beta1.GatewaySpec{},
			},
			wantError: false,
			wantValue: &elbv2.TargetGroupHealthCheckConfig{
				Port:                    &port8888,
				Protocol:                (*elbv2.Protocol)(aws.String(string(elbv2.ProtocolTCP))),
				IntervalSeconds:         aws.Int64(10),
				TimeoutSeconds:          aws.Int64(30),
				HealthyThresholdCount:   aws.Int64(5),
				UnhealthyThresholdCount: aws.Int64(5),
			},
			targetType: elbv2.TargetTypeInstance,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				gateway:                              tt.gateway,
				annotationParser:                     parser,
				featureGates:                         config.NewFeatureGates(),
				defaultAccessLogsS3Bucket:            "",
				defaultAccessLogsS3Prefix:            "",
				defaultLoadBalancingCrossZoneEnabled: false,
				defaultProxyProtocolV2Enabled:        false,
				defaultHealthCheckProtocol:           elbv2.ProtocolTCP,
				defaultHealthCheckPort:               healthCheckPortTrafficPort,
				defaultHealthCheckPath:               "/",
				defaultHealthCheckInterval:           10,
				defaultHealthCheckTimeout:            10,
				defaultHealthCheckHealthyThreshold:   3,
				defaultHealthCheckUnhealthyThreshold: 3,
				defaultHealthCheckMatcherHTTPCode:    "200-399",

				defaultHealthCheckProtocolForInstanceModeLocal:           elbv2.ProtocolHTTP,
				defaultHealthCheckPortForInstanceModeLocal:               "8080",
				defaultHealthCheckPathForInstanceModeLocal:               "/healthz",
				defaultHealthCheckIntervalForInstanceModeLocal:           10,
				defaultHealthCheckTimeoutForInstanceModeLocal:            6,
				defaultHealthCheckHealthyThresholdForInstanceModeLocal:   2,
				defaultHealthCheckUnhealthyThresholdForInstanceModeLocal: 2,
			}
			hc, err := builder.buildTargetGroupHealthCheckConfig(context.Background(), tt.targetType)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.wantValue, hc)
			}
		})
	}
}

func Test_defaultModelBuilderTask_buildTargetGroupBindingNetworking(t *testing.T) {
	networkingProtocolTCP := elbv2api.NetworkingProtocolTCP
	networkingProtocolUDP := elbv2api.NetworkingProtocolUDP
	port80 := intstr.FromInt(80)
	port808 := intstr.FromInt(808)
	trafficPort := intstr.FromString("traffic-port")

	tests := []struct {
		name                string
		gateway             *v1beta1.Gateway
		tgPort              intstr.IntOrString
		hcPort              intstr.IntOrString
		subnets             []*ec2.Subnet
		tgProtocol          corev1.Protocol
		ipAddressType       elbv2.TargetGroupIPAddressType
		preserveClientIP    bool
		defaultSourceRanges []string
		want                *elbv2.TargetGroupBindingNetworking
	}{
		{
			name: "udp-service with source ranges",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{},
			},
			tgPort: port80,
			hcPort: trafficPort,
			subnets: []*ec2.Subnet{{
				CidrBlock: aws.String("172.16.0.0/19"),
				SubnetId:  aws.String("az-1"),
			}},
			tgProtocol:    corev1.ProtocolUDP,
			ipAddressType: elbv2.TargetGroupIPAddressTypeIPv4,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "10.0.0.0/16",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolUDP,
								Port:     &port80,
							},
						},
					},
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "172.16.0.0/19",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
				},
			},
		},
		{
			name: "udp-service with source ranges annotation",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/load-balancer-source-ranges": "1.2.3.4/17, 5.6.7.8/18",
					},
				},
			},
			tgPort: port80,
			hcPort: port808,
			subnets: []*ec2.Subnet{{
				CidrBlock: aws.String("172.16.0.0/19"),
				SubnetId:  aws.String("az-1"),
			}},
			tgProtocol:    corev1.ProtocolUDP,
			ipAddressType: elbv2.TargetGroupIPAddressTypeIPv4,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/17",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "5.6.7.8/18",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolUDP,
								Port:     &port80,
							},
						},
					},
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "172.16.0.0/19",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port808,
							},
						},
					},
				},
			},
		},
		{
			name:                "udp-service with no source ranges configuration",
			gateway:             &v1beta1.Gateway{},
			tgPort:              port80,
			hcPort:              port808,
			defaultSourceRanges: []string{"0.0.0.0/0"},
			subnets: []*ec2.Subnet{{
				CidrBlock: aws.String("172.16.0.0/19"),
				SubnetId:  aws.String("az-1"),
			}},
			tgProtocol:    corev1.ProtocolUDP,
			ipAddressType: elbv2.TargetGroupIPAddressTypeIPv4,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolUDP,
								Port:     &port80,
							},
						},
					},
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "172.16.0.0/19",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port808,
							},
						},
					},
				},
			},
		},
		{
			name:    "tcp-service with traffic-port hc",
			gateway: &v1beta1.Gateway{},
			tgPort:  port80,
			hcPort:  trafficPort,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					SubnetId:  aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					SubnetId:  aws.String("sn-2"),
				},
			},
			tgProtocol:    corev1.ProtocolTCP,
			ipAddressType: elbv2.TargetGroupIPAddressTypeIPv4,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "172.16.0.0/19",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/19",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
				},
			},
		},
		{
			name:    "tcp-service with preserveClient IP, traffic-port hc",
			gateway: &v1beta1.Gateway{},
			tgPort:  port80,
			hcPort:  trafficPort,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					SubnetId:  aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					SubnetId:  aws.String("sn-2"),
				},
			},
			defaultSourceRanges: []string{"0.0.0.0/0"},
			tgProtocol:          corev1.ProtocolTCP,
			ipAddressType:       elbv2.TargetGroupIPAddressTypeIPv4,
			preserveClientIP:    true,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
				},
			},
		},
		{
			name:    "tcp-service with preserveClient IP, hc different",
			gateway: &v1beta1.Gateway{},
			tgPort:  port80,
			hcPort:  port808,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					SubnetId:  aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					SubnetId:  aws.String("sn-2"),
				},
			},
			tgProtocol:          corev1.ProtocolTCP,
			ipAddressType:       elbv2.TargetGroupIPAddressTypeIPv4,
			preserveClientIP:    true,
			defaultSourceRanges: []string{"0.0.0.0/0"},
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "172.16.0.0/19",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/19",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port808,
							},
						},
					},
				},
			},
		},
		{
			name: "tcp-service with preserve Client IP with different hc port and source range specified",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{},
			},
			tgPort: port80,
			hcPort: port808,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					SubnetId:  aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					SubnetId:  aws.String("sn-2"),
				},
			},
			tgProtocol:       corev1.ProtocolTCP,
			ipAddressType:    elbv2.TargetGroupIPAddressTypeIPv4,
			preserveClientIP: true,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "10.0.0.0/16",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "172.16.0.0/19",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/19",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port808,
							},
						},
					},
				},
			},
		},
		{
			name: "tcp-service with preserve Client IP, hc is traffic port, source range specified ",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{},
			},
			tgPort: port80,
			hcPort: port80,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					SubnetId:  aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					SubnetId:  aws.String("sn-2"),
				},
			},
			ipAddressType:    elbv2.TargetGroupIPAddressTypeIPv4,
			tgProtocol:       corev1.ProtocolTCP,
			preserveClientIP: true,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "10.0.0.0/16",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/24",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "172.16.0.0/19",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/19",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
				},
			},
		},
		{
			name: "tcp-service with preserve Client IP, hc is traffic port, source range specified and contains 0/0",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{},
			},
			tgPort: port80,
			hcPort: port80,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					SubnetId:  aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					SubnetId:  aws.String("sn-2"),
				},
			},
			tgProtocol:       corev1.ProtocolTCP,
			ipAddressType:    elbv2.TargetGroupIPAddressTypeIPv4,
			preserveClientIP: true,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "10.0.0.0/16",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "1.2.3.4/24",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "0.0.0.0/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
				},
			},
		},
		{
			name:                "ipv6 preserve client IP enabled",
			gateway:             &v1beta1.Gateway{},
			defaultSourceRanges: []string{"::/0"},
			tgPort:              port80,
			hcPort:              port80,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2300:1ab3:ab0:1900::/56"),
						},
					},
					SubnetId: aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2000:1ee3:5d0:fe00::/56"),
						},
					},
					SubnetId: aws.String("sn-2"),
				},
			},
			tgProtocol:       corev1.ProtocolTCP,
			ipAddressType:    elbv2.TargetGroupIPAddressTypeIPv6,
			preserveClientIP: true,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "::/0",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
				},
			},
		},
		{
			name:                "ipv6 preserve client IP disabled",
			gateway:             &v1beta1.Gateway{},
			defaultSourceRanges: []string{"::/0"},
			tgPort:              port80,
			hcPort:              port80,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2300:1ab3:ab0:1900::/64"),
						},
					},
					SubnetId: aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2300:1ab3:ab0:1901::/64"),
						},
					},
					SubnetId: aws.String("sn-2"),
				},
			},
			tgProtocol:       corev1.ProtocolTCP,
			ipAddressType:    elbv2.TargetGroupIPAddressTypeIPv6,
			preserveClientIP: false,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2300:1ab3:ab0:1900::/64",
								},
							},
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2300:1ab3:ab0:1901::/64",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
				},
			},
		},
		{
			name:                "ipv6 preserve client IP enabled, vpc range default",
			gateway:             &v1beta1.Gateway{},
			defaultSourceRanges: []string{"2300:1ab3:ab0:1900::/56"},
			tgPort:              port80,
			hcPort:              port80,
			subnets: []*ec2.Subnet{
				{
					CidrBlock: aws.String("172.16.0.0/19"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2300:1ab3:ab0:1900::/64"),
						},
					},
					SubnetId: aws.String("sn-1"),
				},
				{
					CidrBlock: aws.String("1.2.3.4/19"),
					Ipv6CidrBlockAssociationSet: []*ec2.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: aws.String("2300:1ab3:ab0:1901::/64"),
						},
					},
					SubnetId: aws.String("sn-2"),
				},
			},
			tgProtocol:       corev1.ProtocolTCP,
			ipAddressType:    elbv2.TargetGroupIPAddressTypeIPv6,
			preserveClientIP: true,
			want: &elbv2.TargetGroupBindingNetworking{
				Ingress: []elbv2.NetworkingIngressRule{
					{
						From: []elbv2.NetworkingPeer{
							{
								IPBlock: &elbv2api.IPBlock{
									CIDR: "2300:1ab3:ab0:1900::/56",
								},
							},
						},
						Ports: []elbv2api.NetworkingPort{
							{
								Protocol: &networkingProtocolTCP,
								Port:     &port80,
							},
						},
					},
				},
			},
		},
		{
			name: "with manage backend SG disabled via annotation",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules": "false",
					},
				},
			},
			tgPort: port80,
			hcPort: port808,
			subnets: []*ec2.Subnet{{
				CidrBlock: aws.String("172.16.0.0/19"),
				SubnetId:  aws.String("az-1"),
			}},
			tgProtocol:    corev1.ProtocolTCP,
			ipAddressType: elbv2.TargetGroupIPAddressTypeIPv4,
			want:          nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{gateway: tt.gateway, annotationParser: parser, ec2Subnets: tt.subnets}
			port := corev1.ServicePort{
				Protocol: tt.tgProtocol,
			}
			got, _ := builder.buildTargetGroupBindingNetworking(context.Background(), tt.tgPort, tt.preserveClientIP, tt.hcPort, port, tt.defaultSourceRanges, tt.ipAddressType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultModelBuilder_buildPreserveClientIPFlag(t *testing.T) {
	tests := []struct {
		testName   string
		targetType elbv2.TargetType
		tgAttrs    []elbv2.TargetGroupAttribute
		want       bool
		wantErr    error
	}{
		{
			testName:   "IP mode default",
			targetType: elbv2.TargetTypeIP,
			tgAttrs: []elbv2.TargetGroupAttribute{
				{
					Key:   tgAttrsProxyProtocolV2Enabled,
					Value: "false",
				},
				{
					Key:   "target.group-attr-1",
					Value: "80",
				},
				{
					Key:   "t2.enabled",
					Value: "false",
				},
			},
			want: false,
		},
		{
			testName:   "IP mode annotation",
			targetType: elbv2.TargetTypeIP,
			tgAttrs: []elbv2.TargetGroupAttribute{
				{
					Key:   "key1",
					Value: "value",
				},
				{
					Key:   tgAttrsPreserveClientIPEnabled,
					Value: "true",
				},
			},
			want: true,
		},
		{
			testName:   "Instance mode default",
			targetType: elbv2.TargetTypeInstance,
			want:       true,
		},
		{
			testName:   "Instance mode annotation",
			targetType: elbv2.TargetTypeInstance,
			tgAttrs: []elbv2.TargetGroupAttribute{
				{
					Key:   tgAttrsPreserveClientIPEnabled,
					Value: "false",
				},
				{
					Key:   "key1",
					Value: "value",
				},
			},
			want: false,
		},
		{
			testName:   "Attribute Parse error",
			targetType: elbv2.TargetTypeInstance,
			tgAttrs: []elbv2.TargetGroupAttribute{
				{
					Key:   tgAttrsPreserveClientIPEnabled,
					Value: " FalSe",
				},
				{
					Key:   "key1",
					Value: "value",
				},
			},
			wantErr: errors.New("failed to parse attribute preserve_client_ip.enabled= FalSe: strconv.ParseBool: parsing \" FalSe\": invalid syntax"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
			}
			got, err := builder.buildPreserveClientIPFlag(context.Background(), tt.targetType, tt.tgAttrs)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuilder_buildTargetType(t *testing.T) {

	tests := []struct {
		testName           string
		gateway            *v1beta1.Gateway
		defaultTargetType  string
		want               elbv2.TargetType
		enableIPTargetType *bool
		wantErr            error
	}{
		{
			testName: "empty annotation",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: elbv2.TargetTypeInstance,
		},
		{
			testName: "default type ip",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			defaultTargetType: "ip",
			want:              elbv2.TargetTypeIP,
		},
		{
			testName: "lb type nlb-ip",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: elbv2.TargetTypeIP,
		},
		{
			testName: "lb type external, target instance",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
					},
				},
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: elbv2.TargetTypeInstance,
		},
		{
			testName: "lb type external, target ip",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
					},
				},
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: elbv2.TargetTypeIP,
		},
		{
			testName: "enableIPTargetType is false, target ip",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
					},
				},
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			enableIPTargetType: aws.Bool(false),
			wantErr:            errors.New("unsupported targetType: ip when EnableIPTargetType is false"),
		},
		{
			testName: "external, ClusterIP with target type instance",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
					},
				},
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			wantErr: errors.New("unsupported service type \"ClusterIP\" for load balancer target type \"instance\""),
		},
		{
			testName: "load balancer class, default target type",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: elbv2.TargetTypeInstance,
		},
		{
			testName: "allocate load balancer node ports false",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			want: elbv2.TargetTypeInstance,
		},
		{
			testName: "allocate load balancer node ports false, node port unspecified",
			gateway: &v1beta1.Gateway{
				Spec: v1beta1.GatewaySpec{
					GatewayClassName: "gateway-class",
					Listeners: []v1beta1.Listener{
						{
							Name:     "gateway-listener-1",
							Port:     80,
							Protocol: v1beta1.ProtocolType(corev1.ProtocolTCP),
						},
					},
				},
			},
			wantErr: errors.New("unable to support instance target type with an unallocated NodePort"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser:  parser,
				gateway:           tt.gateway,
				defaultTargetType: elbv2.TargetType(tt.defaultTargetType),
			}
			if tt.defaultTargetType == "" {
				builder.defaultTargetType = elbv2.TargetTypeInstance
			}
			if tt.enableIPTargetType == nil {
				builder.enableIPTargetType = true
			} else {
				builder.enableIPTargetType = *tt.enableIPTargetType
			}
			got, err := builder.buildTargetType(context.Background(), &tt.gateway.Spec.Listeners[0])
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuilder_buildTargetGroupBindingNodeSelector(t *testing.T) {
	tests := []struct {
		testName   string
		gateway    *v1beta1.Gateway
		targetType elbv2.TargetType
		want       *metav1.LabelSelector
		wantErr    error
	}{
		{
			testName:   "IP target empty selector",
			targetType: elbv2.TargetTypeIP,
		},
		{
			testName:   "IP Target with selector",
			targetType: elbv2.TargetTypeIP,
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-target-node-labels": "key1=value1, k2=v2",
					},
				},
			},
		},
		{
			testName:   "Instance target empty selector",
			targetType: elbv2.TargetTypeInstance,
			gateway:    &v1beta1.Gateway{},
		},
		{
			testName:   "Instance target with selector",
			targetType: elbv2.TargetTypeInstance,
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-target-node-labels": "key1=value1, key2=value.2",
					},
				},
			},
			want: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
					"key2": "value.2",
				},
			},
		},
		{
			testName:   "Instance target with invalid selector",
			targetType: elbv2.TargetTypeInstance,
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-target-node-labels": "key1=value1, invalid",
					},
				},
			},
			wantErr: errors.New("failed to parse stringMap annotation, service.beta.kubernetes.io/aws-load-balancer-target-node-labels: key1=value1, invalid"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser: parser,
				gateway:          tt.gateway,
			}
			got, err := builder.buildTargetGroupBindingNodeSelector(context.Background(), tt.targetType)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}

		})
	}
}

func Test_defaultModelBuilder_buildTargetGroupHealthCheckPort(t *testing.T) {
	tests := []struct {
		testName    string
		gateway     *v1beta1.Gateway
		defaultPort string
		want        intstr.IntOrString
		wantErr     error
	}{
		{
			testName:    "default traffic-port",
			gateway:     &v1beta1.Gateway{},
			defaultPort: "traffic-port",
			want:        intstr.FromString("traffic-port"),
		},
		{
			testName: "with annotation",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port": "34576",
					},
				},
			},
			defaultPort: "traffic-port",
			want:        intstr.FromInt(34576),
		},
		{
			testName: "unsupported annotation value",
			gateway: &v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port": "a34576",
					},
				},
			},
			defaultPort: "traffic-port",
			wantErr:     errors.New("health check port \"a34576\" not supported"),
		},
		{
			testName:    "default health check nodeport",
			gateway:     &v1beta1.Gateway{},
			defaultPort: "31227",
			want:        intstr.FromInt(31227),
		},
		{
			testName:    "invalid default",
			gateway:     &v1beta1.Gateway{},
			defaultPort: "abs",
			wantErr:     errors.New("health check port \"abs\" not supported"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				annotationParser:       parser,
				gateway:                tt.gateway,
				defaultHealthCheckPort: tt.defaultPort,
			}
			got, err := builder.buildTargetGroupHealthCheckPort(context.Background(), tt.defaultPort)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
