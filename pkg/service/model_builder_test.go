package service

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

func Test_defaultModelBuilderTask_Build(t *testing.T) {
	type resolveViaDiscoveryCall struct {
		subnets []*ec2.Subnet
		err     error
	}
	type resolveViaNameOrIDSliceCall struct {
		subnets []*ec2.Subnet
		err     error
	}
	type listLoadBalancerCall struct {
		sdkLBs []elbv2.LoadBalancerWithTags
		err    error
	}
	type fetchVPCInfoCall struct {
		wantVPCInfo networking.VPCInfo
		err         error
	}
	cidrBlockStateAssociated := ec2.VpcCidrBlockStateCodeAssociated
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
	resolveViaNameOrIDSliceCallForThreeSubnet := resolveViaNameOrIDSliceCall{
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
	listLoadBalancerCallForEmptyLB := listLoadBalancerCall{
		sdkLBs: []elbv2.LoadBalancerWithTags{},
	}
	tests := []struct {
		testName                     string
		resolveViaDiscoveryCalls     []resolveViaDiscoveryCall
		resolveViaNameOrIDSliceCalls []resolveViaNameOrIDSliceCall
		listLoadBalancerCalls        []listLoadBalancerCall
		fetchVPCInfoCalls            []fetchVPCInfoCall
		enableIPTargetType           *bool
		svc                          *corev1.Service
		wantError                    bool
		wantValue                    string
		wantNumResources             int
		restrictToTypeLoadBalancer   bool
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
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
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
             "name":"k8s-default-nlbipsvc-6b0ba8ff70",
             "type":"network",
             "scheme":"internal",
             "ipAddressType":"ipv4",
             "subnetMapping":[
                {
                   "subnetID":"subnet-1"
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
             "ipAddressType":"ipv4",
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
                   "ipAddressType":"ipv4",
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
						"service.beta.kubernetes.io/aws-load-balancer-scheme":          "internet-facing",
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
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
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
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "default/nlb-ip-svc-tls:80":{
          "spec":{
             "name":"k8s-default-nlbipsvc-d4818dcd51",
             "targetType":"ip",
             "ipAddressType":"ipv4",
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
                   "ipAddressType":"ipv4",
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
						"service.beta.kubernetes.io/aws-load-balancer-scheme":                          "internal",
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
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
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
             "name":"k8s-default-nlbipsvc-518cdfc227",
             "type":"network",
             "scheme":"internal",
             "ipAddressType":"ipv4",
             "subnetMapping":[
                {
                   "subnetID":"subnet-1"
                },
                {
                   "subnetID":"subnet-2"
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
             "ipAddressType":"ipv4",
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
             "ipAddressType":"ipv4",
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
                   "ipAddressType":"ipv4",
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
                   "ipAddressType":"ipv4",
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
						"service.beta.kubernetes.io/aws-load-balancer-internal":                          "false",
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
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
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
             "sslPolicy": "ELBSecurityPolicy-2016-08",
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
                   "key":"access_logs.s3.bucket",
                   "value":"nlb-bucket"
                },
                {
                   "key":"access_logs.s3.enabled",
                   "value":"true"
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
             "ipAddressType":"ipv4",
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
             "ipAddressType":"ipv4",
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
                   "ipAddressType":"ipv4",
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
                   "ipAddressType":"ipv4",
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
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-internal":        "true",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
					},
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
		{
			testName: "Instance mode, external traffic policy cluster",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance-mode",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
					},
					UID: "2dc098f0-ae33-4378-af7b-83e2a0424495",
				},
				Spec: corev1.ServiceSpec{
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
					Type:                  corev1.ServiceTypeLoadBalancer,
					Selector:              map[string]string{"app": "hello"},
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   31223,
						},
						{
							Name:       "alt2",
							Port:       83,
							TargetPort: intstr.FromInt(8883),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32323,
						},
					},
				},
			},
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForThreeSubnet},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: networking.VPCInfo{
						CidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
							{
								CidrBlock: aws.String("192.168.0.0/16"),
								CidrBlockState: &ec2.VpcCidrBlockState{
									State: &cidrBlockStateAssociated,
								},
							},
						},
					},
				},
			},
			listLoadBalancerCalls: []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantError:             false,
			wantValue: `
{
 "id":"default/instance-mode",
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
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/instance-mode:80/status/targetGroupARN"
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
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/instance-mode:83/status/targetGroupARN"
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
             "name":"k8s-default-instance-7ca1de7e6c",
             "type":"network",
             "scheme":"internal",
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
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "default/instance-mode:80":{
          "spec":{
             "name":"k8s-default-instance-0c68c79423",
             "targetType":"instance",
             "ipAddressType":"ipv4",
             "port":31223,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port": "traffic-port",
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
       },
       "default/instance-mode:83":{
          "spec":{
             "name":"k8s-default-instance-c200165858",
             "targetType":"instance",
             "ipAddressType":"ipv4",
             "port":32323,
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
       "default/instance-mode:80":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-instance-0c68c79423",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/instance-mode:80/status/targetGroupARN"
                   },
                   "targetType":"instance",
                   "ipAddressType":"ipv4",
                   "serviceRef":{
                      "name":"instance-mode",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/16"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":31223
                               }
                            ]
                         }
                      ]
                   }
                }
             }
          }
       },
       "default/instance-mode:83":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-instance-c200165858",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/instance-mode:83/status/targetGroupARN"
                   },
                   "targetType":"instance",
                   "ipAddressType":"ipv4",
                   "serviceRef":{
                      "name":"instance-mode",
                      "port":83
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"192.168.0.0/16"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":32323
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
			testName: "Instance mode, external traffic policy local",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "traffic-local",
					Namespace: "app",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
					},
					UID: "2dc098f0-ae33-4378-af7b-83e2a0424495",
				},
				Spec: corev1.ServiceSpec{
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
					Type:                  corev1.ServiceTypeLoadBalancer,
					Selector:              map[string]string{"app": "hello"},
					HealthCheckNodePort:   29123,
					LoadBalancerSourceRanges: []string{
						"10.20.0.0/16",
						"1.2.3.4/19",
					},
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   31223,
						},
						{
							Name:       "alt2",
							Port:       83,
							TargetPort: intstr.FromInt(8883),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32323,
						},
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								Hostname: "k8s-existing-nlb",
							},
						},
					},
				},
			},
			resolveViaNameOrIDSliceCalls: []resolveViaNameOrIDSliceCall{resolveViaNameOrIDSliceCallForThreeSubnet},
			listLoadBalancerCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2.LoadBalancerWithTags{
						{
							LoadBalancer: &elbv2sdk.LoadBalancer{
								Scheme: aws.String("internet-facing"),
							},
						},
					},
				},
			},
			wantError: false,
			wantValue: `
{
 "id":"app/traffic-local",
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
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/app/traffic-local:80/status/targetGroupARN"
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
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/app/traffic-local:83/status/targetGroupARN"
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
             "name":"k8s-app-trafficl-2af705447d",
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
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "app/traffic-local:80":{
          "spec":{
             "name":"k8s-app-trafficl-d2b8571b2f",
             "targetType":"instance",
             "ipAddressType":"ipv4",
             "port":31223,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port": 29123,
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
       "app/traffic-local:83":{
          "spec":{
             "name":"k8s-app-trafficl-4be0ac1fb8",
             "targetType":"instance",
             "ipAddressType":"ipv4",
             "port":32323,
             "protocol":"TCP",
             "healthCheckConfig":{
                "port": 29123,
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
       "app/traffic-local:80":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-app-trafficl-d2b8571b2f",
                   "namespace":"app",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/app/traffic-local:80/status/targetGroupARN"
                   },
                   "targetType":"instance",
                   "ipAddressType":"ipv4",
                   "serviceRef":{
                      "name":"traffic-local",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"10.20.0.0/16"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"1.2.3.4/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":31223
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
                                  "port":29123
                               }
                            ]
                         }
                      ]
                   }
                }
             }
          }
       },
       "app/traffic-local:83":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-app-trafficl-4be0ac1fb8",
                   "namespace":"app",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/app/traffic-local:83/status/targetGroupARN"
                   },
                   "targetType":"instance",
                   "ipAddressType":"ipv4",
                   "serviceRef":{
                      "name":"traffic-local",
                      "port":83
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "ipBlock":{
                                     "cidr":"10.20.0.0/16"
                                  }
                               },
                               {
                                  "ipBlock":{
                                     "cidr":"1.2.3.4/19"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP",
                                  "port":32323
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
                                  "port":29123
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
			testName: "additional resource tags",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip-svc-tls",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                     "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "resource.tag1=value1,tag2/purpose=test.2",
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
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
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
             "tags": {
               "tag2/purpose": "test.2",
               "resource.tag1": "value1"
             },
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
             "name":"k8s-default-nlbipsvc-6b0ba8ff70",
             "type":"network",
             "scheme":"internal",
             "ipAddressType":"ipv4",
             "subnetMapping":[
                {
                   "subnetID":"subnet-1"
                }
             ],
             "tags": {
               "tag2/purpose": "test.2",
               "resource.tag1": "value1"
             }
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "default/nlb-ip-svc-tls:80":{
          "spec":{
             "name":"k8s-default-nlbipsvc-d4818dcd51",
             "targetType":"ip",
             "ipAddressType":"ipv4",
             "port":80,
             "protocol":"TCP",
             "tags": {
               "tag2/purpose": "test.2",
               "resource.tag1": "value1"
             },
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
                   "ipAddressType":"ipv4",
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
			testName: "ip target, preserve client IP, scheme internal",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ip-target",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                    "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":         "ip",
						"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": "preserve_client_ip.enabled=true",
					},
					UID: "7ab4be33-11c2-4a7b-b622-7add8affab36",
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
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: networking.VPCInfo{
						CidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
							{
								CidrBlock: aws.String("192.160.0.0/16"),
								CidrBlockState: &ec2.VpcCidrBlockState{
									State: &cidrBlockStateAssociated,
								},
							},
							{
								CidrBlock: aws.String("100.64.0.0/16"),
								CidrBlockState: &ec2.VpcCidrBlockState{
									State: &cidrBlockStateAssociated,
								},
							},
						},
					},
				},
			},
			listLoadBalancerCalls: []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantNumResources:      4,
			wantValue: `
{
  "id": "default/ip-target",
  "resources": {
    "AWS::ElasticLoadBalancingV2::Listener": {
      "80": {
        "spec": {
          "protocol": "TCP",
          "defaultActions": [
            {
              "forwardConfig": {
                "targetGroups": [
                  {
                    "targetGroupARN": {
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/ip-target:80/status/targetGroupARN"
                    }
                  }
                ]
              },
              "type": "forward"
            }
          ],
          "loadBalancerARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
          },
          "port": 80
        }
      }
    },
    "K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
      "default/ip-target:80": {
        "spec": {
          "template": {
            "spec": {
              "targetType": "ip",
              "ipAddressType":"ipv4",
              "targetGroupARN": {
                "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/ip-target:80/status/targetGroupARN"
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "ipBlock": {
                          "cidr": "192.160.0.0/16"
                        }
                      },
                      {
                        "ipBlock": {
                          "cidr": "100.64.0.0/16"
                        }
                      }
                    ],
                    "ports": [
                      {
                        "protocol": "TCP",
                        "port": 80
                      }
                    ]
                  }
                ]
              },
              "serviceRef": {
                "name": "ip-target",
                "port": 80
              }
            },
            "metadata": {
              "creationTimestamp": null,
              "namespace": "default",
              "name": "k8s-default-iptarget-cc40ce9c73"
            }
          }
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::LoadBalancer": {
      "LoadBalancer": {
        "spec": {
          "ipAddressType": "ipv4",
          "name": "k8s-default-iptarget-b44ef5a42d",
          "subnetMapping": [
            {
              "subnetID": "subnet-1"
            }
          ],
          "scheme": "internal",
          "type": "network"
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "default/ip-target:80": {
        "spec": {
          "targetType": "ip",
          "ipAddressType":"ipv4",
          "protocol": "TCP",
          "name": "k8s-default-iptarget-cc40ce9c73",
          "healthCheckConfig": {
            "healthyThresholdCount": 3,
            "unhealthyThresholdCount": 3,
            "protocol": "TCP",
            "port": "traffic-port",
            "intervalSeconds": 10
          },
          "targetGroupAttributes": [
            {
              "value": "true",
              "key": "preserve_client_ip.enabled"
            },
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
		},
		{
			testName:           "service with enableIPTargetType set to false and type IP",
			enableIPTargetType: aws.Bool(false),
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "traffic-local",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:       corev1.ServiceTypeLoadBalancer,
					Selector:   map[string]string{"app": "hello"},
					IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32332,
						},
					},
				},
			},
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantError:                true,
		},
		{
			testName: "list load balancers error",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "traffic-local",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								Hostname: "k8s-existing-nlb",
							},
						},
					},
				},
			},
			listLoadBalancerCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2.LoadBalancerWithTags{},
					err:    errors.New("error listing load balancer"),
				},
			},
			wantError: true,
		},
		{
			testName: "resolve VPC CIDRs error",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "traffic-local",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
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
							NodePort:   32332,
						},
					},
				},
			},
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					err: errors.New("unable to resolve VPC CIDRs"),
				},
			},
			listLoadBalancerCalls: []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantError:             true,
		},
		{
			testName: "deletion protection enabled error",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hello-svc",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
						"service.beta.kubernetes.io/aws-load-balancer-attributes":      "deletion_protection.enabled=true",
					},
					Finalizers: []string{"service.k8s.aws/resources"},
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
			wantError: true,
		},
		{
			testName: "ipv6 service without dualstask",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "traffic-local",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:       corev1.ServiceTypeLoadBalancer,
					Selector:   map[string]string{"app": "hello"},
					IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32332,
						},
					},
				},
			},
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantError:                true,
		},
		{
			testName: "ipv6 for NLB",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "traffic-local",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
						"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:       corev1.ServiceTypeLoadBalancer,
					Selector:   map[string]string{"app": "hello"},
					IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32332,
						},
					},
				},
			},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: networking.VPCInfo{
						Ipv6CidrBlockAssociationSet: []*ec2.VpcIpv6CidrBlockAssociation{
							{
								Ipv6CidrBlock: aws.String("2600:1fe3:3c0:1d00::/56"),
								Ipv6CidrBlockState: &ec2.VpcCidrBlockState{
									State: &cidrBlockStateAssociated,
								},
							},
						},
					},
				},
			},
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantNumResources:         4,
			wantValue: `
{
  "id": "default/traffic-local",
  "resources": {
    "AWS::ElasticLoadBalancingV2::Listener": {
      "80": {
        "spec": {
          "loadBalancerARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
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
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/traffic-local:80/status/targetGroupARN"
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
      "LoadBalancer": {
        "spec": {
          "name": "k8s-default-trafficl-6652458428",
          "type": "network",
          "scheme": "internal",
          "ipAddressType": "dualstack",
          "subnetMapping": [
            {
              "subnetID": "subnet-1"
            }
          ]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "default/traffic-local:80": {
        "spec": {
          "name": "k8s-default-trafficl-060a48475d",
          "targetType": "instance",
          "port": 32332,
          "protocol": "TCP",
          "ipAddressType": "ipv6",
          "healthCheckConfig": {
            "port": "traffic-port",
            "protocol": "TCP",
            "intervalSeconds": 10,
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
      "default/traffic-local:80": {
        "spec": {
          "template": {
            "metadata": {
              "name": "k8s-default-trafficl-060a48475d",
              "namespace": "default",
              "creationTimestamp": null
            },
            "spec": {
              "targetGroupARN": {
                "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/traffic-local:80/status/targetGroupARN"
              },
              "targetType": "instance",
              "serviceRef": {
                "name": "traffic-local",
                "port": 80
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "ipBlock": {
                          "cidr": "2600:1fe3:3c0:1d00::/56"
                        }
                      }
                    ],
                    "ports": [
                      {
                        "protocol": "TCP",
                        "port": 32332
                      }
                    ]
                  }
                ]
              },
              "ipAddressType": "ipv6"
            }
          }
        }
      }
    }
  }
}
`,
		},
		{
			testName: "ipv6 for NLB internet-facing scheme",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "traffic-local",
					Namespace: "default",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "instance",
						"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
						"service.beta.kubernetes.io/aws-load-balancer-scheme":          "internet-facing",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:       corev1.ServiceTypeLoadBalancer,
					Selector:   map[string]string{"app": "hello"},
					IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32332,
						},
					},
				},
			},
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantNumResources:         4,
			wantValue: `
{
  "id": "default/traffic-local",
  "resources": {
    "AWS::ElasticLoadBalancingV2::Listener": {
      "80": {
        "spec": {
          "loadBalancerARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
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
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/traffic-local:80/status/targetGroupARN"
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
      "LoadBalancer": {
        "spec": {
          "name": "k8s-default-trafficl-579592c587",
          "type": "network",
          "scheme": "internet-facing",
          "ipAddressType": "dualstack",
          "subnetMapping": [
            {
              "subnetID": "subnet-1"
            }
          ]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "default/traffic-local:80": {
        "spec": {
          "name": "k8s-default-trafficl-060a48475d",
          "targetType": "instance",
          "port": 32332,
          "protocol": "TCP",
          "ipAddressType": "ipv6",
          "healthCheckConfig": {
            "port": "traffic-port",
            "protocol": "TCP",
            "intervalSeconds": 10,
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
      "default/traffic-local:80": {
        "spec": {
          "template": {
            "metadata": {
              "name": "k8s-default-trafficl-060a48475d",
              "namespace": "default",
              "creationTimestamp": null
            },
            "spec": {
              "targetGroupARN": {
                "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/traffic-local:80/status/targetGroupARN"
              },
              "targetType": "instance",
              "serviceRef": {
                "name": "traffic-local",
                "port": 80
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "ipBlock": {
                          "cidr": "::/0"
                        }
                      }
                    ],
                    "ports": [
                      {
                        "protocol": "TCP",
                        "port": 32332
                      }
                    ]
                  }
                ]
              },
              "ipAddressType": "ipv6"
            }
          }
        }
      }
    }
  }
}
`,
		},
		{
			testName: "service type NodePort, restrict to LoadBalancer enabled",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "type-nodeport",
					Namespace: "some-namespace",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-internal":        "true",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeNodePort,
				},
			},
			restrictToTypeLoadBalancer: true,
			wantValue: `
{
"id": "some-namespace/type-nodeport",
"resources": {}
}
`,
		},
		{
			testName: "service type LoadBalancer, no lb type annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "type-loadbalancer",
					Namespace: "some-namespace",
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeNodePort,
				},
			},
			wantValue: `
{
"id": "some-namespace/type-loadbalancer",
"resources": {}
}
`,
		},
		{
			testName: "spec.loadBalancerClass specified",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lb-with-class",
					Namespace: "awesome",
				},
				Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: aws.String("service.k8s.aws/nlb"),
					Selector:          map[string]string{"app": "class"},
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   32110,
						},
					},
				},
			},
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					wantVPCInfo: networking.VPCInfo{
						CidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
							{
								CidrBlock: aws.String("192.168.0.0/16"),
								CidrBlockState: &ec2.VpcCidrBlockState{
									State: &cidrBlockStateAssociated,
								},
							},
						},
					},
				},
			},
			wantNumResources: 4,
			wantValue: `
{
  "id": "awesome/lb-with-class",
  "resources": {
    "AWS::ElasticLoadBalancingV2::Listener": {
      "80": {
        "spec": {
          "loadBalancerARN": {
            "$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
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
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/awesome/lb-with-class:80/status/targetGroupARN"
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
      "LoadBalancer": {
        "spec": {
          "name": "k8s-awesome-lbwithcl-6652458428",
          "type": "network",
          "scheme": "internal",
          "ipAddressType": "ipv4",
          "subnetMapping": [
            {
              "subnetID": "subnet-1"
            }
          ]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "awesome/lb-with-class:80": {
        "spec": {
          "name": "k8s-awesome-lbwithcl-081c7df2ca",
          "targetType": "instance",
          "port": 32110,
          "protocol": "TCP",
          "ipAddressType": "ipv4",
          "healthCheckConfig": {
            "port": "traffic-port",
            "protocol": "TCP",
            "intervalSeconds": 10,
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
      "awesome/lb-with-class:80": {
        "spec": {
          "template": {
            "metadata": {
              "name": "k8s-awesome-lbwithcl-081c7df2ca",
              "namespace": "awesome",
              "creationTimestamp": null
            },
            "spec": {
              "targetGroupARN": {
                "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/awesome/lb-with-class:80/status/targetGroupARN"
              },
              "targetType": "instance",
              "serviceRef": {
                "name": "lb-with-class",
                "port": 80
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "ipBlock": {
                          "cidr": "192.168.0.0/16"
                        }
                      }
                    ],
                    "ports": [
                      {
                        "protocol": "TCP",
                        "port": 32110
                      }
                    ]
                  }
                ]
              },
              "ipAddressType": "ipv4"
            }
          }
        }
      }
    }
  }
}`,
		},
		{
			testName: "with backend SG rule management disabled",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "manual-sg-rule",
					Namespace: "default",
					UID:       "c93458ad-9ef5-4c4c-bc0b-b31599ff585b",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                                "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":                     "ip",
						"service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules": "false",
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
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantError:                false,
			wantValue: `
{
 "id":"default/manual-sg-rule",
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
                               "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/manual-sg-rule:80/status/targetGroupARN"
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
             "name":"k8s-default-manualsg-7af4592f28",
             "type":"network",
             "scheme":"internal",
             "ipAddressType":"ipv4",
             "subnetMapping":[
                {
                   "subnetID":"subnet-1"
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "default/manual-sg-rule:80":{
          "spec":{
             "name":"k8s-default-manualsg-4f421e4c8d",
             "targetType":"ip",
             "ipAddressType":"ipv4",
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
       "default/manual-sg-rule:80":{
          "spec":{
             "template":{
                "metadata":{
                   "name":"k8s-default-manualsg-4f421e4c8d",
                   "namespace":"default",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/manual-sg-rule:80/status/targetGroupARN"
                   },
                   "targetType":"ip",
                   "ipAddressType":"ipv4",
                   "serviceRef":{
                      "name":"manual-sg-rule",
                      "port":80
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
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			subnetsResolver := networking.NewMockSubnetsResolver(ctrl)
			for _, call := range tt.resolveViaDiscoveryCalls {
				subnetsResolver.EXPECT().ResolveViaDiscovery(gomock.Any(), gomock.Any()).Return(call.subnets, call.err)
			}
			for _, call := range tt.resolveViaNameOrIDSliceCalls {
				subnetsResolver.EXPECT().ResolveViaNameOrIDSlice(gomock.Any(), gomock.Any(), gomock.Any()).Return(call.subnets, call.err)
			}
			featureGates := config.NewFeatureGates()
			if tt.restrictToTypeLoadBalancer {
				featureGates.Enable(config.ServiceTypeLoadBalancerOnly)
			}
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			trackingProvider := tracking.NewDefaultProvider("service.k8s.aws", "my-cluster")

			elbv2TaggingManager := elbv2.NewMockTaggingManager(ctrl)
			for _, call := range tt.listLoadBalancerCalls {
				elbv2TaggingManager.EXPECT().ListLoadBalancers(gomock.Any(), gomock.Any()).Return(call.sdkLBs, call.err)
			}
			vpcInfoProvider := networking.NewMockVPCInfoProvider(ctrl)
			for _, call := range tt.fetchVPCInfoCalls {
				vpcInfoProvider.EXPECT().FetchVPCInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(call.wantVPCInfo, call.err).AnyTimes()
			}
			serviceUtils := NewServiceUtils(annotationParser, "service.k8s.aws/resources", "service.k8s.aws/nlb", featureGates)
			var enableIPTargetType bool
			if tt.enableIPTargetType == nil {
				enableIPTargetType = true
			} else {
				enableIPTargetType = *tt.enableIPTargetType
			}
			builder := NewDefaultModelBuilder(annotationParser, subnetsResolver, vpcInfoProvider, "vpc-xxx", trackingProvider, elbv2TaggingManager, featureGates,
				"my-cluster", nil, nil, "ELBSecurityPolicy-2016-08", enableIPTargetType, serviceUtils)
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
