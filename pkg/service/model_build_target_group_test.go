package service

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sort"
	"testing"
)

func Test_defaultModelBuilderTask_targetGroupAttrs(t *testing.T) {
	tests := []struct {
		testName  string
		svc       *corev1.Service
		wantError bool
		wantValue []elbv2.TargetGroupAttribute
	}{
		{
			testName: "Default values",
			svc: &corev1.Service{
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
			svc: &corev1.Service{
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
			svc: &corev1.Service{
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
			svc: &corev1.Service{
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
			svc: &corev1.Service{
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
			svc: &corev1.Service{
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
			svc: &corev1.Service{
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
				service:          tt.svc,
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
	tests := []struct {
		testName  string
		svc       *corev1.Service
		wantError bool
		wantValue *elbv2.TargetGroupHealthCheckConfig
	}{
		{
			testName: "Default config",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			wantError: false,
			wantValue: &elbv2.TargetGroupHealthCheckConfig{
				Port:                    &trafficPort,
				Protocol:                (*elbv2.Protocol)(aws.String(string(elbv2.ProtocolTCP))),
				IntervalSeconds:         aws.Int64(10),
				HealthyThresholdCount:   aws.Int64(3),
				UnhealthyThresholdCount: aws.Int64(3),
			},
		},
		{
			testName: "With annotations",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":            "HTTP",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                "8888",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-path":                "/healthz",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "10",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "30",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "2",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "2",
					},
				},
			},
			wantError: false,
			wantValue: &elbv2.TargetGroupHealthCheckConfig{
				Port:                    &port8888,
				Protocol:                (*elbv2.Protocol)(aws.String("HTTP")),
				Path:                    aws.String("/healthz"),
				IntervalSeconds:         aws.Int64(10),
				HealthyThresholdCount:   aws.Int64(2),
				UnhealthyThresholdCount: aws.Int64(2),
			},
		},
		{
			testName: "default path",
			svc: &corev1.Service{
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
				HealthyThresholdCount:   aws.Int64(3),
				UnhealthyThresholdCount: aws.Int64(3),
			},
		},
		{
			testName: "invalid values",
			svc: &corev1.Service{
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
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{
				service:                              tt.svc,
				annotationParser:                     parser,
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
			}
			hc, err := builder.buildTargetGroupHealthCheckConfig(context.Background())
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
		name             string
		svc              *corev1.Service
		tgPort           intstr.IntOrString
		hcPort           intstr.IntOrString
		subnets          []*ec2.Subnet
		tgProtocol       corev1.Protocol
		preserveClientIP bool
		want             *elbv2.TargetGroupBindingNetworking
	}{
		{
			name: "udp-service with source ranges",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					LoadBalancerSourceRanges: []string{"10.0.0.0/16", "1.2.3.4/24"},
				},
			},
			tgPort: port80,
			hcPort: trafficPort,
			subnets: []*ec2.Subnet{{
				CidrBlock: aws.String("172.16.0.0/19"),
				SubnetId:  aws.String("az-1"),
			}},
			tgProtocol: corev1.ProtocolUDP,
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
			svc: &corev1.Service{
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
			tgProtocol: corev1.ProtocolUDP,
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
			name:   "udp-service with no source ranges configuration",
			svc:    &corev1.Service{},
			tgPort: port80,
			hcPort: port808,
			subnets: []*ec2.Subnet{{
				CidrBlock: aws.String("172.16.0.0/19"),
				SubnetId:  aws.String("az-1"),
			}},
			tgProtocol: corev1.ProtocolUDP,
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
			name:   "tcp-service with traffic-port hc",
			svc:    &corev1.Service{},
			tgPort: port80,
			hcPort: trafficPort,
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
			tgProtocol: corev1.ProtocolTCP,
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
			name:   "tcp-service with preserveClient IP, traffic-port hc",
			svc:    &corev1.Service{},
			tgPort: port80,
			hcPort: trafficPort,
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
			preserveClientIP: true,
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
			name:   "tcp-service with preserveClient IP, hc different",
			svc:    &corev1.Service{},
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
			preserveClientIP: true,
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
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					LoadBalancerSourceRanges: []string{"10.0.0.0/16", "1.2.3.4/24"},
				},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &defaultModelBuildTask{service: tt.svc, annotationParser: parser, ec2Subnets: tt.subnets}
			got := builder.buildTargetGroupBindingNetworking(context.Background(), tt.tgPort, tt.preserveClientIP, tt.hcPort, tt.tgProtocol)
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
