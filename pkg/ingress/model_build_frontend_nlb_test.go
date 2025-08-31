package ingress

import (
	"context"
	"fmt"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/go-logr/logr"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	networking2 "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	networkingpkg "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultModelBuildTask_buildFrontendNlbSecurityGroups(t *testing.T) {
	type describeSecurityGroupsResult struct {
		securityGroups []ec2types.SecurityGroup
		err            error
	}

	type fields struct {
		ingGroup                     Group
		scheme                       elbv2.LoadBalancerScheme
		describeSecurityGroupsResult []describeSecurityGroupsResult
	}

	tests := []struct {
		name         string
		fields       fields
		wantSGTokens []core.StringToken
		wantErr      string
	}{
		{
			name: "with no annotations",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "ing-1",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternal,
			},
		},
		{
			name: "with annotations",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/frontend-nlb-security-groups": "sg-manual",
									},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternal,
				describeSecurityGroupsResult: []describeSecurityGroupsResult{
					{
						securityGroups: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-manual"),
							},
						},
					},
				},
			},
			wantSGTokens: []core.StringToken{core.LiteralStringToken("sg-manual")},
		},
		{
			name: "with two sgs",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/frontend-nlb-security-groups": "sg-manual1, sg-manual2",
									},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternal,
				describeSecurityGroupsResult: []describeSecurityGroupsResult{
					{
						securityGroups: []ec2types.SecurityGroup{
							{
								GroupId: awssdk.String("sg-manual1"),
							},
							{
								GroupId: awssdk.String("sg-manual2"),
							},
						},
					},
				},
			},
			wantSGTokens: []core.StringToken{core.LiteralStringToken("sg-manual1"), core.LiteralStringToken("sg-manual2")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockEC2 := services.NewMockEC2(ctrl)
			vpcID := "vpc-dummy"
			for _, res := range tt.fields.describeSecurityGroupsResult {
				mockEC2.EXPECT().DescribeSecurityGroupsAsList(gomock.Any(), gomock.Any()).Return(res.securityGroups, res.err)
			}
			sgResolver := networkingpkg.NewDefaultSecurityGroupResolver(mockEC2, vpcID)

			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				sgResolver:       sgResolver,
				ingGroup:         tt.fields.ingGroup,
				annotationParser: annotationParser,
			}

			got, err := task.buildFrontendNlbSecurityGroups(context.Background())

			if err != nil {
				assert.EqualError(t, err, tt.wantErr)
			} else {

				var gotSGTokens []core.StringToken
				for _, sgToken := range got {
					gotSGTokens = append(gotSGTokens, sgToken)
				}
				assert.Equal(t, tt.wantSGTokens, gotSGTokens)
			}
		})
	}
}

func Test_buildFrontendNlbSubnetMappings(t *testing.T) {

	type fields struct {
		ingGroup Group
		scheme   elbv2.LoadBalancerScheme
	}

	tests := []struct {
		name    string
		fields  fields
		want    []string
		wantErr string
	}{
		{
			name: "no annotation implicit subnets",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "ing-1",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternal,
			},
		},
		{
			name: "with subnets annoattion",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-2",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/frontend-nlb-subnets": "subnet-1,subnet-2",
									},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternal,
			},
			want: []string{"subnet-1", "subnet-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockEC2 := services.NewMockEC2(ctrl)
			mockEC2.EXPECT().DescribeSubnetsAsList(gomock.Any(), gomock.Any()).
				DoAndReturn(stubDescribeSubnetsAsList).
				AnyTimes()

			azInfoProvider := networking2.NewMockAZInfoProvider(ctrl)
			azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, availabilityZoneIDs []string) (map[string]ec2types.AvailabilityZone, error) {
					ret := make(map[string]ec2types.AvailabilityZone, len(availabilityZoneIDs))
					for _, id := range availabilityZoneIDs {
						ret[id] = ec2types.AvailabilityZone{ZoneType: awssdk.String("availability-zone")}
					}
					return ret, nil
				}).AnyTimes()

			subnetsResolver := networking2.NewDefaultSubnetsResolver(
				azInfoProvider,
				mockEC2,
				"vpc-1",
				"test-cluster",
				true,
				true,
				true,
				logr.New(&log.NullLogSink{}),
			)

			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				annotationParser: annotationParser,
				subnetsResolver:  subnetsResolver,
			}
			got, err := task.buildFrontendNlbSubnetMappings(context.Background(), tt.fields.scheme)

			if err != nil {
				assert.EqualError(t, err, tt.wantErr)
			} else {

				var gotSubnets []string
				for _, mapping := range got {
					gotSubnets = append(gotSubnets, mapping.SubnetID)
				}
				assert.Equal(t, tt.want, gotSubnets)
			}
		})
	}
}

func Test_buildFrontendNlbName(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		namespace   string
		ingName     string
		scheme      elbv2model.LoadBalancerScheme
		wantPrefix  string
		alb         *elbv2model.LoadBalancer
	}{
		{
			name:        "standard case",
			clusterName: "test-cluster",
			namespace:   "awesome-ns",
			ingName:     "my-ingress",
			scheme:      elbv2model.LoadBalancerSchemeInternal,
			wantPrefix:  "test-alb",
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name: "test-alb",
				},
			},
		},
		{
			name:        "with special characters",
			clusterName: "test-cluster",
			namespace:   "awesome-ns",
			ingName:     "my-ingress-1",
			scheme:      elbv2model.LoadBalancerSchemeInternal,
			wantPrefix:  "k8s-awesomen-myingr",
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				clusterName: tt.clusterName,
				ingGroup: Group{
					ID: GroupID{
						Namespace: tt.namespace,
						Name:      tt.ingName,
					},
				},
			}

			got, err := task.buildFrontendNlbName(context.Background(), tt.scheme, tt.alb)
			assert.NoError(t, err)
			assert.Contains(t, got, tt.wantPrefix)

		})
	}
}

func Test_buildFrontendNLBTargetGroupName(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		namespace   string
		ingName     string
		port        int32
		svcPort     intstr.IntOrString
		targetType  elbv2model.TargetType
		protocol    elbv2model.Protocol
		wantPrefix  string
	}{
		{
			name:        "standard case",
			clusterName: "test-cluster",
			namespace:   "default",
			ingName:     "my-ingress",
			port:        80,
			svcPort:     intstr.FromInt(80),
			targetType:  "alb",
			protocol:    elbv2model.ProtocolTCP,
			wantPrefix:  "k8s-default-mying",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				clusterName: tt.clusterName,
				loadBalancer: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Scheme: elbv2.LoadBalancerSchemeInternetFacing,
					},
				},
				ingGroup: Group{
					ID: GroupID{
						Namespace: tt.namespace,
						Name:      tt.ingName,
					},
				},
			}

			port80 := intstr.FromInt(80)

			healthCheckConfig := &elbv2model.TargetGroupHealthCheckConfig{
				Protocol: elbv2model.ProtocolTCP,
				Port:     &port80,
			}

			got := task.buildFrontendNlbTargetGroupName(
				context.Background(),
				tt.port,
				tt.targetType,
				tt.protocol,
				healthCheckConfig,
			)

			assert.Contains(t, got, tt.wantPrefix)

		})
	}
}

func Test_buildFrontendNlbSchemeViaAnnotation(t *testing.T) {
	tests := []struct {
		name          string
		annotations   map[string]string
		defaultScheme elbv2model.LoadBalancerScheme
		wantScheme    elbv2model.LoadBalancerScheme
		wantExplicit  bool
		wantErr       bool
	}{
		{
			name: "explicit internal scheme",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/frontend-nlb-scheme": "internal",
			},
			defaultScheme: elbv2model.LoadBalancerSchemeInternetFacing,
			wantScheme:    elbv2model.LoadBalancerSchemeInternal,
			wantExplicit:  true,
			wantErr:       false,
		},
		{
			name: "explicit internet-facing scheme",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/frontend-nlb-scheme": "internet-facing",
			},
			defaultScheme: elbv2model.LoadBalancerSchemeInternal,
			wantScheme:    elbv2model.LoadBalancerSchemeInternetFacing,
			wantExplicit:  true,
			wantErr:       false,
		},
		{
			name:          "no annotation - use default",
			annotations:   map[string]string{},
			defaultScheme: elbv2model.LoadBalancerSchemeInternal,
			wantScheme:    elbv2model.LoadBalancerSchemeInternal,
			wantExplicit:  false,
			wantErr:       false,
		},
		{
			name: "invalid scheme",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/frontend-nlb-scheme": "invalid",
			},
			defaultScheme: elbv2model.LoadBalancerSchemeInternal,
			wantScheme:    "",
			wantExplicit:  false,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}

			task := &defaultModelBuildTask{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: ing,
						},
					},
				},
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}

			alb := &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Scheme: tt.defaultScheme,
				},
			}

			gotScheme, gotExplicit, err := task.buildFrontendNlbSchemeViaAnnotation(context.Background(), alb)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantScheme, gotScheme)
				assert.Equal(t, tt.wantExplicit, gotExplicit)
			}
		})
	}
}

func Test_buildEnableFrontendNlbViaAnnotation(t *testing.T) {

	type fields struct {
		ingGroup Group
	}

	tests := []struct {
		name           string
		fields         fields
		wantEnabled    bool
		wantErr        bool
		expectedErrMsg string
	}{
		{
			name: "single ingress with enable annotation true",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/enable-frontend-nlb": "true",
									},
								},
							},
						},
					},
				},
			},
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "single ingress without annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "ing-1",
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
			},
			wantEnabled: false,
			wantErr:     false,
		},
		{
			name: "multiple ingresses with same enable value",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/enable-frontend-nlb": "true",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-2",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/enable-frontend-nlb": "true",
									},
								},
							},
						},
					},
				},
			},
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "multiple ingresses with conflicting enable values",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{
						Namespace: "awesome-ns",
						Name:      "my-ingress",
					},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/enable-frontend-nlb": "true",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-2",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/enable-frontend-nlb": "false",
									},
								},
							},
						},
					},
				},
			},
			wantEnabled:    false,
			wantErr:        true,
			expectedErrMsg: "conflicting enable frontend NLB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}

			got, err := task.buildEnableFrontendNlbViaAnnotation(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, got)
		})
	}
}

func Test_mergeFrontendNlbListenPortConfigs(t *testing.T) {
	tests := []struct {
		name           string
		configs        []FrontendNlbListenConfigWithIngress
		expectedConfig FrontendNlbListenerConfig
		wantErr        bool
		expectedErrMsg string
	}{
		{
			name: "valid config with no conflicts",
			configs: []FrontendNlbListenConfigWithIngress{
				{
					ingKey: types.NamespacedName{Namespace: "awesome-ns", Name: "ingress-1"},
					FrontendNlbListenerConfig: FrontendNlbListenerConfig{
						Port:       80,
						Protocol:   elbv2model.ProtocolTCP,
						TargetPort: 80,
						HealthCheckConfig: elbv2model.TargetGroupHealthCheckConfig{
							IntervalSeconds: awssdk.Int32(10),
							TimeoutSeconds:  awssdk.Int32(5),
						},
					},
				},
			},
			expectedConfig: FrontendNlbListenerConfig{
				Port:       80,
				Protocol:   elbv2model.ProtocolTCP,
				TargetPort: 80,
				HealthCheckConfig: elbv2model.TargetGroupHealthCheckConfig{
					IntervalSeconds: awssdk.Int32(10),
					TimeoutSeconds:  awssdk.Int32(5),
				},
			},
			wantErr: false,
		},
		{
			name: "conflicting health check interval",
			configs: []FrontendNlbListenConfigWithIngress{
				{
					ingKey: types.NamespacedName{Namespace: "awesome-ns", Name: "ingress-1"},
					FrontendNlbListenerConfig: FrontendNlbListenerConfig{
						Port:     80,
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: elbv2model.TargetGroupHealthCheckConfig{
							IntervalSeconds: awssdk.Int32(10),
						},
						HealthCheckConfigExplicit: map[string]bool{
							"IntervalSeconds": true,
						},
					},
				},
				{
					ingKey: types.NamespacedName{Namespace: "awesome-ns", Name: "ingress-2"},
					FrontendNlbListenerConfig: FrontendNlbListenerConfig{
						Port:     80,
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: elbv2model.TargetGroupHealthCheckConfig{
							IntervalSeconds: awssdk.Int32(15),
						},
						HealthCheckConfigExplicit: map[string]bool{
							"IntervalSeconds": true,
						},
					},
				},
			},
			wantErr:        true,
			expectedErrMsg: "conflicting IntervalSeconds",
		},
		{
			name: "valid multiple configs with same values",
			configs: []FrontendNlbListenConfigWithIngress{
				{
					ingKey: types.NamespacedName{Namespace: "awesome-ns", Name: "ingress-1"},
					FrontendNlbListenerConfig: FrontendNlbListenerConfig{
						Port:     80,
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: elbv2model.TargetGroupHealthCheckConfig{
							IntervalSeconds: awssdk.Int32(10),
							TimeoutSeconds:  awssdk.Int32(5),
						},
						TargetPort: 80,
						HealthCheckConfigExplicit: map[string]bool{
							"IntervalSeconds": true,
							"TimeoutSeconds":  true,
						},
					},
				},
				{
					ingKey: types.NamespacedName{Namespace: "awesome-ns", Name: "ingress-2"},
					FrontendNlbListenerConfig: FrontendNlbListenerConfig{
						Port:     80,
						Protocol: elbv2model.ProtocolTCP,
						HealthCheckConfig: elbv2model.TargetGroupHealthCheckConfig{
							IntervalSeconds: awssdk.Int32(10),
							TimeoutSeconds:  awssdk.Int32(5),
						},
						TargetPort: 80,
						HealthCheckConfigExplicit: map[string]bool{
							"IntervalSeconds": true,
							"TimeoutSeconds":  true,
						},
					},
				},
			},
			expectedConfig: FrontendNlbListenerConfig{
				Port:     80,
				Protocol: elbv2model.ProtocolTCP,
				HealthCheckConfig: elbv2model.TargetGroupHealthCheckConfig{
					IntervalSeconds: awssdk.Int32(10),
					TimeoutSeconds:  awssdk.Int32(5),
				},
				TargetPort: 80,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			task := &defaultModelBuildTask{}
			got, err := task.mergeFrontendNlbListenPortConfigs(context.Background(), tt.configs)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedConfig, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildFrontendNlbTagsViaAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		ingGroup    Group
		wantTags    map[string]string
		wantErr     bool
		expectedErr string
	}{
		{
			name: "valid tag format parsing - single ingress",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend,Cost-Center=engineering",
								},
							},
						},
					},
				},
			},
			wantTags: map[string]string{
				"Environment": "production",
				"Team":        "backend",
				"Cost-Center": "engineering",
			},
			wantErr: false,
		},
		{
			name: "valid tag format parsing - multiple ingresses with same tags",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
								},
							},
						},
					},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Cost-Center=engineering",
								},
							},
						},
					},
				},
			},
			wantTags: map[string]string{
				"Environment": "production",
				"Team":        "backend",
				"Cost-Center": "engineering",
			},
			wantErr: false,
		},
		{
			name: "conflicting tags across multiple ingresses",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
								},
							},
						},
					},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=staging,Team=backend",
								},
							},
						},
					},
				},
			},
			wantTags:    nil,
			wantErr:     true,
			expectedErr: "conflicting frontend NLB tag Environment: production | staging",
		},
		{
			name: "empty annotation",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "",
								},
							},
						},
					},
				},
			},
			wantTags: map[string]string{},
			wantErr:  false,
		},
		{
			name: "missing annotation",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
			},
			wantTags: nil,
			wantErr:  false,
		},
		{
			name: "special characters in keys and values",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "app.kubernetes.io/name=my-app,app.kubernetes.io/version=1.0.0,special-chars=value_with-special.chars",
								},
							},
						},
					},
				},
			},
			wantTags: map[string]string{
				"app.kubernetes.io/name":    "my-app",
				"app.kubernetes.io/version": "1.0.0",
				"special-chars":             "value_with-special.chars",
			},
			wantErr: false,
		},
		{
			name: "invalid format - missing equals",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment,Team=backend",
								},
							},
						},
					},
				},
			},
			wantTags:    nil,
			wantErr:     true,
			expectedErr: "failed to parse frontend NLB tags annotation",
		},
		{
			name: "mixed scenarios - some ingresses with tags, some without",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production",
								},
							},
						},
					},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-2",
								Annotations: map[string]string{},
							},
						},
					},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-3",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Team=backend",
								},
							},
						},
					},
				},
			},
			wantTags: map[string]string{
				"Environment": "production",
				"Team":        "backend",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:         tt.ingGroup,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}

			got, err := task.buildFrontendNlbTagsViaAnnotation(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, err.Error(), tt.expectedErr)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantTags, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_validateFrontendNlbTags(t *testing.T) {
	tests := []struct {
		name        string
		tags        map[string]string
		wantErr     bool
		expectedErr string
	}{
		{
			name: "valid tags",
			tags: map[string]string{
				"Environment": "production",
				"Team":        "backend",
				"Cost-Center": "engineering",
			},
			wantErr: false,
		},
		{
			name: "tag key exceeds character limit",
			tags: map[string]string{
				strings.Repeat("a", 129): "value", // 129 characters, exceeds 128 limit
			},
			wantErr:     true,
			expectedErr: "exceeds maximum length of 128 characters",
		},
		{
			name: "tag value exceeds character limit",
			tags: map[string]string{
				"key": strings.Repeat("a", 257), // 257 characters, exceeds 256 limit
			},
			wantErr:     true,
			expectedErr: "exceeds maximum length of 256 characters",
		},
		{
			name: "too many tags",
			tags: func() map[string]string {
				tags := make(map[string]string)
				for i := 0; i < 51; i++ { // 51 tags, exceeds 50 limit
					tags[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d", i)
				}
				return tags
			}(),
			wantErr:     true,
			expectedErr: "too many tags: 51 (maximum 50 allowed)",
		},
		{
			name: "AWS reserved tag key - lowercase",
			tags: map[string]string{
				"aws:cloudformation:stack-name": "my-stack",
			},
			wantErr:     true,
			expectedErr: "is reserved by AWS and cannot be used",
		},
		{
			name: "AWS reserved tag key - uppercase",
			tags: map[string]string{
				"AWS:CloudFormation:StackName": "my-stack",
			},
			wantErr:     true,
			expectedErr: "is reserved by AWS and cannot be used",
		},
		{
			name: "empty tag key",
			tags: map[string]string{
				"": "value",
			},
			wantErr:     true,
			expectedErr: "tag key cannot be empty",
		},
		{
			name: "valid edge case - exactly at limits",
			tags: map[string]string{
				strings.Repeat("a", 128): strings.Repeat("b", 256), // exactly at limits
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}

			err := task.validateFrontendNlbTags(tt.tags)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, err.Error(), tt.expectedErr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildFrontendNlbTags(t *testing.T) {
	tests := []struct {
		name                string
		ingGroup            Group
		externalManagedTags []string
		wantTags            map[string]string
		wantErr             bool
		expectedErr         string
	}{
		{
			name: "valid tags with validation passing",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
								},
							},
						},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags: map[string]string{
				"Environment": "production",
				"Team":        "backend",
			},
			wantErr: false,
		},
		{
			name: "no tags annotation returns empty map",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags:            map[string]string{},
			wantErr:             false,
		},
		{
			name: "validation fails for AWS reserved key",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:cloudformation:stack-name=my-stack",
								},
							},
						},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags:            nil,
			wantErr:             true,
			expectedErr:         "is reserved by AWS and cannot be used",
		},
		{
			name: "parsing fails for invalid format",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "invalid-format-no-equals",
								},
							},
						},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags:            nil,
			wantErr:             true,
			expectedErr:         "failed to parse frontend NLB tags annotation",
		},
		{
			name: "external managed tag conflict validation",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,ManagedTag=conflict",
								},
							},
						},
					},
				},
			},
			externalManagedTags: []string{"ManagedTag", "AnotherManagedTag"},
			wantTags:            nil,
			wantErr:             true,
			expectedErr:         "external managed tag key ManagedTag cannot be specified",
		},
		{
			name: "validation passes with non-conflicting external managed tags",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
								},
							},
						},
					},
				},
			},
			externalManagedTags: []string{"ManagedTag", "AnotherManagedTag"},
			wantTags: map[string]string{
				"Environment": "production",
				"Team":        "backend",
			},
			wantErr: false,
		},
		{
			name: "validation fails for tag count limit",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": func() string {
										var tags []string
										for i := 0; i < 51; i++ { // 51 tags, exceeds 50 limit
											tags = append(tags, fmt.Sprintf("key%d=value%d", i, i))
										}
										return strings.Join(tags, ",")
									}(),
								},
							},
						},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags:            nil,
			wantErr:             true,
			expectedErr:         "too many tags: 51 (maximum 50 allowed)",
		},
		{
			name: "validation fails for key length limit",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": strings.Repeat("a", 129) + "=value", // 129 characters, exceeds 128 limit
								},
							},
						},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags:            nil,
			wantErr:             true,
			expectedErr:         "exceeds maximum length of 128 characters",
		},
		{
			name: "validation fails for value length limit",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "key=" + strings.Repeat("a", 257), // 257 characters, exceeds 256 limit
								},
							},
						},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags:            nil,
			wantErr:             true,
			expectedErr:         "exceeds maximum length of 256 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create external managed tags set
			externalManagedTags := sets.NewString(tt.externalManagedTags...)

			task := &defaultModelBuildTask{
				ingGroup:            tt.ingGroup,
				annotationParser:    annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				externalManagedTags: externalManagedTags,
			}

			got, err := task.buildFrontendNlbTags(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, err.Error(), tt.expectedErr)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantTags, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildFrontendNlbSpec(t *testing.T) {
	tests := []struct {
		name                string
		ingGroup            Group
		scheme              elbv2model.LoadBalancerScheme
		alb                 *elbv2model.LoadBalancer
		externalManagedTags []string
		wantTags            map[string]string
		wantErr             bool
		expectedErr         string
	}{
		{
			name: "tags integration with NLB spec creation",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production,Team=backend",
								},
							},
						},
					},
				},
			},
			scheme: elbv2model.LoadBalancerSchemeInternal,
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name:          "test-alb",
					IPAddressType: elbv2model.IPAddressTypeIPV4,
					SecurityGroups: []core.StringToken{
						core.LiteralStringToken("sg-12345"),
					},
					SubnetMappings: []elbv2model.SubnetMapping{
						{SubnetID: "subnet-12345"},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags: map[string]string{
				"Environment": "production",
				"Team":        "backend",
			},
			wantErr: false,
		},
		{
			name: "no tags annotation - empty tags in spec",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
			},
			scheme: elbv2model.LoadBalancerSchemeInternal,
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name:          "test-alb",
					IPAddressType: elbv2model.IPAddressTypeIPV4,
					SecurityGroups: []core.StringToken{
						core.LiteralStringToken("sg-12345"),
					},
					SubnetMappings: []elbv2model.SubnetMapping{
						{SubnetID: "subnet-12345"},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags:            map[string]string{},
			wantErr:             false,
		},
		{
			name: "tag validation failure prevents spec creation",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "aws:cloudformation:stack-name=my-stack",
								},
							},
						},
					},
				},
			},
			scheme: elbv2model.LoadBalancerSchemeInternal,
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name:          "test-alb",
					IPAddressType: elbv2model.IPAddressTypeIPV4,
				},
			},
			externalManagedTags: []string{},
			wantTags:            nil,
			wantErr:             true,
			expectedErr:         "is reserved by AWS and cannot be used",
		},
		{
			name: "interaction with existing NLB configuration",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production",
								},
							},
						},
					},
				},
			},
			scheme: elbv2model.LoadBalancerSchemeInternetFacing,
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name:          "custom-alb-name",
					IPAddressType: elbv2model.IPAddressTypeDualStack,
					SecurityGroups: []core.StringToken{
						core.LiteralStringToken("sg-custom"),
					},
					SubnetMappings: []elbv2model.SubnetMapping{
						{SubnetID: "subnet-custom"},
					},
				},
			},
			externalManagedTags: []string{},
			wantTags: map[string]string{
				"Environment": "production",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Mock EC2 service for security groups and subnets
			mockEC2 := services.NewMockEC2(ctrl)
			mockEC2.EXPECT().DescribeSecurityGroupsAsList(gomock.Any(), gomock.Any()).
				Return([]ec2types.SecurityGroup{
					{GroupId: awssdk.String("sg-12345")},
					{GroupId: awssdk.String("sg-custom")},
				}, nil).AnyTimes()

			mockEC2.EXPECT().DescribeSubnetsAsList(gomock.Any(), gomock.Any()).
				DoAndReturn(stubDescribeSubnetsAsList).AnyTimes()

			azInfoProvider := networking2.NewMockAZInfoProvider(ctrl)
			azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, availabilityZoneIDs []string) (map[string]ec2types.AvailabilityZone, error) {
					ret := make(map[string]ec2types.AvailabilityZone, len(availabilityZoneIDs))
					for _, id := range availabilityZoneIDs {
						ret[id] = ec2types.AvailabilityZone{ZoneType: awssdk.String("availability-zone")}
					}
					return ret, nil
				}).AnyTimes()

			sgResolver := networkingpkg.NewDefaultSecurityGroupResolver(mockEC2, "vpc-1")
			subnetsResolver := networking2.NewDefaultSubnetsResolver(
				azInfoProvider,
				mockEC2,
				"vpc-1",
				"test-cluster",
				true,
				true,
				true,
				logr.New(&log.NullLogSink{}),
			)

			// Create external managed tags set
			externalManagedTags := sets.NewString(tt.externalManagedTags...)

			task := &defaultModelBuildTask{
				ingGroup:            tt.ingGroup,
				annotationParser:    annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				externalManagedTags: externalManagedTags,
				sgResolver:          sgResolver,
				subnetsResolver:     subnetsResolver,
				clusterName:         "test-cluster",
			}

			spec, err := task.buildFrontendNlbSpec(context.Background(), tt.scheme, tt.alb)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, err.Error(), tt.expectedErr)
				}
			} else {
				assert.NoError(t, err)

				// Verify tags are properly integrated into the spec
				assert.Equal(t, tt.wantTags, spec.Tags)

				// Verify other spec properties are correctly set
				assert.Equal(t, elbv2model.LoadBalancerTypeNetwork, spec.Type)
				assert.Equal(t, tt.scheme, spec.Scheme)
				assert.Equal(t, tt.alb.Spec.IPAddressType, spec.IPAddressType)

				// Verify name is generated
				assert.NotEmpty(t, spec.Name)

				// Verify security groups and subnets are inherited from ALB when not explicitly set
				if len(tt.alb.Spec.SecurityGroups) > 0 {
					assert.Equal(t, tt.alb.Spec.SecurityGroups, spec.SecurityGroups)
				}
				if len(tt.alb.Spec.SubnetMappings) > 0 {
					assert.Equal(t, tt.alb.Spec.SubnetMappings, spec.SubnetMappings)
				}
			}
		})
	}
}

func Test_defaultModelBuildTask_buildFrontendNlbModel(t *testing.T) {
	tests := []struct {
		name                        string
		ingGroup                    Group
		alb                         *elbv2model.LoadBalancer
		listenerPortConfigByIngress map[types.NamespacedName]map[int32]listenPortConfig
		wantErr                     bool
		expectedErr                 string
		expectNlbCreated            bool
	}{
		{
			name: "frontend NLB enabled with tags - model created",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/enable-frontend-nlb": "true",
									"alb.ingress.kubernetes.io/frontend-nlb-tags":   "Environment=production,Team=backend",
								},
							},
						},
					},
				},
			},
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name:          "test-alb",
					Scheme:        elbv2model.LoadBalancerSchemeInternal,
					IPAddressType: elbv2model.IPAddressTypeIPV4,
					SecurityGroups: []core.StringToken{
						core.LiteralStringToken("sg-12345"),
					},
					SubnetMappings: []elbv2model.SubnetMapping{
						{SubnetID: "subnet-12345"},
					},
				},
			},
			listenerPortConfigByIngress: map[types.NamespacedName]map[int32]listenPortConfig{},
			wantErr:                     false,
			expectNlbCreated:            true,
		},
		{
			name: "frontend NLB disabled - no model created",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/enable-frontend-nlb": "false",
									"alb.ingress.kubernetes.io/frontend-nlb-tags":   "Environment=production",
								},
							},
						},
					},
				},
			},
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name:   "test-alb",
					Scheme: elbv2model.LoadBalancerSchemeInternal,
				},
			},
			listenerPortConfigByIngress: map[types.NamespacedName]map[int32]listenPortConfig{},
			wantErr:                     false,
			expectNlbCreated:            false,
		},
		{
			name: "no frontend NLB annotation - no model created",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-tags": "Environment=production",
								},
							},
						},
					},
				},
			},
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name:   "test-alb",
					Scheme: elbv2model.LoadBalancerSchemeInternal,
				},
			},
			listenerPortConfigByIngress: map[types.NamespacedName]map[int32]listenPortConfig{},
			wantErr:                     false,
			expectNlbCreated:            false,
		},
		{
			name: "frontend NLB enabled but invalid tags - error",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/enable-frontend-nlb": "true",
									"alb.ingress.kubernetes.io/frontend-nlb-tags":   "aws:reserved=value",
								},
							},
						},
					},
				},
			},
			alb: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					Name:   "test-alb",
					Scheme: elbv2model.LoadBalancerSchemeInternal,
				},
			},
			listenerPortConfigByIngress: map[types.NamespacedName]map[int32]listenPortConfig{},
			wantErr:                     true,
			expectedErr:                 "is reserved by AWS and cannot be used",
			expectNlbCreated:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Mock EC2 service
			mockEC2 := services.NewMockEC2(ctrl)
			mockEC2.EXPECT().DescribeSecurityGroupsAsList(gomock.Any(), gomock.Any()).
				Return([]ec2types.SecurityGroup{
					{GroupId: awssdk.String("sg-12345")},
				}, nil).AnyTimes()

			mockEC2.EXPECT().DescribeSubnetsAsList(gomock.Any(), gomock.Any()).
				DoAndReturn(stubDescribeSubnetsAsList).AnyTimes()

			azInfoProvider := networking2.NewMockAZInfoProvider(ctrl)
			azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, availabilityZoneIDs []string) (map[string]ec2types.AvailabilityZone, error) {
					ret := make(map[string]ec2types.AvailabilityZone, len(availabilityZoneIDs))
					for _, id := range availabilityZoneIDs {
						ret[id] = ec2types.AvailabilityZone{ZoneType: awssdk.String("availability-zone")}
					}
					return ret, nil
				}).AnyTimes()

			sgResolver := networkingpkg.NewDefaultSecurityGroupResolver(mockEC2, "vpc-1")
			subnetsResolver := networking2.NewDefaultSubnetsResolver(
				azInfoProvider,
				mockEC2,
				"vpc-1",
				"test-cluster",
				true,
				true,
				true,
				logr.New(&log.NullLogSink{}),
			)

			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			desiredState := &core.FrontendNlbTargetGroupDesiredState{
				TargetGroups: make(map[string]*core.FrontendNlbTargetGroupState),
			}

			task := &defaultModelBuildTask{
				ingGroup:                           tt.ingGroup,
				annotationParser:                   annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				externalManagedTags:                sets.NewString(),
				sgResolver:                         sgResolver,
				subnetsResolver:                    subnetsResolver,
				clusterName:                        "test-cluster",
				stack:                              stack,
				frontendNlbTargetGroupDesiredState: desiredState,
			}

			err := task.buildFrontendNlbModel(context.Background(), tt.alb, tt.listenerPortConfigByIngress)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, err.Error(), tt.expectedErr)
				}
			} else {
				assert.NoError(t, err)

				if tt.expectNlbCreated {
					// Verify that the frontend NLB was created in the task
					assert.NotNil(t, task.frontendNlb, "Frontend NLB should be created when enabled")
				} else {
					// Verify that no frontend NLB was created
					assert.Nil(t, task.frontendNlb, "Frontend NLB should not be created when disabled")
				}
			}
		})
	}
}

func Test_defaultModelBuildTask_buildFrontendNlbListeners(t *testing.T) {
	tests := []struct {
		name                        string
		ingGroup                    Group
		albListenerPorts            []int32
		listenerPortConfigByIngress map[types.NamespacedName]map[int32]listenPortConfig
		loadBalancer                *elbv2model.LoadBalancer
		wantErr                     bool
		expectedErrMsg              string
	}{
		{
			name: "valid listener configurations",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-1",
								Annotations: map[string]string{"alb.ingress.kubernetes.io/frontend-nlb-listener-port-mapping": "80=443"},
							},
						},
					},
				},
			},
			listenerPortConfigByIngress: map[types.NamespacedName]map[int32]listenPortConfig{
				{Namespace: "awesome-ns", Name: "ing-1"}: {
					443: listenPortConfig{},
				},
			},
			loadBalancer: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					IPAddressType: elbv2model.IPAddressTypeIPV4,
				},
			},
			wantErr: false,
		},
		{
			name: "conflicting listener configurations",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-listener-port-mapping": "80=443",
								},
							},
						}},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-3",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-listener-port-mapping": "80=8443",
								}},
						},
					},
				},
			},
			listenerPortConfigByIngress: map[types.NamespacedName]map[int32]listenPortConfig{
				{Namespace: "awesome-ns", Name: "ing-2"}: {
					443: listenPortConfig{},
				},
				{Namespace: "awesome-ns", Name: "ing-3"}: {
					8443: listenPortConfig{},
				},
			},
			wantErr:        true,
			expectedErrMsg: "failed to merge NLB listenPort config for port: 80",
		},
		{
			name: "valid listener configurations for multiple ingress",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-4",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-listener-port-mapping": "80=443",
								},
							},
						}},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-5",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-listener-port-mapping": "8080=80",
								}},
						},
					},
				},
			},
			listenerPortConfigByIngress: map[types.NamespacedName]map[int32]listenPortConfig{
				{Namespace: "awesome-ns", Name: "ing-4"}: {
					443: listenPortConfig{},
				},
				{Namespace: "awesome-ns", Name: "ing-5"}: {
					80: listenPortConfig{},
				},
			},
			loadBalancer: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					IPAddressType: elbv2model.IPAddressTypeIPV4,
				},
			},
			wantErr: false,
		},
		{
			name: "valid listener configurations for multiple ingress, 8443 is ingnored",
			ingGroup: Group{
				ID: GroupID{
					Namespace: "awesome-ns",
					Name:      "my-ingress",
				},
				Members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-6",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-listener-port-mapping": "80=443",
								},
							},
						}},
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-7",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/frontend-nlb-listener-port-mapping": "80=8443",
								}},
						},
					},
				},
			},
			listenerPortConfigByIngress: map[types.NamespacedName]map[int32]listenPortConfig{
				{Namespace: "awesome-ns", Name: "ing-6"}: {
					443: listenPortConfig{},
				},
				{Namespace: "awesome-ns", Name: "ing-7"}: {},
			},
			loadBalancer: &elbv2model.LoadBalancer{
				Spec: elbv2model.LoadBalancerSpec{
					IPAddressType: elbv2model.IPAddressTypeIPV4,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			desiredState := &core.FrontendNlbTargetGroupDesiredState{
				TargetGroups: make(map[string]*core.FrontendNlbTargetGroupState),
			}
			mockLoadBalancer := elbv2model.NewLoadBalancer(stack, "FrontendNlb", elbv2model.LoadBalancerSpec{
				IPAddressType: elbv2model.IPAddressTypeIPV4,
			})

			task := &defaultModelBuildTask{
				ingGroup:                           tt.ingGroup,
				annotationParser:                   annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				loadBalancer:                       tt.loadBalancer,
				frontendNlb:                        mockLoadBalancer,
				stack:                              stack,
				frontendNlbTargetGroupDesiredState: desiredState,
			}

			err := task.buildFrontendNlbListeners(context.Background(), tt.listenerPortConfigByIngress)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
