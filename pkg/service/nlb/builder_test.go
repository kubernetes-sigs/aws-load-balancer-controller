package nlb

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/deploy"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/elbv2"
	"testing"
	"time"
)

func Test_nlbBuilder_buildNLB(t *testing.T) {
	tests := []struct {
		testName         string
		svc              *corev1.Service
		subnets          []string
		cirds            []string
		wantError        bool
		wantValue        string
		wantNumResources int
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
			subnets:   []string{"subnet-1"},
			cirds:     []string{"192.168.64.0/19"},
			wantError: false,
			wantValue: `
{
  "id": "default/nlb-ip-svc-tls",
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
          "scheme": "internet-facing",
		  "subnetMapping": [
			{
			  "subnetID": "subnet-1"
			}
          ],
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
              "name": "k8s-nlb-ip-s-default-507734fb10",
              "namespace": "default",
              "creationTimestamp": null
            },
            "spec": {
              "targetGroupARN": "",
              "targetType": "ip",
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "ipBlock": {
                          "cidr": "192.168.64.0/19"
                        }
                      }
                    ],
                    "ports": [
                      {
                        "port": 80
                      }
                    ]
				  }
                ]
			  },
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
			wantNumResources: 4,
		},
		{
			testName: "Multple listeners, same target group",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip-svc",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                            "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":            "HTTP",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                "8888",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-path":                "/healthz",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "10",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "30",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "2",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "2",
					},
					UID: "7ab4be33-11c2-4a7b-b655-7add8affab36",
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{"app": "hello"},
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
						{
							Name:       "alt2",
							Port:       83,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			},
			subnets:   []string{"subnet-abc", "test-subnet"},
			cirds:     []string{"172.20.32.0/19", "172.20.64.0/19"},
			wantError: false,
			wantValue: `
{
  "id": "default/nlb-ip-svc",
  "resources": {
    "AWS::ElasticLoadBalancingV2::Listener": {
      "83": {
        "spec": {
          "protocol": "TCP",
          "defaultActions": [
            {
              "forwardConfig": {
                "targetGroups": [
                  {
                    "targetGroupARN": {
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-03582c76a7/status/targetGroupARN"
                    }
                  }
                ]
              },
              "type": "forward"
            }
          ],
          "loadBalancerARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/default/nlb-ip-svc/status/loadBalancerARN"
          },
          "port": 83
        }
      },
      "80": {
        "spec": {
          "protocol": "TCP",
          "defaultActions": [
            {
              "forwardConfig": {
                "targetGroups": [
                  {
                    "targetGroupARN": {
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-03582c76a7/status/targetGroupARN"
                    }
                  }
                ]
              },
              "type": "forward"
            }
          ],
          "loadBalancerARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/default/nlb-ip-svc/status/loadBalancerARN"
          },
          "port": 80
        }
      }
    },
    "K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
      "k8s-nlb-ip-s-default-03582c76a7": {
        "spec": {
          "targetGroupARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-03582c76a7/status/targetGroupARN"
          },
          "template": {
            "spec": {
              "targetType": "ip",
              "targetGroupARN": "",
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "ipBlock": {
                          "cidr": "172.20.32.0/19"
                        }
                      },
                      {
                        "ipBlock": {
                          "cidr": "172.20.64.0/19"
                        }
                      }
                    ],
                    "ports": [
                      {
                        "port": 80
                      }
                    ]
				  }
                ]
			  },
              "serviceRef": {
                "name": "nlb-ip-svc",
                "port": 80
              }
            },
            "metadata": {
              "name": "k8s-nlb-ip-s-default-03582c76a7",
              "namespace": "default",
              "creationTimestamp": null
            }
          }
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::LoadBalancer": {
      "default/nlb-ip-svc": {
        "spec": {
          "ipAddressType": "ipv4",
          "type": "network",
          "scheme": "internet-facing",
		  "subnetMapping": [
			{
			  "subnetID": "subnet-abc"
			},
			{
			  "subnetID": "test-subnet"
			}
          ],
          "name": "a7ab4be3311c24a7bb6557add8affab3",
          "loadBalancerAttributes": [
            {
              "value": "false",
              "key": "access_logs.s3.enabled"
            },
            {
              "value": "",
              "key": "access_logs.s3.bucket"
            },
            {
              "value": "",
              "key": "access_logs.s3.prefix"
            },
            {
              "value": "false",
              "key": "load_balancing.cross_zone.enabled"
            }
          ]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "k8s-nlb-ip-s-default-03582c76a7": {
        "spec": {
          "targetType": "ip",
          "protocol": "TCP",
          "name": "k8s-nlb-ip-s-default-03582c76a7",
          "healthCheckConfig": {
            "protocol": "HTTP",
            "timeoutSeconds": 30,
            "intervalSeconds": 10,
            "unhealthyThresholdCount": 2,
            "healthyThresholdCount": 2,
            "path": "/healthz",
            "port": 8888
          },
          "targetGroupAttributes": [
            {
              "value": "false",
              "key": "proxy_protocol_v2.enabled"
            }
          ],
          "port": 80
        }
      }
    }
  }
}
`,
			wantNumResources: 5,
		},
		{
			testName: "TLS and access logging annotations",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip-svc-tls",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                              "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":              "HTTP",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                  "8888",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-path":                  "/healthz",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":              "10",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":               "30",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":     "2",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold":   "2",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-enabled":                "true",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name":         "nlb-bucket",
						"service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix":       "bkt-pfx",
						"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
						"service.beta.kubernetes.io/aws-load-balancer-ssl-ports":                         "83",
						"service.beta.kubernetes.io/aws-load-balancer-ssl-cert":                          "certArn1,certArn2",
					},
					UID: "7ab4be33-11c2-4a7b-b655-7add8affab36",
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{"app": "hello"},
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
						{
							Name:       "alt2",
							Port:       83,
							TargetPort: intstr.FromInt(8883),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			},
			subnets:   []string{"s-1", "s-2", "s-3"},
			cirds:     []string{"10.1.1.1/32", "10.1.1.2/32", "10.1.1.3/32"},
			wantError: false,
			wantValue: `
{
  "id": "default/nlb-ip-svc-tls",
  "resources": {
    "AWS::ElasticLoadBalancingV2::Listener": {
      "83": {
        "spec": {
          "certificates": [
            {
              "certificateARN": "certArn2"
            },
            {
              "certificateARN": "certArn2"
            }
          ],
          "protocol": "TLS",
          "defaultActions": [
            {
              "forwardConfig": {
                "targetGroups": [
                  {
                    "targetGroupARN": {
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-f4577ac8db/status/targetGroupARN"
                    }
                  }
                ]
              },
              "type": "forward"
            }
          ],
          "loadBalancerARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/default/nlb-ip-svc-tls/status/loadBalancerARN"
          },
          "port": 83
        }
      },
      "80": {
        "spec": {
          "protocol": "TCP",
          "defaultActions": [
            {
              "forwardConfig": {
                "targetGroups": [
                  {
                    "targetGroupARN": {
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-03582c76a7/status/targetGroupARN"
                    }
                  }
                ]
              },
              "type": "forward"
            }
          ],
          "loadBalancerARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/default/nlb-ip-svc-tls/status/loadBalancerARN"
          },
          "port": 80
        }
      }
    },
    "K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
      "k8s-nlb-ip-s-default-03582c76a7": {
        "spec": {
          "targetGroupARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-03582c76a7/status/targetGroupARN"
          },
          "template": {
            "spec": {
              "targetType": "ip",
              "targetGroupARN": "",
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "ipBlock": {
                          "cidr": "10.1.1.1/32"
                        }
                      },
                      {
                        "ipBlock": {
                          "cidr": "10.1.1.2/32"
                        }
                      },
                      {
                        "ipBlock": {
                          "cidr": "10.1.1.3/32"
                        }
                      }
                    ],
                    "ports": [
                      {
                        "port": 80
                      }
                    ]
                  }
                ]
              },
              "serviceRef": {
                "name": "nlb-ip-svc-tls",
                "port": 80
              }
            },
            "metadata": {
              "name": "k8s-nlb-ip-s-default-03582c76a7",
              "namespace": "default",
              "creationTimestamp": null
            }
          }
        }
      },
      "k8s-nlb-ip-s-default-f4577ac8db": {
        "spec": {
          "targetGroupARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/k8s-nlb-ip-s-default-f4577ac8db/status/targetGroupARN"
          },
          "template": {
            "spec": {
              "targetType": "ip",
              "targetGroupARN": "",
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "ipBlock": {
                          "cidr": "10.1.1.1/32"
                        }
                      },
                      {
                        "ipBlock": {
                          "cidr": "10.1.1.2/32"
                        }
                      },
                      {
                        "ipBlock": {
                          "cidr": "10.1.1.3/32"
                        }
                      }
                    ],
                    "ports": [
                      {
                        "port": 8883
                      }
                    ]
                  }
                ]
              },
              "serviceRef": {
                "name": "nlb-ip-svc-tls",
                "port": 83
              }
            },
            "metadata": {
              "name": "k8s-nlb-ip-s-default-f4577ac8db",
              "namespace": "default",
              "creationTimestamp": null
            }
          }
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::LoadBalancer": {
      "default/nlb-ip-svc-tls": {
        "spec": {
          "ipAddressType": "ipv4",
          "type": "network",
          "scheme": "internet-facing",
		  "subnetMapping": [
			{
			  "subnetID": "s-1"
			},
			{
			  "subnetID": "s-2"
			},
			{
			  "subnetID": "s-3"
			}
          ],
          "name": "a7ab4be3311c24a7bb6557add8affab3",
          "loadBalancerAttributes": [
            {
              "value": "true",
              "key": "access_logs.s3.enabled"
            },
            {
              "value": "nlb-bucket",
              "key": "access_logs.s3.bucket"
            },
            {
              "value": "bkt-pfx",
              "key": "access_logs.s3.prefix"
            },
            {
              "value": "true",
              "key": "load_balancing.cross_zone.enabled"
            }
          ]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "k8s-nlb-ip-s-default-03582c76a7": {
        "spec": {
          "targetType": "ip",
          "protocol": "TCP",
          "name": "k8s-nlb-ip-s-default-03582c76a7",
          "healthCheckConfig": {
            "protocol": "HTTP",
            "timeoutSeconds": 30,
            "intervalSeconds": 10,
            "unhealthyThresholdCount": 2,
            "healthyThresholdCount": 2,
            "path": "/healthz",
            "port": 8888
          },
          "targetGroupAttributes": [
            {
              "value": "false",
              "key": "proxy_protocol_v2.enabled"
            }
          ],
          "port": 80
        }
      },
      "k8s-nlb-ip-s-default-f4577ac8db": {
        "spec": {
          "targetType": "ip",
          "protocol": "TCP",
          "name": "k8s-nlb-ip-s-default-f4577ac8db",
          "healthCheckConfig": {
            "protocol": "HTTP",
            "timeoutSeconds": 30,
            "intervalSeconds": 10,
            "unhealthyThresholdCount": 2,
            "healthyThresholdCount": 2,
            "path": "/healthz",
            "port": 8888
          },
          "targetGroupAttributes": [
            {
              "value": "false",
              "key": "proxy_protocol_v2.enabled"
            }
          ],
          "port": 8883
        }
      }
    }
  }
}
`,
			wantNumResources: 7,
		},
		{
			testName: "Service being deleted",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-deleted",
					Namespace: "doesnt-exist",
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
			wantValue: `
{
  "id": "doesnt-exist/service-deleted",
  "resources": {}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := NewServiceBuilder(tt.svc, NewMockSubnetsResolver(tt.subnets, tt.cirds), parser)
			ctx := context.Background()
			stack, _, err := builder.Build(ctx)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				d := deploy.NewDefaultStackMarshaller()
				jsonString, err := d.Marshal(stack)
				assert.Equal(t, nil, err)
				assert.JSONEq(t, tt.wantValue, jsonString)

				visitor := &resourceVisitor{}
				stack.TopologicalTraversal(visitor)
				assert.Equal(t, tt.wantNumResources, len(visitor.resources))
			}
		})
	}
}

func Test_nlbBuilder_buildLBAttributes(t *testing.T) {
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
			lbAttributes, err := builder.buildLoadBalancerAttributes(context.Background())
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.wantValue, lbAttributes)
			}
		})
	}
}

func Test_nlbBuilder_targetGroupAttrs(t *testing.T) {
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

func Test_nlbBuilder_buildTargetHealthCheck(t *testing.T) {
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
				Protocol:                (*elbv2.Protocol)(aws.String(string(elbv2.ProtocolTCP))),
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

func Test_nlbBuilder_buildSubnetMappings(t *testing.T) {
	tests := []struct {
		name    string
		subnets []string
		cidrs   []string
		want    []elbv2.SubnetMapping
		wantErr error
	}{
		{
			name:    "Empty subnets",
			subnets: []string{},
			wantErr: errors.New("Unable to discover at least one subnet across availability zones"),
		},
		{
			name:    "Multiple subnets",
			subnets: []string{"s-1", "s-2"},
			cidrs:   []string{"10.1.1.1/32", "10.1.1.2/32"},
			want: []elbv2.SubnetMapping{
				{
					SubnetID: "s-1",
				},
				{
					SubnetID: "s-2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &nlbBuilder{subnetsResolver: NewMockSubnetsResolver(tt.subnets, tt.cidrs)}
			subnetResolver := NewMockSubnetsResolver(tt.subnets, tt.cidrs)
			ec2Subnets, _ := subnetResolver.DiscoverSubnets(context.Background(), elbv2.LoadBalancerSchemeInternetFacing)
			got, err := builder.buildSubnetMappings(context.Background(), ec2Subnets)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
