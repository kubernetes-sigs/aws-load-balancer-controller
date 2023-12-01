package networking

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	defaultVPCID       = "vpc-xxxyyy"
	defaultClusterName = "testCluster"
)

func Test_defaultBackendSGProvider_Get(t *testing.T) {
	type describeSecurityGroupsAsListCall struct {
		req  *ec2sdk.DescribeSecurityGroupsInput
		resp []*ec2sdk.SecurityGroup
		err  error
	}
	type createSecurityGroupWithContexCall struct {
		req  *ec2sdk.CreateSecurityGroupInput
		resp *ec2sdk.CreateSecurityGroupOutput
		err  error
	}
	type fields struct {
		backendSG       string
		ingResources    []*networking.Ingress
		svcResource     *corev1.Service
		defaultTags     map[string]string
		describeSGCalls []describeSecurityGroupsAsListCall
		createSGCalls   []createSecurityGroupWithContexCall
	}
	defaultEC2Filters := []*ec2sdk.Filter{
		{
			Name:   awssdk.String("vpc-id"),
			Values: awssdk.StringSlice([]string{defaultVPCID}),
		},
		{
			Name:   awssdk.String("tag:elbv2.k8s.aws/cluster"),
			Values: awssdk.StringSlice([]string{"testCluster"}),
		},
		{
			Name:   awssdk.String("tag:elbv2.k8s.aws/resource"),
			Values: awssdk.StringSlice([]string{"backend-sg"}),
		},
	}
	ing := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "awesome-ing",
		},
	}
	ing1 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "name",
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "awesome-svc",
		},
	}
	tests := []struct {
		name    string
		want    string
		fields  fields
		wantErr error
	}{
		{
			name: "backend sg enabled",
			fields: fields{
				backendSG:    "sg-xxx",
				ingResources: []*networking.Ingress{ing},
			},
			want: "sg-xxx",
		},
		{
			name: "backend sg enabled, auto-gen, SG exists",
			fields: fields{
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: defaultEC2Filters,
						},
						resp: []*ec2sdk.SecurityGroup{
							{
								GroupId: awssdk.String("sg-autogen"),
							},
						},
					},
				},
				ingResources: []*networking.Ingress{ing, ing1},
			},
			want: "sg-autogen",
		},
		{
			name: "backend sg enabled, auto-gen new SG",
			fields: fields{
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: defaultEC2Filters,
						},
						err: awserr.New("InvalidGroup.NotFound", "", nil),
					},
				},
				createSGCalls: []createSecurityGroupWithContexCall{
					{
						req: &ec2sdk.CreateSecurityGroupInput{
							Description: awssdk.String(sgDescription),
							GroupName:   awssdk.String("k8s-traffic-testCluster-411a1bcdb1"),
							TagSpecifications: []*ec2sdk.TagSpecification{
								{
									ResourceType: awssdk.String("security-group"),
									Tags: []*ec2sdk.Tag{
										{
											Key:   awssdk.String("elbv2.k8s.aws/cluster"),
											Value: awssdk.String(defaultClusterName),
										},
										{
											Key:   awssdk.String("elbv2.k8s.aws/resource"),
											Value: awssdk.String("backend-sg"),
										},
									},
								},
							},
							VpcId: awssdk.String(defaultVPCID),
						},
						resp: &ec2sdk.CreateSecurityGroupOutput{
							GroupId: awssdk.String("sg-newauto"),
						},
					},
				},
				ingResources: []*networking.Ingress{ing, ing1},
			},
			want: "sg-newauto",
		},
		{
			name: "backend sg enabled, auto-gen new SG with additional defaultTags",
			fields: fields{
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: defaultEC2Filters,
						},
						err: awserr.New("InvalidGroup.NotFound", "", nil),
					},
				},
				createSGCalls: []createSecurityGroupWithContexCall{
					{
						req: &ec2sdk.CreateSecurityGroupInput{
							Description: awssdk.String(sgDescription),
							GroupName:   awssdk.String("k8s-traffic-testCluster-411a1bcdb1"),
							TagSpecifications: []*ec2sdk.TagSpecification{
								{
									ResourceType: awssdk.String("security-group"),
									Tags: []*ec2sdk.Tag{
										{
											Key:   awssdk.String("KubernetesCluster"),
											Value: awssdk.String(defaultClusterName),
										},
										{
											Key:   awssdk.String("defaultTag"),
											Value: awssdk.String("specified"),
										},
										{
											Key:   awssdk.String("zzzKey"),
											Value: awssdk.String("value"),
										},
										{
											Key:   awssdk.String("elbv2.k8s.aws/cluster"),
											Value: awssdk.String(defaultClusterName),
										},
										{
											Key:   awssdk.String("elbv2.k8s.aws/resource"),
											Value: awssdk.String("backend-sg"),
										},
									},
								},
							},
							VpcId: awssdk.String(defaultVPCID),
						},
						resp: &ec2sdk.CreateSecurityGroupOutput{
							GroupId: awssdk.String("sg-newauto"),
						},
					},
				},
				defaultTags: map[string]string{
					"zzzKey":            "value",
					"KubernetesCluster": defaultClusterName,
					"defaultTag":        "specified",
				},
				svcResource: svc,
			},
			want: "sg-newauto",
		},
		{
			name: "describe SG call returns error",
			fields: fields{
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: defaultEC2Filters,
						},
						err: awserr.New("Some.Other.Error", "describe security group as list error", nil),
					},
				},
				ingResources: []*networking.Ingress{ing},
			},
			wantErr: errors.New("Some.Other.Error: describe security group as list error"),
		},
		{
			name: "create SG call returns error",
			fields: fields{
				describeSGCalls: []describeSecurityGroupsAsListCall{
					{
						req: &ec2sdk.DescribeSecurityGroupsInput{
							Filters: defaultEC2Filters,
						},
						err: awserr.New("InvalidGroup.NotFound", "", nil),
					},
				},
				createSGCalls: []createSecurityGroupWithContexCall{
					{
						req: &ec2sdk.CreateSecurityGroupInput{
							Description: awssdk.String(sgDescription),
							GroupName:   awssdk.String("k8s-traffic-testCluster-411a1bcdb1"),
							TagSpecifications: []*ec2sdk.TagSpecification{
								{
									ResourceType: awssdk.String("security-group"),
									Tags: []*ec2sdk.Tag{
										{
											Key:   awssdk.String("elbv2.k8s.aws/cluster"),
											Value: awssdk.String(defaultClusterName),
										},
										{
											Key:   awssdk.String("elbv2.k8s.aws/resource"),
											Value: awssdk.String("backend-sg"),
										},
									},
								},
							},
							VpcId: awssdk.String(defaultVPCID),
						},
						err: awserr.New("Create.Error", "unable to create security group", nil),
					},
				},
				ingResources: []*networking.Ingress{ing1},
			},
			wantErr: errors.New("Create.Error: unable to create security group"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeSGCalls {
				ec2Client.EXPECT().DescribeSecurityGroupsAsList(context.Background(), call.req).Return(call.resp, call.err)
			}
			for _, call := range tt.fields.createSGCalls {
				ec2Client.EXPECT().CreateSecurityGroupWithContext(context.Background(), call.req).Return(call.resp, call.err)
			}
			k8sClient := mock_client.NewMockClient(ctrl)
			sgProvider := NewBackendSGProvider(defaultClusterName, tt.fields.backendSG,
				defaultVPCID, ec2Client, k8sClient, tt.fields.defaultTags, logr.New(&log.NullLogSink{}))

			resourceType := ResourceTypeIngress
			var activeResources []types.NamespacedName
			if len(tt.fields.ingResources) > 0 {
				activeResources = k8s.ToSliceOfNamespacedNames(tt.fields.ingResources)
			} else {
				activeResources = k8s.ToSliceOfNamespacedNames([]*corev1.Service{tt.fields.svcResource})
			}
			got, err := sgProvider.Get(context.Background(), ResourceType(resourceType), activeResources)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultBackendSGProvider_Release(t *testing.T) {
	type env struct {
		ingresses []*networking.Ingress
	}
	type listIngressCall struct {
		ingresses []*networking.Ingress
		err       error
	}
	type listServicesCall struct {
		services []*corev1.Service
		err      error
	}
	type deleteSecurityGroupWithContextCall struct {
		req  *ec2sdk.DeleteSecurityGroupInput
		resp *ec2sdk.DeleteSecurityGroupOutput
		err  error
	}
	type mapItem struct {
		key   metav1.Object
		value bool
	}
	type fields struct {
		autogenSG                  string
		backendSG                  string
		defaultTags                map[string]string
		listIngressCalls           []listIngressCall
		deleteSGCalls              []deleteSecurityGroupWithContextCall
		listServicesCalls          []listServicesCall
		activeIngresses            []*networking.Ingress
		inactiveIngresses          []*networking.Ingress
		svcResource                *corev1.Service
		resourceMapItems           []mapItem
		backendSGRequiredForActive bool
	}
	ing := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "awesome-ing",
		},
	}
	ing1 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "name",
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "awesome-ns",
			Name:      "awesome-svc",
		},
	}
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "svc-1",
		},
	}
	svc2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "svc-2",
		},
	}
	tests := []struct {
		name    string
		env     env
		fields  fields
		wantErr error
	}{
		{
			name: "backend sg specified via flags",
			fields: fields{
				backendSG:         "sg-first",
				inactiveIngresses: []*networking.Ingress{ing},
			},
		},
		{
			name: "backend sg autogenerated",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{
						ingresses: []*networking.Ingress{},
					},
				},
				listServicesCalls: []listServicesCall{
					{
						services: []*corev1.Service{},
					},
				},
				deleteSGCalls: []deleteSecurityGroupWithContextCall{
					{
						req: &ec2sdk.DeleteSecurityGroupInput{
							GroupId: awssdk.String("sg-autogen"),
						},
						resp: &ec2sdk.DeleteSecurityGroupOutput{},
					},
				},
				inactiveIngresses: []*networking.Ingress{ing},
			},
		},
		{
			name: "backend sg required true, for ingress",
			fields: fields{
				autogenSG: "sg-autogen",
				resourceMapItems: []mapItem{
					{
						key:   svc2,
						value: true,
					},
				},
				activeIngresses: []*networking.Ingress{ing},
			},
		},
		{
			name: "backend sg required true, for service",
			fields: fields{
				autogenSG: "sg-autogen",
				resourceMapItems: []mapItem{
					{
						key:   svc2,
						value: true,
					},
				},
				svcResource: svc,
			},
		},
		{
			name: "backend sg requirement true for active resource",
			fields: fields{
				listIngressCalls: []listIngressCall{
					{},
				},
				listServicesCalls: []listServicesCall{
					{},
				},
				resourceMapItems: []mapItem{
					{
						key:   ing,
						value: true,
					},
				},
				backendSGRequiredForActive: true,
			},
		},
		{
			name: "backend sg not required for active ingress",
			fields: fields{
				autogenSG:       "sg-autogen",
				activeIngresses: []*networking.Ingress{ing},
				listIngressCalls: []listIngressCall{
					{
						ingresses: []*networking.Ingress{},
					},
				},
				listServicesCalls: []listServicesCall{
					{
						services: []*corev1.Service{},
					},
				},
				deleteSGCalls: []deleteSecurityGroupWithContextCall{
					{
						req: &ec2sdk.DeleteSecurityGroupInput{
							GroupId: awssdk.String("sg-autogen"),
						},
						resp: &ec2sdk.DeleteSecurityGroupOutput{},
					},
				},
			},
		},
		{
			name: "backend sg required due to standalone ingress",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{
						ingresses: []*networking.Ingress{
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "regular-ns",
									Name:      "ing-nofinalizer",
								},
							},
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:  "awesome-ns",
									Name:       "ing-1",
									Finalizers: []string{"ingress.k8s.aws/resources"},
								},
							},
						},
					},
				},
				inactiveIngresses: []*networking.Ingress{ing},
			},
		},
		{
			name: "backend sg required for ingress group",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{
						ingresses: []*networking.Ingress{
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:  "awesome-ns",
									Name:       "ing-1",
									Finalizers: []string{"group.ingress.k8s.aws/awesome-group"},
								},
							},
						},
					},
				},
				inactiveIngresses: []*networking.Ingress{ing},
			},
		},
		{
			name: "backend sg required for svc",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{},
				},
				listServicesCalls: []listServicesCall{
					{
						services: []*corev1.Service{
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:  "awesome-ns",
									Name:       "svc-1",
									Finalizers: []string{"service.k8s.aws/resources"},
								},
							},
						},
					},
				},
				inactiveIngresses: []*networking.Ingress{ing},
			},
		},
		{
			name: "backend sg requirement for service already known",
			fields: fields{
				autogenSG:         "sg-autogen",
				inactiveIngresses: []*networking.Ingress{ing},
				resourceMapItems: []mapItem{
					{
						key:   svc2,
						value: true,
					},
				},
			},
		},
		{
			name: "backend sg requirement for ingress already known",
			fields: fields{
				autogenSG:         "sg-autogen",
				inactiveIngresses: []*networking.Ingress{ing},
				resourceMapItems: []mapItem{
					{
						key:   ing1,
						value: true,
					},
					{
						key:   svc1,
						value: false,
					},
					{
						key:   svc2,
						value: false,
					},
				},
			},
		},
		{
			name: "backend sg requirement all known, requires delete",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{
						ingresses: []*networking.Ingress{
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:  "ns",
									Name:       "name",
									Finalizers: []string{"ingress.k8s.aws/resources"},
								},
							},
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:  "awesome-ns",
									Name:       "awesome-ing",
									Finalizers: []string{"group.ingress.k8s.aws/awesome-group"},
								},
							},
						},
					},
				},
				listServicesCalls: []listServicesCall{
					{
						services: []*corev1.Service{
							{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:  "awesome-ns",
									Name:       "awesome-svc",
									Finalizers: []string{"service.k8s.aws/resources"},
								},
							},
						},
					},
				},
				deleteSGCalls: []deleteSecurityGroupWithContextCall{
					{
						req: &ec2sdk.DeleteSecurityGroupInput{
							GroupId: awssdk.String("sg-autogen"),
						},
						resp: &ec2sdk.DeleteSecurityGroupOutput{},
					},
				},
				svcResource: svc,
				resourceMapItems: []mapItem{
					{
						key:   ing,
						value: false,
					},
					{
						key:   ing1,
						value: false,
					},
					{
						key:   svc,
						value: false,
					},
				},
			},
		},
		{
			name: "First SG delete attempt fails",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{
						ingresses: []*networking.Ingress{},
					},
				},
				listServicesCalls: []listServicesCall{
					{
						services: []*corev1.Service{},
					},
				},
				deleteSGCalls: []deleteSecurityGroupWithContextCall{
					{
						req: &ec2sdk.DeleteSecurityGroupInput{
							GroupId: awssdk.String("sg-autogen"),
						},
						err: awserr.New("DependencyViolation", "", nil),
					},
					{
						req: &ec2sdk.DeleteSecurityGroupInput{
							GroupId: awssdk.String("sg-autogen"),
						},
						resp: &ec2sdk.DeleteSecurityGroupOutput{},
					},
				},
				inactiveIngresses: []*networking.Ingress{ing},
			},
		},
		{
			name: "SG delete attempt fails return non-dependency violation error",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{},
				},
				listServicesCalls: []listServicesCall{
					{},
				},
				deleteSGCalls: []deleteSecurityGroupWithContextCall{
					{
						req: &ec2sdk.DeleteSecurityGroupInput{
							GroupId: awssdk.String("sg-autogen"),
						},
						err: awserr.New("Something.Else", "unable to delete SG", nil),
					},
				},
				inactiveIngresses: []*networking.Ingress{ing},
			},
			wantErr: errors.New("failed to delete securityGroup: Something.Else: unable to delete SG"),
		},
		{
			name: "k8s ingress list returns error",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{
						err: errors.New("failed"),
					},
				},
				inactiveIngresses: []*networking.Ingress{ing},
			},
			wantErr: errors.New("unable to list ingresses: failed"),
		},
		{
			name: "k8s service list returns error",
			fields: fields{
				autogenSG: "sg-autogen",
				listIngressCalls: []listIngressCall{
					{},
				},
				listServicesCalls: []listServicesCall{
					{
						err: errors.New("failed"),
					},
				},
				inactiveIngresses: []*networking.Ingress{ing},
			},
			wantErr: errors.New("unable to list services: failed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			k8sClient := mock_client.NewMockClient(ctrl)
			sgProvider := NewBackendSGProvider(defaultClusterName, tt.fields.backendSG,
				defaultVPCID, ec2Client, k8sClient, tt.fields.defaultTags, logr.New(&log.NullLogSink{}))
			if len(tt.fields.autogenSG) > 0 {
				sgProvider.backendSG = ""
				sgProvider.autoGeneratedSG = tt.fields.autogenSG
			}
			for _, item := range tt.fields.resourceMapItems {
				var resourceType ResourceType = ResourceTypeIngress
				if reflect.TypeOf(item.key).String() == "*v1.Service" {
					resourceType = ResourceTypeService
				}
				sgProvider.objectsMap.Store(getObjectKey(resourceType, k8s.NamespacedName(item.key)), item.value)
			}
			var deleteCalls []*gomock.Call
			for _, call := range tt.fields.deleteSGCalls {
				deleteCalls = append(deleteCalls, ec2Client.EXPECT().DeleteSecurityGroupWithContext(context.Background(), call.req).Return(call.resp, call.err))
			}
			if len(deleteCalls) > 0 {
				gomock.InAnyOrder(deleteCalls)
			}
			for _, call := range tt.fields.listIngressCalls {
				k8sClient.EXPECT().List(gomock.Any(), &networking.IngressList{}, gomock.Any()).DoAndReturn(
					func(ctx context.Context, ingList *networking.IngressList, opts ...client.ListOption) error {
						for _, ing := range call.ingresses {
							ingList.Items = append(ingList.Items, *(ing.DeepCopy()))
						}
						return call.err
					},
				).AnyTimes()
			}
			for _, call := range tt.fields.listServicesCalls {
				k8sClient.EXPECT().List(gomock.Any(), &corev1.ServiceList{}, gomock.Any()).DoAndReturn(
					func(ctx context.Context, svcList *corev1.ServiceList, opts ...client.ListOption) error {
						for _, svc := range call.services {
							svcList.Items = append(svcList.Items, *(svc.DeepCopy()))
						}
						return call.err
					},
				).AnyTimes()
			}
			for _, ing := range tt.env.ingresses {
				assert.NoError(t, k8sClient.Create(context.Background(), ing.DeepCopy()))
			}
			var inactiveResources []types.NamespacedName
			var resourceType ResourceType = ResourceTypeIngress
			if tt.fields.svcResource != nil {
				resourceType = ResourceTypeService
				inactiveResources = append(inactiveResources, k8s.NamespacedName(tt.fields.svcResource))
			} else {
				inactiveResources = append(inactiveResources, k8s.ToSliceOfNamespacedNames(tt.fields.inactiveIngresses)...)
				if !tt.fields.backendSGRequiredForActive {
					inactiveResources = append(inactiveResources, k8s.ToSliceOfNamespacedNames(tt.fields.activeIngresses)...)
				}
			}
			gotErr := sgProvider.Release(context.Background(), resourceType, k8s.ToSliceOfNamespacedNames(tt.fields.inactiveIngresses))
			if tt.wantErr != nil {
				assert.EqualError(t, gotErr, tt.wantErr.Error())
			} else {
				assert.NoError(t, gotErr)
			}
		})
	}
}
