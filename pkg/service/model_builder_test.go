package service

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	mock_networking "sigs.k8s.io/aws-load-balancer-controller/mocks/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
)

func Test_defaultModelBuilderTask_Build(t *testing.T) {
	type resolveViaDiscoveryCall struct {
		subnets []*ec2.Subnet
		err     error
	}
	resolveViaDiscoveryCallForOneSubnet := resolveViaDiscoveryCall{
		subnets: []*ec2.Subnet{
			{
				SubnetId:  aws.String("subnet-1"),
				CidrBlock: aws.String("192.168.0.0/19"),
			},
		},
	}
	resolveViaDiscoveryCallForTwoSubnet := resolveViaDiscoveryCall{
		subnets: []*ec2.Subnet{
			{
				SubnetId:  aws.String("subnet-1"),
				CidrBlock: aws.String("192.168.0.0/19"),
			},
			{
				SubnetId:  aws.String("subnet-2"),
				CidrBlock: aws.String("192.168.32.0/19"),
			},
		},
	}
	resolveViaDiscoveryCallForThreeSubnet := resolveViaDiscoveryCall{
		subnets: []*ec2.Subnet{
			{
				SubnetId:  aws.String("subnet-1"),
				CidrBlock: aws.String("192.168.0.0/19"),
			},
			{
				SubnetId:  aws.String("subnet-2"),
				CidrBlock: aws.String("192.168.32.0/19"),
			},
			{
				SubnetId:  aws.String("subnet-3"),
				CidrBlock: aws.String("192.168.64.0/19"),
			},
		},
	}

	tests := []struct {
		testName                 string
		resolveViaDiscoveryCalls []resolveViaDiscoveryCall
		svc                      *corev1.Service
		wantError                bool
		wantValue                string
		wantNumResources         int
	}{
		{
			testName: "Simple service",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip-svc-tls",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
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
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			wantError:                false,
			wantValue: `
{
 "id":"default/nlb-ip-svc-tls",
 "resources":{
    "AWS::ElasticLoadBalancingV2::Listener":{
       "80":{
          "spec":{
             "loadBalancerARN":{
                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
             },
             "port":80,
             "protocol":"TCP",
             "defaultActions":[
                {
                   "type":"forward",
                   "forwardConfig":{
                      "targetGroups":[
                         {
                            "targetGroupARN":{
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc-tls:80/status/targetGroupARN"
                            }
                         }
                      ]
                   }
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::LoadBalancer":{
       "LoadBalancer":{
          "spec":{
             "name":"k8s-default-nlbipsvc-4d831c6ca6",
             "type":"network",
             "scheme":"internet-facing",
             "ipAddressType":"ipv4",
             "subnetMapping":[
                {
                   "subnetID":"subnet-1"
                }
             ],
             "loadBalancerAttributes":[
                {
                   "key":"access_logs.s3.enabled",
                   "value":"false"
                },
                {
                   "key":"access_logs.s3.bucket",
                   "value":""
                },
                {
                   "key":"access_logs.s3.prefix",
                   "value":""
                },
                {
                   "key":"load_balancing.cross_zone.enabled",
                   "value":"false"
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "default/nlb-ip-svc-tls:80":{
          "spec":{
             "name":"k8s-default-nlbipsvc-d4818dcd51",
             "targetType":"ip",
             "port":80,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port":"traffic-port",
                "protocol":"TCP",
                "intervalSeconds":10,
                "healthyThresholdCount":3,
                "unhealthyThresholdCount":3
             },
             "targetGroupAttributes":[
                {
                   "key":"proxy_protocol_v2.enabled",
                   "value":"false"
                }
             ]
          }
       }
    },
    "K8S::ElasticLoadBalancingV2::TargetGroupBinding":{
       "default/nlb-ip-svc-tls:80":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-nlbipsvc-d4818dcd51",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc-tls:80/status/targetGroupARN"
                   },
                   "targetType":"ip",
                   "serviceRef":{
                      "name":"nlb-ip-svc-tls",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":80
                               }
                            ]
                         }
                      ]
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
			testName: "Dualstack service",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip-svc-tls",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
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
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			wantError:                false,
			wantValue: `
{
 "id":"default/nlb-ip-svc-tls",
 "resources":{
    "AWS::ElasticLoadBalancingV2::Listener":{
       "80":{
          "spec":{
             "loadBalancerARN":{
                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
             },
             "port":80,
             "protocol":"TCP",
             "defaultActions":[
                {
                   "type":"forward",
                   "forwardConfig":{
                      "targetGroups":[
                         {
                            "targetGroupARN":{
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc-tls:80/status/targetGroupARN"
                            }
                         }
                      ]
                   }
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::LoadBalancer":{
       "LoadBalancer":{
          "spec":{
             "name":"k8s-default-nlbipsvc-4d831c6ca6",
             "type":"network",
             "scheme":"internet-facing",
             "ipAddressType":"dualstack",
             "subnetMapping":[
                {
                   "subnetID":"subnet-1"
                }
             ],
             "loadBalancerAttributes":[
                {
                   "key":"access_logs.s3.enabled",
                   "value":"false"
                },
                {
                   "key":"access_logs.s3.bucket",
                   "value":""
                },
                {
                   "key":"access_logs.s3.prefix",
                   "value":""
                },
                {
                   "key":"load_balancing.cross_zone.enabled",
                   "value":"false"
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "default/nlb-ip-svc-tls:80":{
          "spec":{
             "name":"k8s-default-nlbipsvc-d4818dcd51",
             "targetType":"ip",
             "port":80,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port":"traffic-port",
                "protocol":"TCP",
                "intervalSeconds":10,
                "healthyThresholdCount":3,
                "unhealthyThresholdCount":3
             },
             "targetGroupAttributes":[
                {
                   "key":"proxy_protocol_v2.enabled",
                   "value":"false"
                }
             ]
          }
       }
    },
    "K8S::ElasticLoadBalancingV2::TargetGroupBinding":{
       "default/nlb-ip-svc-tls:80":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-nlbipsvc-d4818dcd51",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc-tls:80/status/targetGroupARN"
                   },
                   "targetType":"ip",
                   "serviceRef":{
                      "name":"nlb-ip-svc-tls",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":80
                               }
                            ]
                         }
                      ]
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
			testName: "Multiple listeners, multiple target groups",
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
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForTwoSubnet},
			wantError:                false,
			wantValue: `
{
 "id":"default/nlb-ip-svc",
 "resources":{
    "AWS::ElasticLoadBalancingV2::Listener":{
       "80":{
          "spec":{
             "loadBalancerARN":{
                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
             },
             "port":80,
             "protocol":"TCP",
             "defaultActions":[
                {
                   "type":"forward",
                   "forwardConfig":{
                      "targetGroups":[
                         {
                            "targetGroupARN":{
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc:80/status/targetGroupARN"
                            }
                         }
                      ]
                   }
                }
             ]
          }
       },
       "83":{
          "spec":{
             "loadBalancerARN":{
                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
             },
             "port":83,
             "protocol":"TCP",
             "defaultActions":[
                {
                   "type":"forward",
                   "forwardConfig":{
                      "targetGroups":[
                         {
                            "targetGroupARN":{
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc:83/status/targetGroupARN"
                            }
                         }
                      ]
                   }
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::LoadBalancer":{
       "LoadBalancer":{
          "spec":{
             "name":"k8s-default-nlbipsvc-33e41aa671",
             "type":"network",
             "scheme":"internet-facing",
             "ipAddressType":"ipv4",
             "subnetMapping":[
                {
                   "subnetID":"subnet-1"
                },
                {
                   "subnetID":"subnet-2"
                }
             ],
             "loadBalancerAttributes":[
                {
                   "key":"access_logs.s3.enabled",
                   "value":"false"
                },
                {
                   "key":"access_logs.s3.bucket",
                   "value":""
                },
                {
                   "key":"access_logs.s3.prefix",
                   "value":""
                },
                {
                   "key":"load_balancing.cross_zone.enabled",
                   "value":"false"
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "default/nlb-ip-svc:80":{
          "spec":{
             "name":"k8s-default-nlbipsvc-62f81639fc",
             "targetType":"ip",
             "port":80,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port":8888,
                "protocol":"HTTP",
                "path":"/healthz",
                "intervalSeconds":10,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2
             },
             "targetGroupAttributes":[
                {
                   "key":"proxy_protocol_v2.enabled",
                   "value":"false"
                }
             ]
          }
       },
       "default/nlb-ip-svc:83":{
          "spec":{
             "name":"k8s-default-nlbipsvc-3ede6b28b6",
             "targetType":"ip",
             "port":80,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port":8888,
                "protocol":"HTTP",
                "path":"/healthz",
                "intervalSeconds":10,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2
             },
             "targetGroupAttributes":[
                {
                   "key":"proxy_protocol_v2.enabled",
                   "value":"false"
                }
             ]
          }
       }
    },
    "K8S::ElasticLoadBalancingV2::TargetGroupBinding":{
       "default/nlb-ip-svc:80":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-nlbipsvc-62f81639fc",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc:80/status/targetGroupARN"
                   },
                   "targetType":"ip",
                   "serviceRef":{
                      "name":"nlb-ip-svc",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.32.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":80
                               }
                            ]
                         },
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.32.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":8888
                               }
                            ]
                         }
                      ]
                   }
                }
             }
          }
       },
       "default/nlb-ip-svc:83":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-nlbipsvc-3ede6b28b6",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc:83/status/targetGroupARN"
                   },
                   "targetType":"ip",
                   "serviceRef":{
                      "name":"nlb-ip-svc",
                      "port":83
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.32.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":80
                               }
                            ]
                         },
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.32.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":8888
                               }
                            ]
                         }
                      ]
                   }
                }
             }
          }
       }
    }
 }
}
`,
			wantNumResources: 7,
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
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                  "80",
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
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForThreeSubnet},
			wantError:                false,
			wantValue: `
{
 "id":"default/nlb-ip-svc-tls",
 "resources":{
    "AWS::ElasticLoadBalancingV2::Listener":{
       "80":{
          "spec":{
             "loadBalancerARN":{
                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
             },
             "port":80,
             "protocol":"TCP",
             "defaultActions":[
                {
                   "type":"forward",
                   "forwardConfig":{
                      "targetGroups":[
                         {
                            "targetGroupARN":{
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc-tls:80/status/targetGroupARN"
                            }
                         }
                      ]
                   }
                }
             ]
          }
       },
       "83":{
          "spec":{
             "loadBalancerARN":{
                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
             },
             "port":83,
             "protocol":"TLS",
             "defaultActions":[
                {
                   "type":"forward",
                   "forwardConfig":{
                      "targetGroups":[
                         {
                            "targetGroupARN":{
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc-tls:83/status/targetGroupARN"
                            }
                         }
                      ]
                   }
                }
             ],
             "certificates":[
                {
                   "certificateARN":"certArn1"
                },
                {
                   "certificateARN":"certArn2"
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::LoadBalancer":{
       "LoadBalancer":{
          "spec":{
             "name":"k8s-default-nlbipsvc-33e41aa671",
             "type":"network",
             "scheme":"internet-facing",
             "ipAddressType":"ipv4",
             "subnetMapping":[
                {
                   "subnetID":"subnet-1"
                },
                {
                   "subnetID":"subnet-2"
                },
                {
                   "subnetID":"subnet-3"
                }
             ],
             "loadBalancerAttributes":[
                {
                   "key":"access_logs.s3.enabled",
                   "value":"true"
                },
                {
                   "key":"access_logs.s3.bucket",
                   "value":"nlb-bucket"
                },
                {
                   "key":"access_logs.s3.prefix",
                   "value":"bkt-pfx"
                },
                {
                   "key":"load_balancing.cross_zone.enabled",
                   "value":"true"
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "default/nlb-ip-svc-tls:80":{
          "spec":{
             "name":"k8s-default-nlbipsvc-62f81639fc",
             "targetType":"ip",
             "port":80,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port":80,
                "protocol":"HTTP",
                "path":"/healthz",
                "intervalSeconds":10,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2
             },
             "targetGroupAttributes":[
                {
                   "key":"proxy_protocol_v2.enabled",
                   "value":"false"
                }
             ]
          }
       },
       "default/nlb-ip-svc-tls:83":{
          "spec":{
             "name":"k8s-default-nlbipsvc-77ea0c7734",
             "targetType":"ip",
             "port":8883,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port":80,
                "protocol":"HTTP",
                "path":"/healthz",
                "intervalSeconds":10,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2
             },
             "targetGroupAttributes":[
                {
                   "key":"proxy_protocol_v2.enabled",
                   "value":"false"
                }
             ]
          }
       }
    },
    "K8S::ElasticLoadBalancingV2::TargetGroupBinding":{
       "default/nlb-ip-svc-tls:80":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-nlbipsvc-62f81639fc",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc-tls:80/status/targetGroupARN"
                   },
                   "targetType":"ip",
                   "serviceRef":{
                      "name":"nlb-ip-svc-tls",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.32.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.64.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":80
                               }
                            ]
                         }
                      ]
                   }
                }
             }
          }
       },
       "default/nlb-ip-svc-tls:83":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-nlbipsvc-77ea0c7734",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/nlb-ip-svc-tls:83/status/targetGroupARN"
                   },
                   "targetType":"ip",
                   "serviceRef":{
                      "name":"nlb-ip-svc-tls",
                      "port":83
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.32.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.64.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":8883
                               }
                            ]
                         },
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.32.0/19"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.64.0/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":80
                               }
                            ]
                         }
                      ]
                   }
                }
             }
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			subnetsResolver := mock_networking.NewMockSubnetsResolver(ctrl)
			for _, call := range tt.resolveViaDiscoveryCalls {
				subnetsResolver.EXPECT().ResolveViaDiscovery(gomock.Any(), gomock.Any()).Return(call.subnets, call.err)
			}

			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			builder := NewDefaultModelBuilder(annotationParser, subnetsResolver, "my-cluster", nil)
			ctx := context.Background()
			stack, _, err := builder.Build(ctx, tt.svc)
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
