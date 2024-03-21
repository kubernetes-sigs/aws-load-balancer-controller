package service

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	type resolveSGViaNameOrIDCall struct {
		args []string
		want []string
		err  error
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
		defaultTargetType            string
		enableIPTargetType           *bool
		resolveSGViaNameOrIDCall     []resolveSGViaNameOrIDCall
		backendSecurityGroup         string
		enableBackendSG              bool
		disableRestrictedSGRules     bool
		svc                          *corev1.Service
		wantError                    bool
		wantValue                    string
		wantNumResources             int
		featureGates                 map[config.Feature]bool
	}{
		{
			testName: "Simple service",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip-svc-tls",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                                     "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-inbound-sg-rules-on-private-link-traffic": "on",
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
			wantNumResources:         4,
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
             "securityGroupsInboundRulesOnPrivateLink":"on",
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
                "timeoutSeconds":10,
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
                   "vpcID": "vpc-xxx",
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
		},
		{
			testName: "Dualstack service",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nlb-ip-svc-tls",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                                     "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-ip-address-type":                          "dualstack",
						"service.beta.kubernetes.io/aws-load-balancer-scheme":                                   "internet-facing",
						"service.beta.kubernetes.io/aws-load-balancer-inbound-sg-rules-on-private-link-traffic": "on",
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
             "securityGroupsInboundRulesOnPrivateLink":"on",
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
                "timeoutSeconds":10,
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
                   "vpcID": "vpc-xxx",
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
						"service.beta.kubernetes.io/aws-load-balancer-type":                                     "nlb-ip",
						"service.beta.kubernetes.io/aws-load-balancer-scheme":                                   "internal",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol":                     "HTTP",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":                         "8888",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-path":                         "/healthz",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":                     "10",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":                      "30",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":            "2",
						"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold":          "2",
						"service.beta.kubernetes.io/aws-load-balancer-inbound-sg-rules-on-private-link-traffic": "off",
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
             "securityGroupsInboundRulesOnPrivateLink":"off",
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
                "timeoutSeconds":30,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                "timeoutSeconds":30,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                   "vpcID": "vpc-xxx",
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
                   "vpcID": "vpc-xxx",
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
                "timeoutSeconds":30,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                "timeoutSeconds":30,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                   "vpcID": "vpc-xxx",
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
                   "vpcID": "vpc-xxx",
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
			enableBackendSG: true,
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
                "timeoutSeconds":10,
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
                "timeoutSeconds":10,
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
                   "vpcID": "vpc-xxx",
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
                   "vpcID": "vpc-xxx",
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
                "timeoutSeconds":6,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                "timeoutSeconds":6,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                   "vpcID": "vpc-xxx",
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
                   "vpcID": "vpc-xxx",
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
                "timeoutSeconds":10,
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
                   "vpcID": "vpc-xxx",
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
              "vpcID": "vpc-xxx",
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
            "intervalSeconds": 10,
            "timeoutSeconds":10
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
			testName: "default ip target",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-ip-target",
					Namespace: "default",
					UID:       "7ab4be33-11c2-4a7b-b622-7add8affab36",
				},
				Spec: corev1.ServiceSpec{
					Type:              corev1.ServiceTypeLoadBalancer,
					LoadBalancerClass: aws.String("service.k8s.aws/nlb"),
					Selector:          map[string]string{"app": "hello"},
					Ports: []corev1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			},
			defaultTargetType:        "ip",
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
			wantNumResources: 4,
			wantValue: `
{
  "id": "default/default-ip-target",
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
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/default-ip-target:80/status/targetGroupARN"
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
      "default/default-ip-target:80": {
        "spec": {
          "template": {
            "spec": {
              "targetType": "ip",
              "ipAddressType":"ipv4",
              "vpcID": "vpc-xxx",
              "targetGroupARN": {
                "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/default-ip-target:80/status/targetGroupARN"
              },
              "networking": {
                "ingress": [
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
              },
              "serviceRef": {
                "name": "default-ip-target",
                "port": 80
              }
            },
            "metadata": {
              "creationTimestamp": null,
              "namespace": "default",
              "name": "k8s-default-defaulti-cc40ce9c73"
            }
          }
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::LoadBalancer": {
      "LoadBalancer": {
        "spec": {
          "ipAddressType": "ipv4",
          "name": "k8s-default-defaulti-b44ef5a42d",
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
      "default/default-ip-target:80": {
        "spec": {
          "targetType": "ip",
          "ipAddressType":"ipv4",
          "protocol": "TCP",
          "name": "k8s-default-defaulti-cc40ce9c73",
          "healthCheckConfig": {
            "healthyThresholdCount": 3,
            "unhealthyThresholdCount": 3,
            "protocol": "TCP",
            "port": "traffic-port",
            "intervalSeconds": 10,
            "timeoutSeconds":10
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
			enableBackendSG: false,
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
			enableBackendSG:          true,
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			fetchVPCInfoCalls: []fetchVPCInfoCall{
				{
					err: errors.New("unable to resolve VPC CIDRs"),
				},
			},
			listLoadBalancerCalls: []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
			wantError: true,
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
			enableBackendSG: true,
			wantError:       true,
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
            "timeoutSeconds":10,
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
              "vpcID": "vpc-xxx",
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
            "timeoutSeconds":10,
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
              "vpcID": "vpc-xxx",
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
			featureGates: map[config.Feature]bool{
				config.ServiceTypeLoadBalancerOnly: true,
			},
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
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
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
            "timeoutSeconds":10,
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
              "vpcID": "vpc-xxx",
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
			testName: "with backend SG rule management disabled for legacy nlb",
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
			resolveViaNameOrIDSliceCalls: []resolveViaNameOrIDSliceCall{
				{
					subnets: []*ec2.Subnet{
						{
							SubnetId:  aws.String("subnet-1"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
					},
				},
			},
			listLoadBalancerCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2.LoadBalancerWithTags{
						{
							LoadBalancer: &elbv2sdk.LoadBalancer{
								Scheme: aws.String("internal"),
								AvailabilityZones: []*elbv2sdk.AvailabilityZone{
									{
										SubnetId: aws.String("subnet-1"),
									},
								},
							},
						},
					},
				},
			},
			wantError: false,
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
                "timeoutSeconds":10,
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
                   "vpcID": "vpc-xxx",
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
		{
			testName: "Simple service with NLB SG",
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
			disableRestrictedSGRules: true,
			enableBackendSG:          true,
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			backendSecurityGroup:     "sg-backend",
			wantError:                false,
			wantValue: `
{
 "id":"default/nlb-ip-svc-tls",
 "resources":{
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-default-nlbipsvc-1da4b78715",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                }
             ]
          }
       }
    },
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
             ],
           "securityGroups": [
            {
			   "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
            },
            "sg-backend"
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
                "timeoutSeconds":10,
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
                   "vpcID": "vpc-xxx",
                   "serviceRef":{
                      "name":"nlb-ip-svc-tls",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "securityGroup":{
                                     "groupID":"sg-backend"
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP"
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
			wantNumResources: 5,
		},
		{
			testName: "Dualstack service with SG",
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
			enableBackendSG:          true,
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForOneSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			backendSecurityGroup:     "sg-backend",
			wantError:                false,
			wantValue: `
{
 "id":"default/nlb-ip-svc-tls",
 "resources":{
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-default-nlbipsvc-1da4b78715",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                },
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipv6Ranges": [
                      {
                         "cidrIPv6": "::/0"
                      }
                   ]
                }
             ]
          }
       }
    },
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
           "securityGroups": [
            {
			   "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
            },
            "sg-backend"
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
                "timeoutSeconds":10,
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
                   "vpcID": "vpc-xxx",
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
                                  "securityGroup":{
                                     "groupID":"sg-backend"
                                  }
                               }
                            ],
                            "ports":[
                               {
								  "port": 80,
                                  "protocol":"TCP"
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
			wantNumResources: 5,
		},
		{
			testName: "Multiple listeners, multiple target groups with SG",
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
			enableBackendSG:          true,
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForTwoSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			backendSecurityGroup:     "sg-backend",
			wantError:                false,
			wantValue: `
{
 "id":"default/nlb-ip-svc",
 "resources":{
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-default-nlbipsvc-51de41384e",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                },
                {
                   "ipProtocol": "tcp",
                   "fromPort": 83,
                   "toPort": 83,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                }
             ]
          }
       }
    },
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
             ],
             "securityGroups": [
               {
			     "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
               },
               "sg-backend"
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
                "timeoutSeconds":30,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                "timeoutSeconds":30,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                   "vpcID": "vpc-xxx",
                   "serviceRef":{
                      "name":"nlb-ip-svc",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "securityGroup":{
                                     "groupID":"sg-backend"
                                  }
                               }
                            ],
                            "ports":[
							   {
							      "port": 80,
                                  "protocol":"TCP"
                               },
							   {
							      "port": 8888,
                                  "protocol":"TCP"
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
                   "vpcID": "vpc-xxx",
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
                                  "securityGroup":{
                                     "groupID":"sg-backend"
                                  }
                               }
                            ],
                            "ports":[
                               {
							      "port": 80,
                                  "protocol":"TCP"
                               },
							   {
							      "port": 8888,
                                  "protocol":"TCP"
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
			wantNumResources: 8,
		},
		{
			testName: "TLS and access logging annotations with SG",
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
			enableBackendSG:          true,
			resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForThreeSubnet},
			listLoadBalancerCalls:    []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			backendSecurityGroup:     "sg-backend",
			wantError:                false,
			wantValue: `
{
 "id":"default/nlb-ip-svc-tls",
 "resources":{
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-default-nlbipsvc-1b1762b6f6",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                },
                {
                   "ipProtocol": "tcp",
                   "fromPort": 83,
                   "toPort": 83,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                }
             ]
          }
       }
    },
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
             ],
           "securityGroups": [
            {
			   "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
            },
            "sg-backend"
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
                "timeoutSeconds":30,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                "timeoutSeconds":30,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                   "vpcID": "vpc-xxx",
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
                                  "securityGroup":{
                                     "groupID":"sg-backend"
                                  }
                               }
                            ],
                            "ports":[
                               {
								  "port": 80,
                                  "protocol":"TCP"
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
                   "vpcID": "vpc-xxx",
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
                                  "securityGroup":{
                                     "groupID":"sg-backend"
                                  }
                               }
                            ],
                            "ports":[
                               {
								  "port": 8883,
                                  "protocol":"TCP"
                               },
							   {
								  "port": 80,
                                  "protocol":"TCP"
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
			wantNumResources: 8,
		},
		{
			testName: "Instance mode, external traffic policy cluster with SG",
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
			enableBackendSG:          false,
			disableRestrictedSGRules: true,
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
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-default-instance-38fe420757",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                },
                {
                   "ipProtocol": "tcp",
                   "fromPort": 83,
                   "toPort": 83,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                }
             ]
          }
       }
    },
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
             ],
           "securityGroups": [
            {
			   "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
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
                "timeoutSeconds":10,
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
                "timeoutSeconds":10,
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
                   "vpcID": "vpc-xxx",
                   "serviceRef":{
                      "name":"instance-mode",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "securityGroup":{
									"groupID": {
										"$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
									}
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP"
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
                   "vpcID": "vpc-xxx",
                   "serviceRef":{
                      "name":"instance-mode",
                      "port":83
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "securityGroup":{
                                     "groupID": {
									"$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
								 }
                                  }
                               }
                            ],
                            "ports":[
                               {
                                  "protocol":"TCP"
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
			wantNumResources: 8,
		},
		{
			testName: "Instance mode, external traffic policy local with SG",
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
			enableBackendSG:              false,
			resolveViaNameOrIDSliceCalls: []resolveViaNameOrIDSliceCall{resolveViaNameOrIDSliceCallForThreeSubnet},
			listLoadBalancerCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2.LoadBalancerWithTags{
						{
							LoadBalancer: &elbv2sdk.LoadBalancer{
								Scheme:         aws.String("internet-facing"),
								SecurityGroups: []*string{aws.String("sg-lb")},
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
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-app-trafficl-7b81fc143c",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "10.20.0.0/16"
                      }
                   ]
                },
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "1.2.3.4/19"
                      }
                   ]
                },
                {
                   "ipProtocol": "tcp",
                   "fromPort": 83,
                   "toPort": 83,
                   "ipRanges": [
                      {
                         "cidrIP": "10.20.0.0/16"
                      }
                   ]
                },
                {
                   "ipProtocol": "tcp",
                   "fromPort": 83,
                   "toPort": 83,
                   "ipRanges": [
                      {
                         "cidrIP": "1.2.3.4/19"
                      }
                   ]
                }
             ]
          }
       }
    },
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
             ],
           "securityGroups": [
            {
			   "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
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
                "timeoutSeconds":6,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                "timeoutSeconds":6,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                   "vpcID": "vpc-xxx",
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
                                  "securityGroup":{
                                     "groupID": {
									"$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
								 }
                                  }
                               }
                            ],
                            "ports":[
                               {
								  "port": 31223,
                                  "protocol":"TCP"
                               },
							   {
							      "port": 29123,
                                  "protocol":"TCP"
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
                   "vpcID": "vpc-xxx",
                   "serviceRef":{
                      "name":"traffic-local",
                      "port":83
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "securityGroup":{
									"groupID": {
										"$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
								 	}
                                  }
                               }
                            ],
                            "ports":[
                               {
								  "port": 32323,
                                  "protocol":"TCP"
                               },
							   {
								  "port": 29123,
                                  "protocol":"TCP"
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
			wantNumResources: 8,
		},
		{
			testName: "Instance mode, external traffic policy local, UDP as protocol with SG",
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
							Protocol:   corev1.ProtocolUDP,
							NodePort:   31223,
						},
						{
							Name:       "alt2",
							Port:       83,
							TargetPort: intstr.FromInt(8883),
							Protocol:   corev1.ProtocolUDP,
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
			enableBackendSG:              false,
			resolveViaNameOrIDSliceCalls: []resolveViaNameOrIDSliceCall{resolveViaNameOrIDSliceCallForThreeSubnet},
			listLoadBalancerCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2.LoadBalancerWithTags{
						{
							LoadBalancer: &elbv2sdk.LoadBalancer{
								Scheme:         aws.String("internet-facing"),
								SecurityGroups: []*string{aws.String("sg-lb")},
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
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-app-trafficl-7b81fc143c",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "udp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "10.20.0.0/16"
                      }
                   ]
                },
                {
                   "ipProtocol": "udp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "1.2.3.4/19"
                      }
                   ]
                },
                {
                   "ipProtocol": "udp",
                   "fromPort": 83,
                   "toPort": 83,
                   "ipRanges": [
                      {
                         "cidrIP": "10.20.0.0/16"
                      }
                   ]
                },
                {
                   "ipProtocol": "udp",
                   "fromPort": 83,
                   "toPort": 83,
                   "ipRanges": [
                      {
                         "cidrIP": "1.2.3.4/19"
                      }
                   ]
                }
             ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::Listener":{
       "80":{
          "spec":{
             "loadBalancerARN":{
                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
             },
             "port":80,
             "protocol":"UDP",
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
             "protocol":"UDP",
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
             ],
           "securityGroups": [
            {
			   "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
            }
		 ]
          }
       }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup":{
       "app/traffic-local:80":{
          "spec":{
             "name":"k8s-app-trafficl-5f852f95c2",
             "targetType":"instance",
             "ipAddressType":"ipv4",
             "port":31223,
             "protocol":"UDP",
             "healthCheckConfig":{
                "port": 29123,
                "protocol":"HTTP",
				"path":"/healthz",
                "intervalSeconds":10,
                "timeoutSeconds":6,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
             "name":"k8s-app-trafficl-5eafb7edb6",
             "targetType":"instance",
             "ipAddressType":"ipv4",
             "port":32323,
             "protocol":"UDP",
             "healthCheckConfig":{
                "port": 29123,
                "protocol":"HTTP",
				"path":"/healthz",
                "intervalSeconds":10,
                "timeoutSeconds":6,
                "healthyThresholdCount":2,
                "unhealthyThresholdCount":2,
                "matcher":{
					"httpCode": "200-399"
				}
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
                   "name":"k8s-app-trafficl-5f852f95c2",
                   "namespace":"app",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/app/traffic-local:80/status/targetGroupARN"
                   },
                   "targetType":"instance",
                   "ipAddressType":"ipv4",
                   "vpcID": "vpc-xxx",
                   "serviceRef":{
                      "name":"traffic-local",
                      "port":80
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "securityGroup":{
                                     "groupID": {
									    "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
								     }
                                  }
                               }
                            ],
                            "ports":[
							   {
								  "port": 31223,
                                  "protocol":"UDP"
                               },
							   {
							      "port": 29123,
                                  "protocol":"TCP"
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
                   "name":"k8s-app-trafficl-5eafb7edb6",
                   "namespace":"app",
                   "creationTimestamp":null
                },
                "spec":{
                   "targetGroupARN":{
                      "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/app/traffic-local:83/status/targetGroupARN"
                   },
                   "targetType":"instance",
                   "ipAddressType":"ipv4",
                   "vpcID": "vpc-xxx",
                   "serviceRef":{
                      "name":"traffic-local",
                      "port":83
                   },
                   "networking":{
                      "ingress":[
                         {
                            "from":[
                               {
                                  "securityGroup":{
									"groupID": {
										"$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
								 	}
                                  }
                               }
                            ],
                            "ports":[
							   {
								  "port": 32323,
                                  "protocol":"UDP"
                               },
							   {
								  "port": 29123,
                                  "protocol":"TCP"
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
			wantNumResources: 8,
		},
		{
			testName: "additional resource tags with SG",
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
			enableBackendSG:          true,
			disableRestrictedSGRules: true,
			resolveViaNameOrIDSliceCalls: []resolveViaNameOrIDSliceCall{
				{
					subnets: []*ec2.Subnet{
						{
							SubnetId:  aws.String("subnet-1"),
							CidrBlock: aws.String("192.168.0.0/19"),
						},
					},
				},
			},
			listLoadBalancerCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2.LoadBalancerWithTags{
						{
							LoadBalancer: &elbv2sdk.LoadBalancer{
								Scheme: aws.String("internal"),
							},
						},
					},
				},
			},
			wantError: false,
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
				"timeoutSeconds": 10,
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
                   "vpcID": "vpc-xxx",
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
			testName: "ip target, preserve client IP, scheme internal with SG",
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
			enableBackendSG:          true,
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
			backendSecurityGroup:  "sg-backend",
			listLoadBalancerCalls: []listLoadBalancerCall{listLoadBalancerCallForEmptyLB},
			wantNumResources:      5,
			wantValue: `
{
  "id": "default/ip-target",
  "resources": {
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-default-iptarget-b58c2e5bdf",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                }
             ]
          }
       }
    },
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
              "vpcID": "vpc-xxx",
              "targetGroupARN": {
                "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/ip-target:80/status/targetGroupARN"
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "securityGroup":{
                          "groupID":"sg-backend"
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
          "securityGroups": [
            {
		     "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
            },
            "sg-backend"
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
			"timeoutSeconds": 10,
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
			testName: "ipv6 for NLB with SG",
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
			wantNumResources:         5,
			wantValue: `
{
  "id": "default/traffic-local",
  "resources": {
    "AWS::EC2::SecurityGroup": {
      "ManagedLBSecurityGroup": {
        "spec": {
          "groupName": "k8s-default-trafficl-27d98646ec",
          "description": "[k8s] Managed SecurityGroup for LoadBalancer",
          "ingress": [
            {
              "ipProtocol": "tcp",
              "fromPort": 80,
              "toPort": 80,
              "ipRanges": [
                {
                  "cidrIP": "0.0.0.0/0"
                }
              ]
            },
            {
              "ipProtocol": "tcp",
              "fromPort": 80,
              "toPort": 80,
              "ipv6Ranges": [
                {
                  "cidrIPv6": "::/0"
                }
              ]
            }
          ]
        }
      }
    },
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
          ],
          "securityGroups": [
            {
              "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
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
              "vpcID": "vpc-xxx",
              "serviceRef": {
                "name": "traffic-local",
                "port": 80
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "securityGroup": {
                          "groupID": {
                            "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
                          }
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
			testName: "ipv6 for NLB internet-facing scheme with SG",
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
			wantNumResources:         5,
			wantValue: `
{
  "id": "default/traffic-local",
  "resources": {
    "AWS::EC2::SecurityGroup": {
      "ManagedLBSecurityGroup": {
        "spec": {
          "groupName": "k8s-default-trafficl-27d98646ec",
          "description": "[k8s] Managed SecurityGroup for LoadBalancer",
          "ingress": [
            {
              "ipProtocol": "tcp",
              "fromPort": 80,
              "toPort": 80,
              "ipRanges": [
                {
                  "cidrIP": "0.0.0.0/0"
                }
              ]
            },
            {
              "ipProtocol": "tcp",
              "fromPort": 80,
              "toPort": 80,
              "ipv6Ranges": [
                {
                  "cidrIPv6": "::/0"
                }
              ]
            }
          ]
        }
      }
    },
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
          ],
          "securityGroups": [
            {
              "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
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
              "vpcID": "vpc-xxx",
              "serviceRef": {
                "name": "traffic-local",
                "port": 80
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "securityGroup": {
                          "groupID": {
                            "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
                          }
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
			testName: "spec.loadBalancerClass specified with SG",
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
			wantNumResources: 5,
			wantValue: `
{
  "id": "awesome/lb-with-class",
  "resources": {
    "AWS::EC2::SecurityGroup": {
       "ManagedLBSecurityGroup": {
          "spec": {
             "groupName": "k8s-awesome-lbwithcl-1054401dd7",
             "description": "[k8s] Managed SecurityGroup for LoadBalancer",
             "ingress": [
                {
                   "ipProtocol": "tcp",
                   "fromPort": 80,
                   "toPort": 80,
                   "ipRanges": [
                      {
                         "cidrIP": "0.0.0.0/0"
                      }
                   ]
                }
             ]
          }
       }
    },
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
          ],
          "securityGroups": [
            {
		       "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
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
              "vpcID": "vpc-xxx",
              "serviceRef": {
                "name": "lb-with-class",
                "port": 80
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "securityGroup": {
                          "groupID": {
                            "$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
                          }
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
			testName: "existing NLB with Security group, NLB SG feature disabled",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-svc",
					Namespace: "some-namespace",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-internal":        "true",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
			},
			resolveViaNameOrIDSliceCalls: []resolveViaNameOrIDSliceCall{resolveViaNameOrIDSliceCallForThreeSubnet},
			listLoadBalancerCalls: []listLoadBalancerCall{
				{
					sdkLBs: []elbv2.LoadBalancerWithTags{
						{
							LoadBalancer: &elbv2sdk.LoadBalancer{
								Scheme: aws.String("internal"),
								AvailabilityZones: []*elbv2sdk.AvailabilityZone{
									{
										SubnetId: aws.String("subnet-1"),
									},
								},
								SecurityGroups: []*string{aws.String("sg-lb")},
							},
						},
					},
				},
			},
			featureGates: map[config.Feature]bool{
				config.NLBSecurityGroup: false,
			},
			wantError: true,
		},
		{
			testName: "With security groups annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "manual-security-groups",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":            "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type": "ip",
						"service.beta.kubernetes.io/aws-load-balancer-security-groups": "named security group,sg-id2",
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
			resolveSGViaNameOrIDCall: []resolveSGViaNameOrIDCall{
				{
					args: []string{"named security group", "sg-id2"},
					want: []string{"sg-id1", "sg-id2"},
				},
			},
			wantNumResources: 4,
			wantValue: `
{
  "id": "default/manual-security-groups",
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
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/manual-security-groups:80/status/targetGroupARN"
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
          "name": "k8s-default-manualse-6b0ba8ff70",
          "type": "network",
          "scheme": "internal",
          "ipAddressType": "ipv4",
          "subnetMapping": [
            {
              "subnetID": "subnet-1"
            }
          ],
          "securityGroups": [
            "sg-id1",
            "sg-id2"
          ]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "default/manual-security-groups:80": {
        "spec": {
          "name": "k8s-default-manualse-d4818dcd51",
          "targetType": "ip",
          "port": 80,
          "protocol": "TCP",
          "ipAddressType": "ipv4",
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
      "default/manual-security-groups:80": {
        "spec": {
          "template": {
            "metadata": {
              "name": "k8s-default-manualse-d4818dcd51",
              "namespace": "default",
              "creationTimestamp": null
            },
            "spec": {
              "targetGroupARN": {
                "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/manual-security-groups:80/status/targetGroupARN"
              },
              "targetType": "ip",
              "vpcID": "vpc-xxx",
              "serviceRef": {
                "name": "manual-security-groups",
                "port": 80
              },
              "ipAddressType": "ipv4"
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
			testName: "Manage backend rules with manual security groups",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "manual-security-groups",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                                "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":                     "ip",
						"service.beta.kubernetes.io/aws-load-balancer-security-groups":                     "named security group,sg-id2",
						"service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules": "true",
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
			resolveSGViaNameOrIDCall: []resolveSGViaNameOrIDCall{
				{
					args: []string{"named security group", "sg-id2"},
					want: []string{"sg-id1", "sg-id2"},
				},
			},
			enableBackendSG:      true,
			backendSecurityGroup: "sg-backend",
			wantNumResources:     4,
			wantValue: `
{
  "id": "default/manual-security-groups",
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
                      "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/manual-security-groups:80/status/targetGroupARN"
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
          "name": "k8s-default-manualse-6b0ba8ff70",
          "type": "network",
          "scheme": "internal",
          "ipAddressType": "ipv4",
          "subnetMapping": [
            {
              "subnetID": "subnet-1"
            }
          ],
          "securityGroups": [
            "sg-id1",
            "sg-id2",
            "sg-backend"
          ]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "default/manual-security-groups:80": {
        "spec": {
          "name": "k8s-default-manualse-d4818dcd51",
          "targetType": "ip",
          "port": 80,
          "protocol": "TCP",
          "ipAddressType": "ipv4",
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
      "default/manual-security-groups:80": {
        "spec": {
          "template": {
            "metadata": {
              "name": "k8s-default-manualse-d4818dcd51",
              "namespace": "default",
              "creationTimestamp": null
            },
            "spec": {
              "targetGroupARN": {
                "$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/default/manual-security-groups:80/status/targetGroupARN"
              },
              "targetType": "ip",
              "vpcID": "vpc-xxx",
              "serviceRef": {
                "name": "manual-security-groups",
                "port": 80
              },
              "networking": {
                "ingress": [
                  {
                    "from": [
                      {
                        "securityGroup": {
                          "groupID": "sg-backend"
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
              "ipAddressType": "ipv4"
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
			testName: "Manage backend rules with manual security groups, backend sg not enabled",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "manual-security-groups",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                                "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":                     "ip",
						"service.beta.kubernetes.io/aws-load-balancer-security-groups":                     "named security group,sg-id2",
						"service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules": "true",
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
			resolveSGViaNameOrIDCall: []resolveSGViaNameOrIDCall{
				{
					args: []string{"named security group", "sg-id2"},
					want: []string{"sg-id1", "sg-id2"},
				},
			},
			wantError: true,
		},
		{
			testName: "Manage backend rules with manual security groups, resolve SG error",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "manual-security-groups",
					Namespace: "default",
					UID:       "bdca2bd0-bfc6-449a-88a3-03451f05f18c",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-type":                                "external",
						"service.beta.kubernetes.io/aws-load-balancer-nlb-target-type":                     "ip",
						"service.beta.kubernetes.io/aws-load-balancer-security-groups":                     "named security group,sg-id2",
						"service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules": "true",
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
			resolveSGViaNameOrIDCall: []resolveSGViaNameOrIDCall{
				{
					args: []string{"named security group", "sg-id2"},
					err:  errors.New("unable to resolve security group"),
				},
			},
			wantError: true,
		},
		{
			testName: "ipv6 source ranges, but lb not dual stack",
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
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{"app": "hello"},
					LoadBalancerSourceRanges: []string{
						"2002::1234:abcd:ffff:c0a8:101/64",
					},
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
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)

			subnetsResolver := networking.NewMockSubnetsResolver(ctrl)
			for _, call := range tt.resolveViaDiscoveryCalls {
				subnetsResolver.EXPECT().ResolveViaDiscovery(gomock.Any(), gomock.Any()).Return(call.subnets, call.err)
			}
			for _, call := range tt.resolveViaNameOrIDSliceCalls {
				subnetsResolver.EXPECT().ResolveViaNameOrIDSlice(gomock.Any(), gomock.Any(), gomock.Any()).Return(call.subnets, call.err)
			}
			featureGates := config.NewFeatureGates()
			for key, value := range tt.featureGates {
				if value {
					featureGates.Enable(key)
				} else {
					featureGates.Disable(key)
				}
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
			defaultTargetType := tt.defaultTargetType
			if defaultTargetType == "" {
				defaultTargetType = "instance"
			}
			backendSGProvider := networking.NewMockBackendSGProvider(ctrl)
			if tt.enableBackendSG {
				backendSGProvider.EXPECT().Get(gomock.Any(), networking.ResourceType(networking.ResourceTypeService), gomock.Any()).Return(tt.backendSecurityGroup, nil).AnyTimes()
				backendSGProvider.EXPECT().Release(gomock.Any(), networking.ResourceType(networking.ResourceTypeService), gomock.Any()).Return(nil).AnyTimes()
			}
			sgResolver := networking.NewMockSecurityGroupResolver(ctrl)
			for _, call := range tt.resolveSGViaNameOrIDCall {
				sgResolver.EXPECT().ResolveViaNameOrID(gomock.Any(), call.args).Return(call.want, call.err)
			}
			var enableIPTargetType bool
			if tt.enableIPTargetType == nil {
				enableIPTargetType = true
			} else {
				enableIPTargetType = *tt.enableIPTargetType
			}
			builder := NewDefaultModelBuilder(annotationParser, subnetsResolver, vpcInfoProvider, "vpc-xxx", trackingProvider, elbv2TaggingManager, ec2Client, featureGates,
				"my-cluster", nil, nil, "ELBSecurityPolicy-2016-08", defaultTargetType, enableIPTargetType, serviceUtils,
				backendSGProvider, sgResolver, tt.enableBackendSG, tt.disableRestrictedSGRules, logr.New(&log.NullLogSink{}))
			ctx := context.Background()
			stack, _, _, err := builder.Build(ctx, tt.svc)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
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
