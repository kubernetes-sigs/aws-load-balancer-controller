package ingress

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	networkingpkg "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

const baseStackJSON = `
{
    "id":"ns-1/ing-1",
    "resources":{
        "AWS::EC2::SecurityGroup":{
            "ManagedLBSecurityGroup":{
                "spec":{
                    "groupName":"k8s-ns1-ing1-bd83176788",
                    "description":"[k8s] Managed SecurityGroup for LoadBalancer",
                    "ingress":[
                        {
                            "ipProtocol":"tcp",
                            "fromPort":80,
                            "toPort":80,
                            "ipRanges":[
                                {
                                    "cidrIP":"0.0.0.0/0"
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
                    "protocol":"HTTP",
                    "defaultActions":[
                        {
                            "type":"fixed-response",
                            "fixedResponseConfig":{
                                "contentType":"text/plain",
                                "statusCode":"404"
                            }
                        }
                    ]
                }
            }
        },
        "AWS::ElasticLoadBalancingV2::ListenerRule":{
            "80:1":{
                "spec":{
                    "listenerARN":{
                        "$ref":"#/resources/AWS::ElasticLoadBalancingV2::Listener/80/status/listenerARN"
                    },
                    "priority":1,
                    "actions":[
                        {
                            "type":"forward",
                            "forwardConfig":{
                                "targetGroups":[
                                    {
                                        "targetGroupARN":{
                                            "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-1:http/status/targetGroupARN"
                                        }
                                    }
                                ]
                            }
                        }
                    ],
                    "conditions":[
                        {
                            "field":"host-header",
                            "hostHeaderConfig":{
                                "values":[
                                    "app-1.example.com"
                                ]
                            }
                        },
                        {
                            "field":"path-pattern",
                            "pathPatternConfig":{
                                "values":[
                                    "/svc-1"
                                ]
                            }
                        }
                    ]
                }
            },
            "80:2":{
                "spec":{
                    "listenerARN":{
                        "$ref":"#/resources/AWS::ElasticLoadBalancingV2::Listener/80/status/listenerARN"
                    },
                    "priority":2,
                    "actions":[
                        {
                            "type":"forward",
                            "forwardConfig":{
                                "targetGroups":[
                                    {
                                        "targetGroupARN":{
                                            "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-2:http/status/targetGroupARN"
                                        }
                                    }
                                ]
                            }
                        }
                    ],
                    "conditions":[
                        {
                            "field":"host-header",
                            "hostHeaderConfig":{
                                "values":[
                                    "app-1.example.com"
                                ]
                            }
                        },
                        {
                            "field":"path-pattern",
                            "pathPatternConfig":{
                                "values":[
                                    "/svc-2"
                                ]
                            }
                        }
                    ]
                }
            },
            "80:3":{
                "spec":{
                    "listenerARN":{
                        "$ref":"#/resources/AWS::ElasticLoadBalancingV2::Listener/80/status/listenerARN"
                    },
                    "priority":3,
                    "actions":[
                        {
                            "type":"forward",
                            "forwardConfig":{
                                "targetGroups":[
                                    {
                                        "targetGroupARN":{
                                            "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-3:https/status/targetGroupARN"
                                        }
                                    }
                                ]
                            }
                        }
                    ],
                    "conditions":[
                        {
                            "field":"host-header",
                            "hostHeaderConfig":{
                                "values":[
                                    "app-2.example.com"
                                ]
                            }
                        },
                        {
                            "field":"path-pattern",
                            "pathPatternConfig":{
                                "values":[
                                    "/svc-3"
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
                    "name":"k8s-ns1-ing1-b7e914000d",
                    "type":"application",
                    "scheme":"internal",
                    "ipAddressType":"ipv4",
                    "subnetMapping":[
                        {
                            "subnetID":"subnet-a"
                        },
                        {
                            "subnetID":"subnet-b"
                        }
                    ],
                    "securityGroups":[
                        {
                            "$ref":"#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
                        },
						"sg-auto"
                    ]
                }
            }
        },
        "AWS::ElasticLoadBalancingV2::TargetGroup":{
            "ns-1/ing-1-svc-1:http":{
                "spec":{
                    "name":"k8s-ns1-svc1-9889425938",
                    "targetType":"instance",
                    "ipAddressType":"ipv4",
                    "port":32768,
                    "protocol":"HTTP",
					"protocolVersion":"HTTP1",
                    "healthCheckConfig":{
                        "port":"traffic-port",
                        "protocol":"HTTP",
                        "path":"/",
                        "matcher":{
                            "httpCode":"200"
                        },
                        "intervalSeconds":15,
                        "timeoutSeconds":5,
                        "healthyThresholdCount":2,
                        "unhealthyThresholdCount":2
                    }
                }
            },
            "ns-1/ing-1-svc-2:http":{
                "spec":{
                    "name":"k8s-ns1-svc2-9889425938",
                    "targetType":"instance",
                    "ipAddressType":"ipv4",
                    "port":32768,
                    "protocol":"HTTP",
					"protocolVersion":"HTTP1",
                    "healthCheckConfig":{
                        "port":"traffic-port",
                        "protocol":"HTTP",
                        "path":"/",
                        "matcher":{
                            "httpCode":"200"
                        },
                        "intervalSeconds":15,
                        "timeoutSeconds":5,
                        "healthyThresholdCount":2,
                        "unhealthyThresholdCount":2
                    }
                }
            },
            "ns-1/ing-1-svc-3:https":{
                "spec":{
                    "name":"k8s-ns1-svc3-bf42870fba",
                    "targetType":"ip",
                    "ipAddressType":"ipv4",
                    "port":8443,
                    "protocol":"HTTPS",
					"protocolVersion":"HTTP1",
                    "healthCheckConfig":{
                        "port":9090,
                        "protocol":"HTTPS",
                        "path":"/health-check",
                        "matcher":{
                            "httpCode":"200-300"
                        },
                        "intervalSeconds":20,
                        "timeoutSeconds":10,
                        "healthyThresholdCount":7,
                        "unhealthyThresholdCount":5
                    }
                }
            }
        },
        "K8S::ElasticLoadBalancingV2::TargetGroupBinding":{
            "ns-1/ing-1-svc-1:http":{
                "spec":{
                    "template":{
                        "metadata":{
                            "name":"k8s-ns1-svc1-9889425938",
                            "namespace":"ns-1",
                            "creationTimestamp":null
                        },
                        "spec":{
                            "targetGroupARN":{
                                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-1:http/status/targetGroupARN"
                            },
                            "targetType":"instance",
                            "vpcID": "vpc-dummy",
                            "ipAddressType":"ipv4",
                            "serviceRef":{
                                "name":"svc-1",
                                "port":"http"
                            },
                            "networking":{
                                "ingress":[
                                    {
                                        "from":[
                                            {
                                                "securityGroup":{
                                                    "groupID": "sg-auto"
                                                }
                                            }
                                        ],
                                        "ports":[
                                            {
												"port":32768,
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
            "ns-1/ing-1-svc-2:http":{
                "spec":{
                    "template":{
                        "metadata":{
                            "name":"k8s-ns1-svc2-9889425938",
                            "namespace":"ns-1",
                            "creationTimestamp":null
                        },
                        "spec":{
                            "targetGroupARN":{
                                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-2:http/status/targetGroupARN"
                            },
                            "targetType":"instance",
                            "ipAddressType":"ipv4",
                            "vpcID": "vpc-dummy",
                            "serviceRef":{
                                "name":"svc-2",
                                "port":"http"
                            },
                            "networking":{
                                "ingress":[
                                    {
                                        "from":[
                                            {
                                                "securityGroup":{
                                                    "groupID": "sg-auto"
                                                }
                                            }
                                        ],
                                        "ports":[
                                            {
												"port":32768,
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
            "ns-1/ing-1-svc-3:https":{
                "spec":{
                    "template":{
                        "metadata":{
                            "name":"k8s-ns1-svc3-bf42870fba",
                            "namespace":"ns-1",
                            "creationTimestamp":null
                        },
                        "spec":{
                            "targetGroupARN":{
                                "$ref":"#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-3:https/status/targetGroupARN"
                            },
                            "targetType":"ip",
                            "vpcID": "vpc-dummy",
                            "ipAddressType":"ipv4",
                            "serviceRef":{
                                "name":"svc-3",
                                "port":"https"
                            },
                            "networking":{
                                "ingress":[
                                    {
                                        "from":[
                                            {
                                                "securityGroup":{
                                                    "groupID": "sg-auto"
                                                }
                                            }
                                        ],
                                        "ports":[
                                            {
												"port": 8443,
                                                "protocol":"TCP"
                                            }
                                        ]
                                    },
									{
                                        "from":[
                                            {
                                                "securityGroup":{
                                                    "groupID": "sg-auto"
                                                }
                                            }
                                        ],
                                        "ports":[
                                            {
												"port": 9090,
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
}`

func Test_defaultModelBuilder_Build(t *testing.T) {
	type resolveViaDiscoveryCall struct {
		subnets []ec2types.Subnet
		err     error
	}
	type env struct {
		svcs []*corev1.Service
	}
	type listLoadBalancersCall struct {
		matchedLBs []elbv2.LoadBalancerWithTags
		err        error
	}
	type describeSecurityGroupsResult struct {
		securityGroups []ec2types.SecurityGroup
		err            error
	}
	type fields struct {
		resolveViaDiscoveryCalls     []resolveViaDiscoveryCall
		listLoadBalancersCalls       []listLoadBalancersCall
		describeSecurityGroupsResult []describeSecurityGroupsResult
		backendSecurityGroup         string
		enableBackendSG              bool
	}
	type args struct {
		ingGroup Group
	}

	ns_1_svc_1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-1",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					NodePort:   32768,
				},
			},
		},
	}
	ns_1_svc_2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-2",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/target-type":                  "instance",
				"alb.ingress.kubernetes.io/backend-protocol":             "HTTP",
				"alb.ingress.kubernetes.io/healthcheck-protocol":         "HTTP",
				"alb.ingress.kubernetes.io/healthcheck-port":             "traffic-port",
				"alb.ingress.kubernetes.io/healthcheck-path":             "/",
				"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "15",
				"alb.ingress.kubernetes.io/healthcheck-timeout-seconds":  "5",
				"alb.ingress.kubernetes.io/healthy-threshold-count":      "2",
				"alb.ingress.kubernetes.io/unhealthy-threshold-count":    "2",
				"alb.ingress.kubernetes.io/success-codes":                "200",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					NodePort:   32768,
				},
			},
		},
	}
	ns_1_svc_3 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-3",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/target-type":                  "ip",
				"alb.ingress.kubernetes.io/backend-protocol":             "HTTPS",
				"alb.ingress.kubernetes.io/healthcheck-protocol":         "HTTPS",
				"alb.ingress.kubernetes.io/healthcheck-port":             "9090",
				"alb.ingress.kubernetes.io/healthcheck-path":             "/health-check",
				"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "20",
				"alb.ingress.kubernetes.io/healthcheck-timeout-seconds":  "10",
				"alb.ingress.kubernetes.io/healthy-threshold-count":      "7",
				"alb.ingress.kubernetes.io/unhealthy-threshold-count":    "5",
				"alb.ingress.kubernetes.io/success-codes":                "200-300",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
					NodePort:   32768,
				},
			},
		},
	}

	ns_1_svc_ipv6 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-ipv6",
		},
		Spec: corev1.ServiceSpec{
			IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
					NodePort:   32768,
				},
			},
		},
	}

	svcWithNamedTargetPort := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-1",
			Name:      "svc-named-targetport",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromString("target-port"),
					NodePort:   32768,
				},
			},
		},
	}

	resolveViaDiscoveryCallForInternalLB := resolveViaDiscoveryCall{
		subnets: []ec2types.Subnet{
			{
				SubnetId:  awssdk.String("subnet-a"),
				CidrBlock: awssdk.String("192.168.0.0/19"),
			},
			{
				SubnetId:  awssdk.String("subnet-b"),
				CidrBlock: awssdk.String("192.168.32.0/19"),
			},
		},
	}
	resolveViaDiscoveryCallForInternetFacingLB := resolveViaDiscoveryCall{
		subnets: []ec2types.Subnet{
			{
				SubnetId:  awssdk.String("subnet-c"),
				CidrBlock: awssdk.String("192.168.64.0/19"),
			},
			{
				SubnetId:  awssdk.String("subnet-d"),
				CidrBlock: awssdk.String("192.168.96.0/19"),
			},
		},
	}

	listLoadBalancerCallForEmptyLB := listLoadBalancersCall{
		matchedLBs: []elbv2.LoadBalancerWithTags{},
	}

	tests := []struct {
		name                      string
		env                       env
		defaultTargetType         string
		defaultLoadBalancerScheme string
		enableIPTargetType        *bool
		args                      args
		fields                    fields
		wantStackPatch            string
		wantErr                   string
	}{
		{
			name: "Ingress - vanilla internal",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: "{}",
		},
		{
			name: "Ingress - backend SG feature disabled",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          false,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {
			"LoadBalancer": {
				"spec": {
					"securityGroups": [
						{
							"$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"
						}
					]
				}
			}
		},
		"K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
			"ns-1/ing-1-svc-1:http": {
				"spec": {
					"template": {
						"spec": {
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
												"port": 32768,
												"protocol": "TCP"
											}
										]
									}
								]
							}
						}
					}
				}
			},
			"ns-1/ing-1-svc-2:http": {
				"spec": {
					"template": {
						"spec": {
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
												"port": 32768,
												"protocol": "TCP"
											}
										]
									}
								]
							}
						}
					}
				}
			},
			"ns-1/ing-1-svc-3:https": {
				"spec": {
					"template": {
						"spec": {
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
												"port": 8443,
												"protocol": "TCP"
											}
										]
									},
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
												"port": 9090,
												"protocol": "TCP"
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
}`,
		},
		{
			name: "Ingress - vanilla internet-facing",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternetFacingLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/scheme": "internet-facing",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {
			"LoadBalancer": {
				"spec": {
					"name": "k8s-ns1-ing1-159dd7a143",
					"scheme": "internet-facing",
					"subnetMapping": [
						{
							"subnetID": "subnet-c"
						},
						{
							"subnetID": "subnet-d"
						}
					]
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - using acm and internet-facing",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternetFacingLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/scheme":                "internet-facing",
									"alb.ingress.kubernetes.io/certificate-arn":       "arn:aws:acm:us-east-1:9999999:certificate/22222222,arn:aws:acm:us-east-1:9999999:certificate/33333333,arn:aws:acm:us-east-1:9999999:certificate/11111111,,arn:aws:acm:us-east-1:9999999:certificate/11111111",
									"alb.ingress.kubernetes.io/mutual-authentication": `[{"port":443,"mode":"off"}]`,
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 443,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "0.0.0.0/0"
								}
							],
							"toPort": 443
						}
					]
				}
			}
		},
		"AWS::ElasticLoadBalancingV2::Listener": {
			"443": {
				"spec": {
					"certificates": [
						{
							"certificateARN": "arn:aws:acm:us-east-1:9999999:certificate/22222222"
						},
						{
							"certificateARN": "arn:aws:acm:us-east-1:9999999:certificate/33333333"
						},
						{
							"certificateARN": "arn:aws:acm:us-east-1:9999999:certificate/11111111"
						}
					],
					"defaultActions": [
						{
							"fixedResponseConfig": {
								"contentType": "text/plain",
								"statusCode": "404"
							},
							"type": "fixed-response"
						}
					],
					"loadBalancerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
					},
					"port": 443,
					"protocol": "HTTPS",
					"sslPolicy": "ELBSecurityPolicy-2016-08",
                    "mutualAuthentication" : {
						"mode" : "off",
                        "trustStoreArn": ""
					}
				}
			},
			"80": null
		},
		"AWS::ElasticLoadBalancingV2::ListenerRule": {
			"443:1": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-1:http/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-1.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-1"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 1
				}
			},
			"443:2": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-2:http/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-1.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-2"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 2
				}
			},
			"443:3": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-3:https/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-2.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-3"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 3
				}
			},
			"80:1": null,
			"80:2": null,
			"80:3": null
		},
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {
			"LoadBalancer": {
				"spec": {
					"name": "k8s-ns1-ing1-159dd7a143",
					"scheme": "internet-facing",
					"subnetMapping": [
						{
							"subnetID": "subnet-c"
						},
						{
							"subnetID": "subnet-d"
						}
					]
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - referenced same service port with both name and port",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1-name",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-1-port",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Number: 80,
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::ElasticLoadBalancingV2::ListenerRule": {
			"80:1": {
				"spec": {
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-1.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-1-name"
								]
							}
						}
					]
				}
			},
			"80:2": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-1:80/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-1.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-1-port"
								]
							}
						}
					]
				}
			},
			"80:3": null
		},
		"AWS::ElasticLoadBalancingV2::TargetGroup": {
			"ns-1/ing-1-svc-1:80": {
				"spec": {
					"healthCheckConfig": {
						"healthyThresholdCount": 2,
						"intervalSeconds": 15,
						"matcher": {
							"httpCode": "200"
						},
						"path": "/",
						"port": "traffic-port",
						"protocol": "HTTP",
						"timeoutSeconds": 5,
						"unhealthyThresholdCount": 2
					},
					"ipAddressType": "ipv4",
					"name": "k8s-ns1-svc1-90b7d93b18",
					"port": 32768,
					"protocol": "HTTP",
					"protocolVersion": "HTTP1",
					"targetType": "instance"
				}
			},
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": null
		},
		"K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
			"ns-1/ing-1-svc-1:80": {
				"spec": {
					"template": {
						"metadata": {
							"creationTimestamp": null,
							"name": "k8s-ns1-svc1-90b7d93b18",
							"namespace": "ns-1"
						},
						"spec": {
							"ipAddressType": "ipv4",
							"vpcID": "vpc-dummy",
							"networking": {
								"ingress": [
									{
										"from": [
											{
												"securityGroup": {
													"groupID": "sg-auto"
												}
											}
										],
										"ports": [
											{
												"port": 32768,
												"protocol": "TCP"
											}
										]
									}
								]
							},
							"serviceRef": {
								"name": "svc-1",
								"port": 80
							},
							"targetGroupARN": {
								"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-1:80/status/targetGroupARN"
							},
							"targetType": "instance"
						}
					}
				}
			},
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": null
		}
	}
}`,
		},
		{
			name: "Ingress - inboundCIDRs in IngressClassParams",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										InboundCIDRs: []string{
											"10.0.0.0/8",
											"172.16.0.0/12",
										},
									},
								},
							},
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/inbound-cidrs": "20.0.0.0/8",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "10.0.0.0/8"
								}
							],
							"toPort": 80
						},
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "172.16.0.0/12"
								}
							],
							"toPort": 80
						}
					]
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - ssl-policy in IngressClassParams",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										SSLPolicy: "ingress-class-policy",
									},
								},
							},
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:acm:us-east-1:9999999:certificate/11111111",
									"alb.ingress.kubernetes.io/ssl-policy":      "annotated-ssl-policy",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 443,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "0.0.0.0/0"
								}
							],
							"toPort": 443
						}
					]
				}
			}
		},
		"AWS::ElasticLoadBalancingV2::Listener": {
			"443": {
				"spec": {
					"certificates": [
						{
							"certificateARN": "arn:aws:acm:us-east-1:9999999:certificate/11111111"
						}
					],
					"defaultActions": [
						{
							"fixedResponseConfig": {
								"contentType": "text/plain",
								"statusCode": "404"
							},
							"type": "fixed-response"
						}
					],
					"loadBalancerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
					},
					"port": 443,
					"protocol": "HTTPS",
					"sslPolicy": "ingress-class-policy"
				}
			},
			"80": null
		},
		"AWS::ElasticLoadBalancingV2::ListenerRule": {
			"443:1": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-1:http/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-1.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-1"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 1
				}
			},
			"443:2": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-2:http/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-1.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-2"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 2
				}
			},
			"443:3": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-3:https/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-2.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-3"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 3
				}
			},
			"80:1": null,
			"80:2": null,
			"80:3": null
		}
	}
}`,
		},
		{
			name: "Ingress - certificate-arn in IngressClassParams",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										CertificateArn: []string{"arn:aws:acm:us-east-1:9999999:certificate/ingress-class-certificate-arn"},
									},
								},
							},
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:acm:us-east-1:9999999:certificate/annotated-certificate-arn",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 443,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "0.0.0.0/0"
								}
							],
							"toPort": 443
						}
					]
				}
			}
		},
		"AWS::ElasticLoadBalancingV2::Listener": {
			"443": {
				"spec": {
					"certificates": [
						{
							"certificateARN": "arn:aws:acm:us-east-1:9999999:certificate/ingress-class-certificate-arn"
						}
					],
					"defaultActions": [
						{
							"fixedResponseConfig": {
								"contentType": "text/plain",
								"statusCode": "404"
							},
							"type": "fixed-response"
						}
					],
					"loadBalancerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
					},
					"port": 443,
					"protocol": "HTTPS",
					"sslPolicy": "ELBSecurityPolicy-2016-08"
				}
			},
			"80": null
		},
		"AWS::ElasticLoadBalancingV2::ListenerRule": {
			"443:1": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-1:http/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-1.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-1"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 1
				}
			},
			"443:2": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-2:http/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-1.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-2"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 2
				}
			},
			"443:3": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-3:https/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-2.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-3"
								]
							}
						}
					],
					"listenerARN": {
						"$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/443/status/listenerARN"
					},
					"priority": 3
				}
			},
			"80:1": null,
			"80:2": null,
			"80:3": null
		}
	}
}`,
		},
		{
			name: "Ingress - wafv2AclArn in IngressClassParams",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										WAFv2ACLArn: "alb.ingress.kubernetes.io/wafv2-acl-arn: arn:aws:wafv2:us-west-2:xxxxx:regional/webacl/xxxxxxx/3ab78708-85b0-49d3-b4e1-7a9615a6613b",
									},
								},
							},
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
    "id":"ns-1/ing-1",
    "resources":{
		"AWS::WAFv2::WebACLAssociation":{
			"LoadBalancer":{
				"spec":{
					"resourceARN":{
						"$ref":"#/resources/AWS::ElasticLoadBalancingV2::LoadBalancer/LoadBalancer/status/loadBalancerARN"
					},
					"webACLARN":"alb.ingress.kubernetes.io/wafv2-acl-arn: arn:aws:wafv2:us-west-2:xxxxx:regional/webacl/xxxxxxx/3ab78708-85b0-49d3-b4e1-7a9615a6613b"
				}
			}
		}
    }
}`,
		},
		{
			name: "Ingress - not using subnet auto-discovery and internal",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				listLoadBalancersCalls: []listLoadBalancersCall{
					{
						matchedLBs: []elbv2.LoadBalancerWithTags{
							{
								LoadBalancer: &elbv2types.LoadBalancer{
									LoadBalancerArn: awssdk.String("lb-1"),
									AvailabilityZones: []elbv2types.AvailabilityZone{
										{
											SubnetId: awssdk.String("subnet-e"),
										},
										{
											SubnetId: awssdk.String("subnet-f"),
										},
									},
									Scheme: elbv2types.LoadBalancerSchemeEnumInternal,
								},
								Tags: map[string]string{
									"elbv2.k8s.aws/cluster": "cluster-name",
									"ingress.k8s.aws/stack": "ns-1/ing-1",
								},
							},
							{
								LoadBalancer: &elbv2types.LoadBalancer{
									LoadBalancerArn: awssdk.String("lb-2"),
									AvailabilityZones: []elbv2types.AvailabilityZone{
										{
											SubnetId: awssdk.String("subnet-e"),
										},
										{
											SubnetId: awssdk.String("subnet-f"),
										},
									},
									Scheme: elbv2types.LoadBalancerSchemeEnumInternal,
								},
								Tags: map[string]string{
									"keyA": "valueA2",
									"keyB": "valueB2",
								},
							},
							{
								LoadBalancer: &elbv2types.LoadBalancer{
									LoadBalancerArn: awssdk.String("lb-3"),
									AvailabilityZones: []elbv2types.AvailabilityZone{
										{
											SubnetId: awssdk.String("subnet-e"),
										},
										{
											SubnetId: awssdk.String("subnet-f"),
										},
									},
									Scheme: elbv2types.LoadBalancerSchemeEnumInternal,
								},
								Tags: map[string]string{
									"keyA": "valueA3",
									"keyB": "valueB3",
								},
							},
						},
					},
				},
				enableBackendSG: true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {
			"LoadBalancer": {
				"spec": {
					"subnetMapping": [
						{
							"subnetID": "subnet-e"
						},
						{
							"subnetID": "subnet-f"
						}
					]
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - deletion protection enabled error",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					InactiveMembers: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "default",
								Name:      "hello-ingress",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":                        "alb",
									"alb.ingress.kubernetes.io/load-balancer-attributes": "deletion_protection.enabled=true",
								},
								Finalizers: []string{
									"ingress.k8s.aws/resources",
								},
								DeletionTimestamp: &metav1.Time{
									Time: time.Now(),
								},
							},
						},
					},
				},
			},
			wantErr: "deletion_protection is enabled, cannot delete the ingress: hello-ingress",
		},
		{
			name: "Ingress - with SG annotation",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				describeSecurityGroupsResult: []describeSecurityGroupsResult{
					{
						securityGroups: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-manual"),
							},
						},
					},
				},
				backendSecurityGroup: "sg-backend",
				enableBackendSG:      true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "ns-1",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/security-groups": "sg-manual",
										"alb.ingress.kubernetes.io/scheme":          "internet-facing",
										"alb.ingress.kubernetes.io/target-type":     "instance",
									},
								},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": null,
		"AWS::ElasticLoadBalancingV2::ListenerRule": {
			"80:1": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-3:https/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "host-header",
							"hostHeaderConfig": {
								"values": [
									"app-2.example.com"
								]
							}
						},
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/svc-3"
								]
							}
						}
					]
				}
			},
			"80:2": null,
			"80:3": null
		},
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {
			"LoadBalancer": {
				"spec": {
					"name": "k8s-ns1-ing1-159dd7a143",
					"scheme": "internet-facing",
					"securityGroups": [
						"sg-manual"
					]
				}
			}
		},
		"AWS::ElasticLoadBalancingV2::TargetGroup": {
			"ns-1/ing-1-svc-1:http": null,
			"ns-1/ing-1-svc-2:http": null
		},
		"K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
			"ns-1/ing-1-svc-1:http": null,
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": {
				"spec": {
					"template": {
						"spec": {
							"networking": null
						}
					}
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - with SG annotation, backend SG feature disabled, managed backend sg set to true",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				describeSecurityGroupsResult: []describeSecurityGroupsResult{
					{
						securityGroups: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-manual"),
							},
						},
					},
				},
				backendSecurityGroup: "sg-backend",
				enableBackendSG:      false,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "ns-1",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/security-groups":                     "sg-manual",
										"alb.ingress.kubernetes.io/scheme":                              "internet-facing",
										"alb.ingress.kubernetes.io/target-type":                         "instance",
										"alb.ingress.kubernetes.io/manage-backend-security-group-rules": "true",
									},
								},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: "backendSG feature is required to manage worker node SG rules when frontendSG manually specified",
		},
		{
			name: "Ingress with IPv6 service",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_ipv6},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/target-type":     "ip",
									"alb.ingress.kubernetes.io/ip-address-type": "dualstack",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_ipv6.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "0.0.0.0/0"
								}
							],
							"toPort": 80
						},
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"ipv6Ranges": [
								{
									"cidrIPv6": "::/0"
								}
							],
							"toPort": 80
						}
					]
				}
			}
		},
		"AWS::ElasticLoadBalancingV2::ListenerRule": {
			"80:1": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-ipv6:https/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/"
								]
							}
						}
					]
				}
			},
			"80:2": null,
			"80:3": null
		},
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {
			"LoadBalancer": {
				"spec": {
					"ipAddressType": "dualstack"
				}
			}
		},
		"AWS::ElasticLoadBalancingV2::TargetGroup": {
			"ns-1/ing-1-svc-1:http": null,
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": null,
			"ns-1/ing-1-svc-ipv6:https": {
				"spec": {
					"healthCheckConfig": {
						"healthyThresholdCount": 2,
						"intervalSeconds": 15,
						"matcher": {
							"httpCode": "200"
						},
						"path": "/",
						"port": "traffic-port",
						"protocol": "HTTP",
						"timeoutSeconds": 5,
						"unhealthyThresholdCount": 2
					},
					"ipAddressType": "ipv6",
					"name": "k8s-ns1-svcipv6-c387b9e773",
					"port": 8443,
					"protocol": "HTTP",
					"protocolVersion": "HTTP1",
					"targetType": "ip"
				}
			}
		},
		"K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
			"ns-1/ing-1-svc-1:http": null,
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": null,
			"ns-1/ing-1-svc-ipv6:https": {
				"spec": {
					"template": {
						"metadata": {
							"creationTimestamp": null,
							"name": "k8s-ns1-svcipv6-c387b9e773",
							"namespace": "ns-1"
						},
						"spec": {
							"ipAddressType": "ipv6",
							"vpcID": "vpc-dummy",
							"networking": {
								"ingress": [
									{
										"from": [
											{
												"securityGroup": {
													"groupID": "sg-auto"
												}
											}
										],
										"ports": [
											{
												"port": 8443,
												"protocol": "TCP"
											}
										]
									}
								]
							},
							"serviceRef": {
								"name": "svc-ipv6",
								"port": "https"
							},
							"targetGroupARN": {
								"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-ipv6:https/status/targetGroupARN"
							},
							"targetType": "ip"
						}
					}
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress with IPv6 service but not dualstack",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_ipv6},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/target-type": "ip",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_ipv6.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: "ingress: ns-1/ing-1: unsupported IPv6 configuration, lb not dual-stack",
		},
		{
			name: "target type IP with enableIPTargetType set to false",
			env: env{
				svcs: []*corev1.Service{svcWithNamedTargetPort},
			},
			enableIPTargetType: awssdk.Bool(false),
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/target-type": "ip",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: svcWithNamedTargetPort.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: "ingress: ns-1/ing-1: unsupported targetType: ip when EnableIPTargetType is false",
		},
		{
			name: "target type IP with named target port",
			env: env{
				svcs: []*corev1.Service{svcWithNamedTargetPort},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/target-type": "ip",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: svcWithNamedTargetPort.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::ElasticLoadBalancingV2::ListenerRule": {
			"80:1": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-named-targetport:https/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/"
								]
							}
						}
					]
				}
			},
			"80:2": null,
			"80:3": null
		},
		"AWS::ElasticLoadBalancingV2::TargetGroup": {
			"ns-1/ing-1-svc-1:http": null,
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": null,
			"ns-1/ing-1-svc-named-targetport:https": {
				"spec": {
					"healthCheckConfig": {
						"healthyThresholdCount": 2,
						"intervalSeconds": 15,
						"matcher": {
							"httpCode": "200"
						},
						"path": "/",
						"port": "traffic-port",
						"protocol": "HTTP",
						"timeoutSeconds": 5,
						"unhealthyThresholdCount": 2
					},
					"ipAddressType": "ipv4",
					"name": "k8s-ns1-svcnamed-3430e53ee8",
					"port": 1,
					"protocol": "HTTP",
					"protocolVersion": "HTTP1",
					"targetType": "ip"
				}
			}
		},
		"K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
			"ns-1/ing-1-svc-1:http": null,
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": null,
			"ns-1/ing-1-svc-named-targetport:https": {
				"spec": {
					"template": {
						"metadata": {
							"creationTimestamp": null,
							"name": "k8s-ns1-svcnamed-3430e53ee8",
							"namespace": "ns-1"
						},
						"spec": {
							"ipAddressType": "ipv4",
							"vpcID": "vpc-dummy",
							"networking": {
								"ingress": [
									{
										"from": [
											{
												"securityGroup": {
													"groupID": "sg-auto"
												}
											}
										],
										"ports": [
											{
												"port": "target-port",
												"protocol": "TCP"
											}
										]
									}
								]
							},
							"serviceRef": {
								"name": "svc-named-targetport",
								"port": "https"
							},
							"targetGroupARN": {
								"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-named-targetport:https/status/targetGroupARN"
							},
							"targetType": "ip"
						}
					}
				}
			}
		}
	}
}`,
		},
		{
			name: "default target type IP with named target port",
			env: env{
				svcs: []*corev1.Service{svcWithNamedTargetPort},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: svcWithNamedTargetPort.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			defaultTargetType: "ip",
			wantStackPatch: `
{
	"resources": {
		"AWS::ElasticLoadBalancingV2::ListenerRule": {
			"80:1": {
				"spec": {
					"actions": [
						{
							"forwardConfig": {
								"targetGroups": [
									{
										"targetGroupARN": {
											"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-named-targetport:https/status/targetGroupARN"
										}
									}
								]
							},
							"type": "forward"
						}
					],
					"conditions": [
						{
							"field": "path-pattern",
							"pathPatternConfig": {
								"values": [
									"/"
								]
							}
						}
					]
				}
			},
			"80:2": null,
			"80:3": null
		},
		"AWS::ElasticLoadBalancingV2::TargetGroup": {
			"ns-1/ing-1-svc-1:http": null,
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": null,
			"ns-1/ing-1-svc-named-targetport:https": {
				"spec": {
					"healthCheckConfig": {
						"healthyThresholdCount": 2,
						"intervalSeconds": 15,
						"matcher": {
							"httpCode": "200"
						},
						"path": "/",
						"port": "traffic-port",
						"protocol": "HTTP",
						"timeoutSeconds": 5,
						"unhealthyThresholdCount": 2
					},
					"ipAddressType": "ipv4",
					"name": "k8s-ns1-svcnamed-3430e53ee8",
					"port": 1,
					"protocol": "HTTP",
					"protocolVersion": "HTTP1",
					"targetType": "ip"
				}
			}
		},
		"K8S::ElasticLoadBalancingV2::TargetGroupBinding": {
			"ns-1/ing-1-svc-1:http": null,
			"ns-1/ing-1-svc-2:http": null,
			"ns-1/ing-1-svc-3:https": null,
			"ns-1/ing-1-svc-named-targetport:https": {
				"spec": {
					"template": {
						"metadata": {
							"creationTimestamp": null,
							"name": "k8s-ns1-svcnamed-3430e53ee8",
							"namespace": "ns-1"
						},
						"spec": {
							"ipAddressType": "ipv4",
							"vpcID": "vpc-dummy",
							"networking": {
								"ingress": [
									{
										"from": [
											{
												"securityGroup": {
													"groupID": "sg-auto"
												}
											}
										],
										"ports": [
											{
												"port": "target-port",
												"protocol": "TCP"
											}
										]
									}
								]
							},
							"serviceRef": {
								"name": "svc-named-targetport",
								"port": "https"
							},
							"targetGroupARN": {
								"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns-1/ing-1-svc-named-targetport:https/status/targetGroupARN"
							},
							"targetType": "ip"
						}
					}
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - default IPv4 CIDR when no IP range",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{},
								},
							},
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace:   "ns-1",
								Name:        "ing-1",
								Annotations: map[string]string{},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "0.0.0.0/0"
								}
							],
							"toPort": 80
						}
					]
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - default dualstack CIDR when no IP range",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{},
								},
							},
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ip-address-type": "dualstack",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "0.0.0.0/0"
								}
							],
							"toPort": 80
						},
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"ipv6Ranges": [
								{
									"cidrIPv6": "::/0"
								}
							],
							"toPort": 80
						}
					]
				}
			}
		},
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {
			"LoadBalancer": {
				"spec": {
					"ipAddressType": "dualstack"
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - ingress with managed prefix list",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{},
								},
							},
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/security-group-prefix-lists": "pl-00000000",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"prefixLists": [
								{
									"listID": "pl-00000000"
								}
							],
							"toPort": 80
						}
					]
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - ingress mixed ip address & managed prefix list",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternalLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{},
								},
							},
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/inbound-cidrs":               "20.45.16.0/26",
									"alb.ingress.kubernetes.io/security-group-prefix-lists": "pl-00000000",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::EC2::SecurityGroup": {
			"ManagedLBSecurityGroup": {
				"spec": {
					"ingress": [
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"ipRanges": [
								{
									"cidrIP": "20.45.16.0/26"
								}
							],
							"toPort": 80
						},
						{
							"fromPort": 80,
							"ipProtocol": "tcp",
							"prefixLists": [
								{
									"listID": "pl-00000000"
								}
							],
							"toPort": 80
						}
					]
				}
			}
		}
	}
}`,
		},
		{
			name: "Ingress - vanilla with default-load-balancer-scheme internet-facing",
			env: env{
				svcs: []*corev1.Service{ns_1_svc_1, ns_1_svc_2, ns_1_svc_3},
			},
			fields: fields{
				resolveViaDiscoveryCalls: []resolveViaDiscoveryCall{resolveViaDiscoveryCallForInternetFacingLB},
				listLoadBalancersCalls:   []listLoadBalancersCall{listLoadBalancerCallForEmptyLB},
				enableBackendSG:          true,
			},
			defaultLoadBalancerScheme: string(elbv2model.LoadBalancerSchemeInternetFacing),
			args: args{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_1.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_2.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-3",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: ns_1_svc_3.Name,
																	Port: networking.ServiceBackendPort{
																		Name: "https",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantStackPatch: `
{
	"resources": {
		"AWS::ElasticLoadBalancingV2::LoadBalancer": {
			"LoadBalancer": {
				"spec": {
					"name": "k8s-ns1-ing1-159dd7a143",
					"scheme": "internet-facing",
					"subnetMapping": [
						{
							"subnetID": "subnet-c"
						},
						{
							"subnetID": "subnet-d"
						}
					]
				}
			}
		}
	}
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			elbv2TaggingManager := elbv2.NewMockTaggingManager(ctrl)
			for _, call := range tt.fields.listLoadBalancersCalls {
				elbv2TaggingManager.EXPECT().ListLoadBalancers(gomock.Any(), gomock.Any()).Return(call.matchedLBs, call.err)
			}

			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			for _, svc := range tt.env.svcs {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}
			eventRecorder := record.NewFakeRecorder(10)
			vpcID := "vpc-dummy"
			clusterName := "cluster-dummy"
			ec2Client := services.NewMockEC2(ctrl)
			elbv2Client := services.NewMockELBV2(ctrl)
			for _, res := range tt.fields.describeSecurityGroupsResult {
				ec2Client.EXPECT().DescribeSecurityGroupsAsList(gomock.Any(), gomock.Any()).Return(res.securityGroups, res.err)
			}
			subnetsResolver := networkingpkg.NewMockSubnetsResolver(ctrl)
			for _, call := range tt.fields.resolveViaDiscoveryCalls {
				subnetsResolver.EXPECT().ResolveViaDiscovery(gomock.Any(), gomock.Any()).Return(call.subnets, call.err)
			}

			certDiscovery := NewMockCertDiscovery(ctrl)
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			authConfigBuilder := NewDefaultAuthConfigBuilder(annotationParser)
			enhancedBackendBuilder := NewDefaultEnhancedBackendBuilder(k8sClient, annotationParser, authConfigBuilder, true, true)
			ruleOptimizer := NewDefaultRuleOptimizer(logr.New(&log.NullLogSink{}))
			trackingProvider := tracking.NewDefaultProvider("ingress.k8s.aws", clusterName)
			stackMarshaller := deploy.NewDefaultStackMarshaller()
			backendSGProvider := networkingpkg.NewMockBackendSGProvider(ctrl)
			sgResolver := networkingpkg.NewDefaultSecurityGroupResolver(ec2Client, vpcID)
			if tt.fields.enableBackendSG {
				if len(tt.fields.backendSecurityGroup) > 0 {
					backendSGProvider.EXPECT().Get(gomock.Any(), networkingpkg.ResourceType(networkingpkg.ResourceTypeIngress), gomock.Any()).Return(tt.fields.backendSecurityGroup, nil).AnyTimes()
				} else {
					backendSGProvider.EXPECT().Get(gomock.Any(), networkingpkg.ResourceType(networkingpkg.ResourceTypeIngress), gomock.Any()).Return("sg-auto", nil).AnyTimes()
				}
				backendSGProvider.EXPECT().Release(gomock.Any(), networkingpkg.ResourceType(networkingpkg.ResourceTypeIngress), gomock.Any()).Return(nil).AnyTimes()
			}
			defaultTargetType := tt.defaultTargetType
			if defaultTargetType == "" {
				defaultTargetType = "instance"
			}
			defaultLoadBalancerScheme := tt.defaultLoadBalancerScheme
			if defaultLoadBalancerScheme == "" {
				defaultLoadBalancerScheme = string(elbv2model.LoadBalancerSchemeInternal)
			}

			b := &defaultModelBuilder{
				k8sClient:              k8sClient,
				eventRecorder:          eventRecorder,
				ec2Client:              ec2Client,
				elbv2Client:            elbv2Client,
				vpcID:                  vpcID,
				clusterName:            clusterName,
				annotationParser:       annotationParser,
				subnetsResolver:        subnetsResolver,
				sgResolver:             sgResolver,
				backendSGProvider:      backendSGProvider,
				certDiscovery:          certDiscovery,
				authConfigBuilder:      authConfigBuilder,
				enhancedBackendBuilder: enhancedBackendBuilder,
				ruleOptimizer:          ruleOptimizer,
				trackingProvider:       trackingProvider,
				elbv2TaggingManager:    elbv2TaggingManager,
				enableBackendSG:        tt.fields.enableBackendSG,
				featureGates:           config.NewFeatureGates(),
				logger:                 logr.New(&log.NullLogSink{}),

				defaultSSLPolicy:          "ELBSecurityPolicy-2016-08",
				defaultTargetType:         elbv2model.TargetType(defaultTargetType),
				defaultLoadBalancerScheme: elbv2model.LoadBalancerScheme(defaultLoadBalancerScheme),
			}

			if tt.enableIPTargetType == nil {
				b.enableIPTargetType = true
			} else {
				b.enableIPTargetType = *tt.enableIPTargetType
			}

			gotStack, _, _, _, err := b.Build(context.Background(), tt.args.ingGroup)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)

				wantStackJSON, err := jsonpatch.MergePatch([]byte(baseStackJSON), []byte(tt.wantStackPatch))
				require.NoError(t, err, "patching wantStack")
				var wantStack struct {
					ID        string                            `json:"id"`
					Resources map[string]map[string]interface{} `json:"resources"`
				}
				err = json.Unmarshal(wantStackJSON, &wantStack)
				require.NoError(t, err, "unmarshalling wantStack")

				// Set explicit null on all creationTimestamps in the binding, as JSON merge patches can't do that.
				for binding, value := range wantStack.Resources["K8S::ElasticLoadBalancingV2::TargetGroupBinding"] {
					if bindingMap, ok := value.(map[string]interface{}); ok {
						if specMap, ok := bindingMap["spec"].(map[string]interface{}); ok {
							if templateMap, ok := specMap["template"].(map[string]interface{}); ok {
								if metadataMap, ok := templateMap["metadata"].(map[string]interface{}); ok {
									metadataMap["creationTimestamp"] = nil
									templateMap["metadata"] = metadataMap
									specMap["template"] = templateMap
									bindingMap["spec"] = specMap
									wantStack.Resources["K8S::ElasticLoadBalancingV2::TargetGroupBinding"][binding] = bindingMap
								}
							}
						}
					}
				}

				wantStackYAML, _ := yaml.Marshal(wantStack)

				stackJSON, err := stackMarshaller.Marshal(gotStack)
				assert.NoError(t, err, "marshalling stack")
				var stack interface{}
				_ = json.Unmarshal([]byte(stackJSON), &stack)
				gotStackYAML, _ := yaml.Marshal(stack)

				eq := assert.Equal(t, string(wantStackYAML), string(gotStackYAML))
				if !eq {
					patch, _ := jsonpatch.CreateMergePatch([]byte(baseStackJSON), []byte(stackJSON))
					t.Log(string(patch))
				}
			}
		})
	}
}

func Test_defaultModelBuildTask_buildSSLRedirectConfig(t *testing.T) {
	type fields struct {
		ingGroup Group
	}
	type args struct {
		listenPortConfigByPort map[int32]listenPortConfig
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *SSLRedirectConfig
		wantErr error
	}{
		{
			name: "single Ingress without ssl-redirect annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-1",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				listenPortConfigByPort: map[int32]listenPortConfig{
					80: {
						protocol: elbv2model.ProtocolHTTP,
					},
					443: {
						protocol: elbv2model.ProtocolHTTPS,
					},
				},
			},
			want:    nil,
			wantErr: nil,
		},
		{
			name: "single Ingress with ssl-redirect annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ssl-redirect": "443",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-1",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				listenPortConfigByPort: map[int32]listenPortConfig{
					80: {
						protocol: elbv2model.ProtocolHTTP,
					},
					443: {
						protocol: elbv2model.ProtocolHTTPS,
					},
				},
			},
			want: &SSLRedirectConfig{
				SSLPort:    443,
				StatusCode: "HTTP_301",
			},
			wantErr: nil,
		},
		{
			name: "single Ingress with ssl-redirect annotation but refer non-exists port",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ssl-redirect": "8443",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-1",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				listenPortConfigByPort: map[int32]listenPortConfig{
					80: {
						protocol: elbv2model.ProtocolHTTP,
					},
					443: {
						protocol: elbv2model.ProtocolHTTPS,
					},
				},
			},
			want:    nil,
			wantErr: errors.New("listener does not exist for SSLRedirect port: 8443"),
		},
		{
			name: "single Ingress with ssl-redirect annotation but refer non-SSL port",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "ns-1", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ssl-redirect": "80",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-1",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				listenPortConfigByPort: map[int32]listenPortConfig{
					80: {
						protocol: elbv2model.ProtocolHTTP,
					},
					443: {
						protocol: elbv2model.ProtocolHTTPS,
					},
				},
			},
			want:    nil,
			wantErr: errors.New("listener protocol non-SSL for SSLRedirect port: 80"),
		},
		{
			name: "multiple Ingress without ssl-redirect annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "", Name: "awesome-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-1",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-2",
								Name:      "ing-2",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-2",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				listenPortConfigByPort: map[int32]listenPortConfig{
					80: {
						protocol: elbv2model.ProtocolHTTP,
					},
					443: {
						protocol: elbv2model.ProtocolHTTPS,
					},
				},
			},
			want:    nil,
			wantErr: nil,
		},
		{
			name: "multiple Ingress with one ingress have ssl-redirect annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "", Name: "awesome-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ssl-redirect": "443",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-1",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-2",
								Name:      "ing-2",
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-2",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				listenPortConfigByPort: map[int32]listenPortConfig{
					80: {
						protocol: elbv2model.ProtocolHTTP,
					},
					443: {
						protocol: elbv2model.ProtocolHTTPS,
					},
				},
			},
			want: &SSLRedirectConfig{
				SSLPort:    443,
				StatusCode: "HTTP_301",
			},
			wantErr: nil,
		},
		{
			name: "multiple Ingress with same ssl-redirect annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "", Name: "awesome-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ssl-redirect": "443",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-1",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-2",
								Name:      "ing-2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ssl-redirect": "443",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-2",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				listenPortConfigByPort: map[int32]listenPortConfig{
					80: {
						protocol: elbv2model.ProtocolHTTP,
					},
					443: {
						protocol: elbv2model.ProtocolHTTPS,
					},
				},
			},
			want: &SSLRedirectConfig{
				SSLPort:    443,
				StatusCode: "HTTP_301",
			},
			wantErr: nil,
		},
		{
			name: "multiple Ingress with conflicting ssl-redirect annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "", Name: "awesome-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-1",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ssl-redirect": "443",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-1.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-1",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-1",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{ObjectMeta: metav1.ObjectMeta{
								Namespace: "ns-2",
								Name:      "ing-2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ssl-redirect": "8443",
								},
							},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{
											Host: "app-2.example.com",
											IngressRuleValue: networking.IngressRuleValue{
												HTTP: &networking.HTTPIngressRuleValue{
													Paths: []networking.HTTPIngressPath{
														{
															Path: "/svc-2",
															Backend: networking.IngressBackend{
																Service: &networking.IngressServiceBackend{
																	Name: "svc-2",
																	Port: networking.ServiceBackendPort{
																		Name: "http",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				listenPortConfigByPort: map[int32]listenPortConfig{
					80: {
						protocol: elbv2model.ProtocolHTTP,
					},
					443: {
						protocol: elbv2model.ProtocolHTTPS,
					},
				},
			},
			want:    nil,
			wantErr: errors.New("conflicting sslRedirect port: [443 8443]"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				ingGroup:         tt.fields.ingGroup,
			}
			got, err := task.buildSSLRedirectConfig(context.Background(), tt.args.listenPortConfigByPort)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
