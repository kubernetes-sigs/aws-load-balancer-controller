package nlb

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"testing"
)

func Test_buildNLB(t *testing.T) {
	tests := []struct {
		testName  string
		svc       *corev1.Service
		wantError bool
		wantValue string
	}{
		{
			testName: "Simple service",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip-svc-tls",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{"app": "hello"},
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			},
			wantError: false,
			wantValue: `
{
    "resources": {
        "AWS::ElasticLoadBalancingV2::Listener": {
            "80": {
                "spec": {
                    "loadBalancerARN": {
                        "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/default/nlb-ip-svc-tls/status/loadBalancerARN"
                    },
                    "port": 80,
                    "protocol": "TCP",
                    "defaultActions": [
                        {
                            "type": "forward",
                            "forwardConfig": {
                                "targetGroups": [
                                    {
                                        "targetGroupARN": {
                                            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-507734fb10/status/targetGroupARN"
                                        }
                                    }
                                ]
                            }
                        }
                    ]
                }
            }
        },
        "AWS::ElasticLoadBalancingV2::LoadBalancer": {
            "default/nlb-ip-svc-tls": {
                "spec": {
                    "name": "a",
                    "type": "network",
                    "ipAddressType": "ipv4",
                    "loadBalancerAttributes": [
                        {
                            "key": "access_logs.s3.enabled",
                            "value": "false"
                        },
                        {
                            "key": "access_logs.s3.bucket",
                            "value": ""
                        },
                        {
                            "key": "access_logs.s3.prefix",
                            "value": ""
                        },
                        {
                            "key": "load_balancing.cross_zone.enabled",
                            "value": "false"
                        }
                    ]
                }
            }
        },
        "AWS::ElasticLoadBalancingV2::TargetGroup": {
            "k8s-nlb-ip-s-default-507734fb10": {
                "spec": {
                    "name": "k8s-nlb-ip-s-default-507734fb10",
                    "targetType": "ip",
                    "port": 80,
                    "protocol": "TCP",
                    "healthCheckConfig": {
                        "port": "traffic-port",
                        "protocol": "TCP",
                        "intervalSeconds": 10,
                        "timeoutSeconds": 10,
                        "healthyThresholdCount": 3,
                        "unhealthyThresholdCount": 3
                    },
                    "targetGroupAttributes": [
                        {
                            "key": "proxy_protocol_v2.enabled",
                            "value": "false"
                        }
                    ]
                }
            }
        },
        "K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
            "k8s-nlb-ip-s-default-507734fb10": {
                "spec": {
                    "targetGroupARN": {
                        "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-507734fb10/status/targetGroupARN"
                    },
                    "template": {
                        "metadata": {
                            "generateName": "k8s-nlb-ip-s-default-507734fb10",
                            "namespace": "default",
                            "creationTimestamp": null
                        },
                        "spec": {
                            "targetGroupARN": "",
                            "targetType": "ip",
                            "serviceRef": {
                                "name": "nlb-ip-svc-tls",
                                "port": 80
                            }
                        }
                    }
                }
            }
        }
    }
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := NewServiceBuilder(tt.svc, k8s.NamespacedName(tt.svc), parser)
			ctx := context.Background()
			stack, err := builder.Build(ctx)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				d := deploy.NewDefaultStackMarshaller()
				jsonString, err := d.Marshal(stack)
				assert.Equal(t, nil, err)
				assert.JSONEq(t, tt.wantValue, jsonString)
			}
		})
	}
}

func Test_buildLBAttributes(t *testing.T) {
	tests := []struct {
		testName  string
		svc       *corev1.Service
		wantError bool
		wantValue []elbv2.LoadBalancerAttribute
	}{
		{
			testName: "Default values",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
					},
				},
			},
			wantError: false,
			wantValue: []elbv2.LoadBalancerAttribute{
				{
					Key:   LBAttrsAccessLogsS3Enabled,
					Value: "false",
				},
				{
					Key:   LBAttrsAccessLogsS3Bucket,
					Value: "",
				},
				{
					Key:   LBAttrsAccessLogsS3Prefix,
					Value: "",
				},
				{
					Key:   LBAttrsLoadBalancingCrossZoneEnabled,
					Value: "false",
				},
			},
		},
		{
			testName: "Annotation specified",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                              "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-enabled":                "true",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name":         "nlb-bucket",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix":       "bkt-pfx",
						"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
					},
				},
			},
			wantError: false,
			wantValue: []elbv2.LoadBalancerAttribute{
				{
					Key:   LBAttrsAccessLogsS3Enabled,
					Value: "true",
				},
				{
					Key:   LBAttrsAccessLogsS3Bucket,
					Value: "nlb-bucket",
				},
				{
					Key:   LBAttrsAccessLogsS3Prefix,
					Value: "bkt-pfx",
				},
				{
					Key:   LBAttrsLoadBalancingCrossZoneEnabled,
					Value: "true",
				},
			},
		},
		{
			testName: "Annotation invalid",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                              "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-enabled":                "FalSe",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name":         "nlb-bucket",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix":       "bkt-pfx",
						"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
					},
				},
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &nlbBuilder{
				service:          tt.svc,
				key:              types.NamespacedName{},
				annotationParser: parser,
			}
			lbAttributes, err := builder.buildLBAttributes(context.Background())
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.wantValue, lbAttributes)
			}
		})
	}
}

func Test_targetGroupAttrs(t *testing.T) {
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
					Key:   TGAttrsProxyProtocolV2Enabled,
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
					Key:   TGAttrsProxyProtocolV2Enabled,
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
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := &nlbBuilder{
				service:          tt.svc,
				key:              types.NamespacedName{},
				annotationParser: parser,
			}
			tgAttrs, err := builder.targetGroupAttrs(context.Background())
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.wantValue, tgAttrs)
			}
		})
	}
}

func Test_buildTargetHealthCheck(t *testing.T) {
	trafficPort := intstr.FromString("traffic-port")
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
				Protocol:                (*elbv2.Protocol)(aws.String(elbv2.ProtocolTCP)),
				IntervalSeconds:         aws.Int64(DefaultHealthCheckInterval),
				TimeoutSeconds:          aws.Int64(DefaultHealthCheckTimeout),
				HealthyThresholdCount:   aws.Int64(DefaultHealthCheckHealthyThreshold),
				UnhealthyThresholdCount: aws.Int64(DefaultHealthCheckUnhealthyThreshold),
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
				TimeoutSeconds:          aws.Int64(30),
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
				TimeoutSeconds:          aws.Int64(10),
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
			builder := &nlbBuilder{
				service:          tt.svc,
				key:              types.NamespacedName{},
				annotationParser: parser,
			}
			hc, err := builder.buildTargetHealthCheck(context.Background())
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.wantValue, hc)
			}
		})
	}
}
