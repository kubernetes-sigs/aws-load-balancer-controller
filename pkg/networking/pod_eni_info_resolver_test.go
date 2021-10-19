package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultPodENIInfoResolver_Resolve(t *testing.T) {
	nodeA := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69a",
		},
	}
	instanceA := &ec2sdk.Instance{
		InstanceId: awssdk.String("i-0fa2d0064e848c69a"),
		NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
			{
				NetworkInterfaceId: awssdk.String("eni-a"),
				PrivateIpAddresses: []*ec2sdk.InstancePrivateIpAddress{
					{
						PrivateIpAddress: awssdk.String("192.168.200.1"),
					},
					{
						PrivateIpAddress: awssdk.String("192.168.200.2"),
					},
				},
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-a-1"),
					},
				},
			},
		},
	}
	type describeNetworkInterfacesAsListCall struct {
		req  *ec2sdk.DescribeNetworkInterfacesInput
		resp []*ec2sdk.NetworkInterface
		err  error
	}
	type fetchNodeInstancesCall struct {
		nodes                 []*corev1.Node
		nodeInstanceByNodeKey map[types.NamespacedName]*ec2sdk.Instance
		err                   error
	}
	type env struct {
		nodes []*corev1.Node
	}
	type fields struct {
		describeNetworkInterfacesAsListCalls []describeNetworkInterfacesAsListCall
		fetchNodeInstancesCalls              []fetchNodeInstancesCall
	}
	type args struct {
		pods []k8s.PodInfo
	}
	type resolveCall struct {
		args    args
		want    map[types.NamespacedName]ENIInfo
		wantErr error
	}
	tests := []struct {
		name             string
		env              env
		fields           fields
		wantResolveCalls []resolveCall
	}{
		{
			name: "successfully resolve twice without cache hit",
			env: env{
				nodes: []*corev1.Node{nodeA},
			},
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-a", "eni-b"}),
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-c", "eni-d"}),
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-c"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-c-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-d"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-d-1"),
									},
								},
							},
						},
					},
				},
			},
			wantResolveCalls: []resolveCall{
				{
					args: args{
						pods: []k8s.PodInfo{
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-1"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-a",
										PrivateIP: "192.168.100.1",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.1",
							},
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-2"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-b",
										PrivateIP: "192.168.100.2",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.2",
							},
						},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
							NetworkInterfaceID: "eni-a",
							SecurityGroups:     []string{"sg-a-1"},
						},
						types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
							NetworkInterfaceID: "eni-b",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
				{
					args: args{
						pods: []k8s.PodInfo{
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-3"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-c",
										PrivateIP: "192.168.100.3",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.3",
							},
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-4"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-d",
										PrivateIP: "192.168.100.4",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.4",
							},
						},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Namespace: "default", Name: "pod-3"}: {
							NetworkInterfaceID: "eni-c",
							SecurityGroups:     []string{"sg-c-1"},
						},
						types.NamespacedName{Namespace: "default", Name: "pod-4"}: {
							NetworkInterfaceID: "eni-d",
							SecurityGroups:     []string{"sg-d-1"},
						},
					},
				},
			},
		},
		{
			name: "successfully resolve twice with cache partially hit",
			env: env{
				nodes: []*corev1.Node{nodeA},
			},
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-a", "eni-b"}),
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-c"}),
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-c"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-c-1"),
									},
								},
							},
						},
					},
				},
			},
			wantResolveCalls: []resolveCall{
				{
					args: args{
						pods: []k8s.PodInfo{
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-1"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-a",
										PrivateIP: "192.168.100.1",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.1",
							},
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-2"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-b",
										PrivateIP: "192.168.100.2",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.2",
							},
						},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
							NetworkInterfaceID: "eni-a",
							SecurityGroups:     []string{"sg-a-1"},
						},
						types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
							NetworkInterfaceID: "eni-b",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
				{
					args: args{
						pods: []k8s.PodInfo{
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-2"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-b",
										PrivateIP: "192.168.100.2",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.2",
							},
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-3"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-c",
										PrivateIP: "192.168.100.3",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.3",
							},
						},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
							NetworkInterfaceID: "eni-b",
							SecurityGroups:     []string{"sg-b-1"},
						},
						types.NamespacedName{Namespace: "default", Name: "pod-3"}: {
							NetworkInterfaceID: "eni-c",
							SecurityGroups:     []string{"sg-c-1"},
						},
					},
				},
			},
		},
		{
			name: "successfully resolve twice with cache fully hit",
			env: env{
				nodes: []*corev1.Node{nodeA},
			},
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-a", "eni-b"}),
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
				},
			},
			wantResolveCalls: []resolveCall{
				{
					args: args{
						pods: []k8s.PodInfo{
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-1"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-a",
										PrivateIP: "192.168.100.1",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.1",
							},
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-2"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-b",
										PrivateIP: "192.168.100.2",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.2",
							},
						},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
							NetworkInterfaceID: "eni-a",
							SecurityGroups:     []string{"sg-a-1"},
						},
						types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
							NetworkInterfaceID: "eni-b",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
				{
					args: args{
						pods: []k8s.PodInfo{
							{
								Key: types.NamespacedName{Namespace: "default", Name: "pod-2"},
								UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
								ENIInfos: []k8s.PodENIInfo{
									{
										ENIID:     "eni-b",
										PrivateIP: "192.168.100.2",
									},
								},
								NodeName: "node-a",
								PodIP:    "192.168.100.2",
							},
						},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
							NetworkInterfaceID: "eni-b",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
			},
		},
		{
			name: "failed to resolve some pod's ENIInfo",
			env: env{
				nodes: []*corev1.Node{nodeA},
			},
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-abc"}),
								},
								{
									Name:   awssdk.String("addresses.private-ip-address"),
									Values: awssdk.StringSlice([]string{"192.168.200.3"}),
								},
							},
						},
						resp: nil,
					},
				},
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
						},
					},
				},
			},
			wantResolveCalls: []resolveCall{
				{
					args: args{
						pods: []k8s.PodInfo{
							{
								Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
								UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
								NodeName: "node-a",
								PodIP:    "192.168.200.1",
							},
							{
								Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
								UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
								NodeName: "node-a",
								PodIP:    "192.168.200.2",
							},
							{
								Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
								UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
								NodeName: "node-a",
								PodIP:    "192.168.200.3",
							},
						},
					},
					wantErr: errors.New("cannot resolve pod ENI for pods: [default/pod-3]"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeNetworkInterfacesAsListCalls {
				ec2Client.EXPECT().DescribeNetworkInterfacesAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := fake.NewClientBuilder().WithScheme(k8sSchema).Build()
			for _, node := range tt.env.nodes {
				assert.NoError(t, k8sClient.Create(context.Background(), node.DeepCopy()))
			}
			nodeInfoProvider := NewMockNodeInfoProvider(ctrl)
			for _, call := range tt.fields.fetchNodeInstancesCalls {
				updatedNodes := make([]*corev1.Node, 0, len(call.nodes))
				for _, node := range call.nodes {
					updatedNode := &corev1.Node{}
					assert.NoError(t, k8sClient.Get(context.Background(), k8s.NamespacedName(node), updatedNode))
					updatedNodes = append(updatedNodes, updatedNode)
				}
				nodeInfoProvider.EXPECT().FetchNodeInstances(gomock.Any(), gomock.InAnyOrder(updatedNodes)).Return(call.nodeInstanceByNodeKey, call.err)
			}
			r := NewDefaultPodENIInfoResolver(k8sClient, ec2Client, nodeInfoProvider, "vpc-abc", &log.NullLogger{})
			for _, call := range tt.wantResolveCalls {
				got, err := r.Resolve(context.Background(), call.args.pods)
				if call.wantErr != nil {
					assert.EqualError(t, err, call.wantErr.Error())
				} else {
					assert.NoError(t, err)
					assert.Equal(t, call.want, got)
				}
			}
		})
	}
}

func Test_defaultPodENIInfoResolver_resolveViaCascadedLookup(t *testing.T) {
	nodeA := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69a",
		},
	}
	nodeB := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-b",
			Labels: map[string]string{
				"eks.amazonaws.com/compute-type": "fargate",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/xxxxxxxx/fargate-ip-192-168-128-147.us-west-2.compute.internal",
		},
	}
	nodeC := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-c",
			Labels: map[string]string{
				"eks.amazonaws.com/compute-type": "fargate",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/xxxxxxxx/fargate-ip-192-168-128-148.us-west-2.compute.internal",
		},
	}
	instanceA := &ec2sdk.Instance{
		InstanceId: awssdk.String("i-0fa2d0064e848c69a"),
		NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
			{
				NetworkInterfaceId: awssdk.String("eni-a"),
				PrivateIpAddresses: []*ec2sdk.InstancePrivateIpAddress{
					{
						PrivateIpAddress: awssdk.String("192.168.200.1"),
					},
					{
						PrivateIpAddress: awssdk.String("192.168.200.2"),
					},
				},
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-a-1"),
					},
				},
			},
			{
				NetworkInterfaceId: awssdk.String("eni-b"),
				PrivateIpAddresses: []*ec2sdk.InstancePrivateIpAddress{
					{
						PrivateIpAddress: awssdk.String("192.168.200.3"),
					},
					{
						PrivateIpAddress: awssdk.String("192.168.200.4"),
					},
				},
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(1),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-b-1"),
					},
				},
			},
		},
	}
	type describeNetworkInterfacesAsListCall struct {
		req  *ec2sdk.DescribeNetworkInterfacesInput
		resp []*ec2sdk.NetworkInterface
		err  error
	}
	type fetchNodeInstancesCall struct {
		nodes                 []*corev1.Node
		nodeInstanceByNodeKey map[types.NamespacedName]*ec2sdk.Instance
		err                   error
	}
	type env struct {
		nodes []*corev1.Node
	}
	type fields struct {
		describeNetworkInterfacesAsListCalls []describeNetworkInterfacesAsListCall
		fetchNodeInstancesCalls              []fetchNodeInstancesCall
	}
	type args struct {
		pods []k8s.PodInfo
	}
	tests := []struct {
		name    string
		env     env
		fields  fields
		args    args
		want    map[types.NamespacedName]ENIInfo
		wantErr error
	}{
		{
			name: "all pod's ENI resolved via ENI annotation",
			env: env{
				nodes: []*corev1.Node{nodeA},
			},
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-a", "eni-b"}),
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-a",
								PrivateIP: "192.168.100.1",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-b",
								PrivateIP: "192.168.100.2",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.2",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "all pod's ENI resolved via Node's ENIs",
			env: env{
				nodes: []*corev1.Node{nodeA},
			},
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.200.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-a",
						PodIP:    "192.168.200.3",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "all pod's ENI resolved via VPC's ENIs",
			env: env{
				nodes: []*corev1.Node{nodeB, nodeC},
			},
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("addresses.private-ip-address"),
									Values: awssdk.StringSlice([]string{"192.168.128.147", "192.168.128.148"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								PrivateIpAddresses: []*ec2sdk.NetworkInterfacePrivateIpAddress{
									{
										PrivateIpAddress: awssdk.String("192.168.128.146"),
									},
									{
										PrivateIpAddress: awssdk.String("192.168.128.147"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								PrivateIpAddresses: []*ec2sdk.NetworkInterfacePrivateIpAddress{
									{
										PrivateIpAddress: awssdk.String("192.168.128.148"),
									},
									{
										PrivateIpAddress: awssdk.String("192.168.128.149"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-b",
						PodIP:    "192.168.128.147",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-c",
						PodIP:    "192.168.128.148",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "pod's ENI resolved via both ENI annotation and Node ENI and VPC ENI, and some pod's ENI not resolved",
			env: env{
				nodes: []*corev1.Node{nodeA, nodeB},
			},
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-a"}),
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("addresses.private-ip-address"),
									Values: awssdk.StringSlice([]string{"192.168.128.147", "192.168.5.3"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-c"),
								PrivateIpAddresses: []*ec2sdk.NetworkInterfacePrivateIpAddress{
									{
										PrivateIpAddress: awssdk.String("192.168.128.147"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-c-1"),
									},
								},
							},
						},
					},
				},
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-a",
								PrivateIP: "192.168.100.1",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-a",
						PodIP:    "192.168.200.3",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						NodeName: "node-a",
						PodIP:    "192.168.5.3",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-b",
						PodIP:    "192.168.128.147",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-4"}: {
					NetworkInterfaceID: "eni-c",
					SecurityGroups:     []string{"sg-c-1"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeNetworkInterfacesAsListCalls {
				ec2Client.EXPECT().DescribeNetworkInterfacesAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := fake.NewClientBuilder().WithScheme(k8sSchema).Build()
			for _, node := range tt.env.nodes {
				assert.NoError(t, k8sClient.Create(context.Background(), node.DeepCopy()))
			}
			nodeInfoProvider := NewMockNodeInfoProvider(ctrl)
			for _, call := range tt.fields.fetchNodeInstancesCalls {
				updatedNodes := make([]*corev1.Node, 0, len(call.nodes))
				for _, node := range call.nodes {
					updatedNode := &corev1.Node{}
					assert.NoError(t, k8sClient.Get(context.Background(), k8s.NamespacedName(node), updatedNode))
					updatedNodes = append(updatedNodes, updatedNode)
				}
				nodeInfoProvider.EXPECT().FetchNodeInstances(gomock.Any(), gomock.InAnyOrder(updatedNodes)).Return(call.nodeInstanceByNodeKey, call.err)
			}
			r := &defaultPodENIInfoResolver{
				ec2Client:                            ec2Client,
				k8sClient:                            k8sClient,
				nodeInfoProvider:                     nodeInfoProvider,
				vpcID:                                "vpc-0d6d9ee10bd062dcc",
				logger:                               &log.NullLogger{},
				describeNetworkInterfacesIPChunkSize: 2,
			}

			got, err := r.resolveViaCascadedLookup(context.Background(), tt.args.pods)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultPodENIInfoResolver_resolveViaPodENIAnnotation(t *testing.T) {
	type describeNetworkInterfacesAsListCall struct {
		req  *ec2sdk.DescribeNetworkInterfacesInput
		resp []*ec2sdk.NetworkInterface
		err  error
	}
	type fields struct {
		describeNetworkInterfacesAsListCalls []describeNetworkInterfacesAsListCall
	}
	type args struct {
		pods []k8s.PodInfo
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[types.NamespacedName]ENIInfo
		wantErr error
	}{
		{
			name: "multiple pods have matching podENI annotation",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-a", "eni-b"}),
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-a",
								PrivateIP: "192.168.100.1",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-a",
								PrivateIP: "192.168.100.2",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.2",
					},
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-b",
								PrivateIP: "192.168.100.3",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.3",
					},
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-c",
								PrivateIP: "192.168.100.1",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.4",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-5"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-a",
						PodIP:    "192.168.100.5",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-3"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "no pods have matching podENI annotation",
			fields: fields{
				describeNetworkInterfacesAsListCalls: nil,
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-c",
								PrivateIP: "192.168.100.1",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.4",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-5"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-a",
						PodIP:    "192.168.100.5",
					},
				},
			},
			want: nil,
		},
		{
			name: "describeNetworkInterfaces failed",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							NetworkInterfaceIds: awssdk.StringSlice([]string{"eni-a"}),
						},
						err: errors.New("eni eni-a not found"),
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key: types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID: types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						ENIInfos: []k8s.PodENIInfo{
							{
								ENIID:     "eni-a",
								PrivateIP: "192.168.100.1",
							},
						},
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
				},
			},
			wantErr: errors.New("eni eni-a not found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeNetworkInterfacesAsListCalls {
				ec2Client.EXPECT().DescribeNetworkInterfacesAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			r := &defaultPodENIInfoResolver{
				ec2Client: ec2Client,
				logger:    &log.NullLogger{},
			}
			got, err := r.resolveViaPodENIAnnotation(context.Background(), tt.args.pods)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultPodENIInfoResolver_resolveViaNodeENIs(t *testing.T) {
	nodeA := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69a",
		},
	}
	nodeB := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-b",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69b",
		},
	}
	nodeC := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-c",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69c",
		},
	}
	nodeD := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-d",
			Labels: map[string]string{
				"eks.amazonaws.com/compute-type": "fargate",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/xxxxxxxx/fargate-ip-192-168-128-147.us-west-2.compute.internal",
		},
	}
	instanceA := &ec2sdk.Instance{
		InstanceId: awssdk.String("i-0fa2d0064e848c69a"),
		NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
			{
				NetworkInterfaceId: awssdk.String("eni-a-1"),
				PrivateIpAddresses: []*ec2sdk.InstancePrivateIpAddress{
					{
						PrivateIpAddress: awssdk.String("192.168.100.1"),
					},
					{
						PrivateIpAddress: awssdk.String("192.168.100.2"),
					},
				},
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-a-1"),
					},
				},
			},
		},
	}
	instanceB := &ec2sdk.Instance{
		InstanceId: awssdk.String("i-0fa2d0064e848c69b"),
		NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
			{
				NetworkInterfaceId: awssdk.String("eni-b-1"),
				Ipv4Prefixes: []*ec2sdk.InstanceIpv4Prefix{
					{
						Ipv4Prefix: awssdk.String("192.168.142.128/28"),
					},
				},
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-b-1"),
					},
				},
			},
		},
	}
	instanceC := &ec2sdk.Instance{
		InstanceId: awssdk.String("i-0fa2d0064e848c69c"),
		NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
			{
				NetworkInterfaceId: awssdk.String("eni-c-1"),
				PrivateIpAddresses: []*ec2sdk.InstancePrivateIpAddress{
					{
						PrivateIpAddress: awssdk.String("192.168.100.3"),
					},
					{
						PrivateIpAddress: awssdk.String("192.168.100.4"),
					},
				},
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-c-1"),
					},
				},
			},
			{
				NetworkInterfaceId: awssdk.String("eni-c-2"),
				Ipv4Prefixes: []*ec2sdk.InstanceIpv4Prefix{
					{
						Ipv4Prefix: awssdk.String("192.168.172.128/28"),
					},
				},
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-c-2"),
					},
				},
			},
		},
	}
	type env struct {
		nodes []*corev1.Node
	}
	type fetchNodeInstancesCall struct {
		nodes                 []*corev1.Node
		nodeInstanceByNodeKey map[types.NamespacedName]*ec2sdk.Instance
		err                   error
	}
	type fields struct {
		fetchNodeInstancesCalls []fetchNodeInstancesCall
	}
	type args struct {
		pods []k8s.PodInfo
	}
	tests := []struct {
		name    string
		env     env
		fields  fields
		args    args
		want    map[types.NamespacedName]ENIInfo
		wantErr error
	}{
		{
			name: "successfully resolved all pod's ENI",
			env: env{
				nodes: []*corev1.Node{nodeA, nodeB, nodeC},
			},
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB, nodeC},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
							types.NamespacedName{Name: "node-b"}: instanceB,
							types.NamespacedName{Name: "node-c"}: instanceC,
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.142.130",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						NodeName: "node-c",
						PodIP:    "192.168.100.3",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-c",
						PodIP:    "192.168.172.133",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a-1",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b-1",
					SecurityGroups:     []string{"sg-b-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-3"}: {
					NetworkInterfaceID: "eni-c-1",
					SecurityGroups:     []string{"sg-c-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-4"}: {
					NetworkInterfaceID: "eni-c-2",
					SecurityGroups:     []string{"sg-c-2"},
				},
			},
		},
		{
			name: "some pod's ENI cannot be resolved due to node is fargate node",
			env: env{
				nodes: []*corev1.Node{nodeA, nodeB, nodeD},
			},
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
							types.NamespacedName{Name: "node-b"}: instanceB,
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.142.130",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						NodeName: "node-d",
						PodIP:    "192.168.128.147",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a-1",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b-1",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "some pod's ENI cannot be resolved due to node's instance is not found",
			env: env{
				nodes: []*corev1.Node{nodeA, nodeB, nodeC},
			},
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB, nodeC},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
							types.NamespacedName{Name: "node-c"}: instanceC,
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.142.130",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						NodeName: "node-c",
						PodIP:    "192.168.100.3",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-c",
						PodIP:    "192.168.172.133",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a-1",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-3"}: {
					NetworkInterfaceID: "eni-c-1",
					SecurityGroups:     []string{"sg-c-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-4"}: {
					NetworkInterfaceID: "eni-c-2",
					SecurityGroups:     []string{"sg-c-2"},
				},
			},
		},
		{
			name: "some pod's ENI cannot be resolved due to node's ENI don't support podIP",
			env: env{
				nodes: []*corev1.Node{nodeA, nodeB, nodeC},
			},
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB, nodeC},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
							types.NamespacedName{Name: "node-b"}: instanceB,
							types.NamespacedName{Name: "node-c"}: instanceC,
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.142.130",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						NodeName: "node-c",
						PodIP:    "192.168.100.3",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-c",
						PodIP:    "192.168.199.133",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a-1",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b-1",
					SecurityGroups:     []string{"sg-b-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-3"}: {
					NetworkInterfaceID: "eni-c-1",
					SecurityGroups:     []string{"sg-c-1"},
				},
			},
		},
		{
			name: "failed to fetch nodes",
			env: env{
				nodes: []*corev1.Node{nodeA, nodeB, nodeC},
			},
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB, nodeC},
						err:   errors.New("instance i-0fa2d0064e848c69b not found"),
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.142.130",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						NodeName: "node-c",
						PodIP:    "192.168.100.3",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-c",
						PodIP:    "192.168.172.133",
					},
				},
			},
			wantErr: errors.New("instance i-0fa2d0064e848c69b not found"),
		},
		{
			name: "all nodes are fargate nodes",
			env: env{
				nodes: []*corev1.Node{nodeA, nodeB, nodeD},
			},
			fields: fields{
				fetchNodeInstancesCalls: nil,
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-d",
						PodIP:    "192.168.128.147",
					},
				},
			},
			want: nil,
		},
		{
			name: "empty nodes",
			env: env{
				nodes: []*corev1.Node{nodeA, nodeB, nodeD},
			},
			fields: fields{
				fetchNodeInstancesCalls: nil,
			},
			args: args{
				pods: nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := fake.NewClientBuilder().WithScheme(k8sSchema).Build()
			for _, node := range tt.env.nodes {
				assert.NoError(t, k8sClient.Create(context.Background(), node.DeepCopy()))
			}
			nodeInfoProvider := NewMockNodeInfoProvider(ctrl)
			for _, call := range tt.fields.fetchNodeInstancesCalls {
				updatedNodes := make([]*corev1.Node, 0, len(call.nodes))
				for _, node := range call.nodes {
					updatedNode := &corev1.Node{}
					assert.NoError(t, k8sClient.Get(context.Background(), k8s.NamespacedName(node), updatedNode))
					updatedNodes = append(updatedNodes, updatedNode)
				}
				nodeInfoProvider.EXPECT().FetchNodeInstances(gomock.Any(), gomock.InAnyOrder(updatedNodes)).Return(call.nodeInstanceByNodeKey, call.err)
			}

			r := &defaultPodENIInfoResolver{
				k8sClient:        k8sClient,
				nodeInfoProvider: nodeInfoProvider,
				logger:           &log.NullLogger{},
			}
			got, err := r.resolveViaNodeENIs(context.Background(), tt.args.pods)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultPodENIInfoResolver_resolveViaVPCENIs(t *testing.T) {
	type describeNetworkInterfacesAsListCall struct {
		req  *ec2sdk.DescribeNetworkInterfacesInput
		resp []*ec2sdk.NetworkInterface
		err  error
	}
	type fields struct {
		describeNetworkInterfacesAsListCalls []describeNetworkInterfacesAsListCall
	}
	type args struct {
		pods []k8s.PodInfo
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[types.NamespacedName]ENIInfo
		wantErr error
	}{
		{
			name: "successfully resolved all pod's ENI - 1 IP chunks",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("addresses.private-ip-address"),
									Values: awssdk.StringSlice([]string{"192.168.100.1", "192.168.100.3"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								PrivateIpAddresses: []*ec2sdk.NetworkInterfacePrivateIpAddress{
									{
										PrivateIpAddress: awssdk.String("192.168.100.1"),
									},
									{
										PrivateIpAddress: awssdk.String("192.168.100.2"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								PrivateIpAddresses: []*ec2sdk.NetworkInterfacePrivateIpAddress{
									{
										PrivateIpAddress: awssdk.String("192.168.100.3"),
									},
									{
										PrivateIpAddress: awssdk.String("192.168.100.4"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.100.3",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "successfully resolved all pod's ENI - 2 IP chunks",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("addresses.private-ip-address"),
									Values: awssdk.StringSlice([]string{"192.168.100.1", "192.168.100.2"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								PrivateIpAddresses: []*ec2sdk.NetworkInterfacePrivateIpAddress{
									{
										PrivateIpAddress: awssdk.String("192.168.100.1"),
									},
									{
										PrivateIpAddress: awssdk.String("192.168.100.2"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("addresses.private-ip-address"),
									Values: awssdk.StringSlice([]string{"192.168.100.3", "192.168.100.4"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{

							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								PrivateIpAddresses: []*ec2sdk.NetworkInterfacePrivateIpAddress{
									{
										PrivateIpAddress: awssdk.String("192.168.100.3"),
									},
									{
										PrivateIpAddress: awssdk.String("192.168.100.4"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.100.2",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						NodeName: "node-c",
						PodIP:    "192.168.100.3",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-d",
						PodIP:    "192.168.100.4",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-3"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-4"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "some pod's ENI cannot be resolved due to ENI don't contain podIP",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("addresses.private-ip-address"),
									Values: awssdk.StringSlice([]string{"192.168.100.1", "192.168.100.3"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								PrivateIpAddresses: []*ec2sdk.NetworkInterfacePrivateIpAddress{
									{
										PrivateIpAddress: awssdk.String("192.168.100.1"),
									},
									{
										PrivateIpAddress: awssdk.String("192.168.100.2"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.100.3",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
			},
		},
		{
			name: "failed to call describeNetworkInterfacesAsList",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("addresses.private-ip-address"),
									Values: awssdk.StringSlice([]string{"192.168.100.1", "192.168.100.3"}),
								},
							},
						},
						err: errors.New("some AWS API Error"),
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "192.168.100.1",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "192.168.100.3",
					},
				},
			},
			wantErr: errors.New("some AWS API Error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeNetworkInterfacesAsListCalls {
				ec2Client.EXPECT().DescribeNetworkInterfacesAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			r := &defaultPodENIInfoResolver{
				ec2Client:                            ec2Client,
				vpcID:                                "vpc-0d6d9ee10bd062dcc",
				logger:                               &log.NullLogger{},
				describeNetworkInterfacesIPChunkSize: 2,
			}
			got, err := r.resolveViaVPCENIs(context.Background(), tt.args.pods)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultPodENIInfoResolver_resolveViaVPCENIsForIPv6(t *testing.T) {
	type describeNetworkInterfacesAsListCall struct {
		req  *ec2sdk.DescribeNetworkInterfacesInput
		resp []*ec2sdk.NetworkInterface
		err  error
	}
	type fields struct {
		describeNetworkInterfacesAsListCalls []describeNetworkInterfacesAsListCall
	}
	type args struct {
		pods []k8s.PodInfo
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[types.NamespacedName]ENIInfo
		wantErr error
	}{
		{
			name: "successfully resolved all pod's ENI - 1 IP chunks",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("ipv6-addresses.ipv6-address"),
									Values: awssdk.StringSlice([]string{"2001:0db8:85a3:0000:0000:8a2e:0370:ee50", "2001:0db8:85a3:0000:0000:9704:6c49:9e7d"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Ipv6Addresses: []*ec2sdk.NetworkInterfaceIpv6Address{
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:8a2e:0370:ee50"),
									},
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:8a2e:0370:ee52"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								Ipv6Addresses: []*ec2sdk.NetworkInterfaceIpv6Address{
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:9704:6c49:9e70"),
									},
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:9704:6c49:9e7d"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "2001:0db8:85a3:0000:0000:8a2e:0370:ee50",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "2001:0db8:85a3:0000:0000:9704:6c49:9e7d",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "successfully resolved all pod's ENI - 2 IP chunks",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("ipv6-addresses.ipv6-address"),
									Values: awssdk.StringSlice([]string{"2001:0db8:85a3:0000:0000:8a2e:0370:ee50", "2001:0db8:85a3:0000:0000:9704:6c49:9e7d"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Ipv6Addresses: []*ec2sdk.NetworkInterfaceIpv6Address{
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:8a2e:0370:ee50"),
									},
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:9704:6c49:9e7d"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
						},
					},
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("ipv6-addresses.ipv6-address"),
									Values: awssdk.StringSlice([]string{"2001:0db8:85a3:0000:0000:8493:9af3:a786", "2001:0db8:85a3:0000:0000:3f04:39e7:58d9"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-b"),
								Ipv6Addresses: []*ec2sdk.NetworkInterfaceIpv6Address{
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:8493:9af3:a786"),
									},
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:3f04:39e7:58d9"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-b-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "2001:0db8:85a3:0000:0000:8a2e:0370:ee50",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "2001:0db8:85a3:0000:0000:9704:6c49:9e7d",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-3"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc03"),
						NodeName: "node-c",
						PodIP:    "2001:0db8:85a3:0000:0000:8493:9af3:a786",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-4"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc04"),
						NodeName: "node-d",
						PodIP:    "2001:0db8:85a3:0000:0000:3f04:39e7:58d9",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-2"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-3"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
				types.NamespacedName{Namespace: "default", Name: "pod-4"}: {
					NetworkInterfaceID: "eni-b",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "some pod's ENI cannot be resolved due to ENI don't contain podIP",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("ipv6-addresses.ipv6-address"),
									Values: awssdk.StringSlice([]string{"2001:0db8:85a3:0000:0000:8493:9af3:a786", "2001:0db8:85a3:0000:0000:3f04:39e7:58d9"}),
								},
							},
						},
						resp: []*ec2sdk.NetworkInterface{
							{
								NetworkInterfaceId: awssdk.String("eni-a"),
								Ipv6Addresses: []*ec2sdk.NetworkInterfaceIpv6Address{
									{
										Ipv6Address: awssdk.String("2001:0db8:85a3:0000:0000:8493:9af3:a786"),
									},
								},
								Groups: []*ec2sdk.GroupIdentifier{
									{
										GroupId: awssdk.String("sg-a-1"),
									},
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "2001:0db8:85a3:0000:0000:8493:9af3:a786",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "2001:0db8:85a3:0000:0000:3f04:39e7:58d9",
					},
				},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Namespace: "default", Name: "pod-1"}: {
					NetworkInterfaceID: "eni-a",
					SecurityGroups:     []string{"sg-a-1"},
				},
			},
		},
		{
			name: "failed to call describeNetworkInterfacesAsList",
			fields: fields{
				describeNetworkInterfacesAsListCalls: []describeNetworkInterfacesAsListCall{
					{
						req: &ec2sdk.DescribeNetworkInterfacesInput{
							Filters: []*ec2sdk.Filter{
								{
									Name:   awssdk.String("vpc-id"),
									Values: awssdk.StringSlice([]string{"vpc-0d6d9ee10bd062dcc"}),
								},
								{
									Name:   awssdk.String("ipv6-addresses.ipv6-address"),
									Values: awssdk.StringSlice([]string{"2001:0db8:85a3:0000:0000:8493:9af3:a786", "2001:0db8:85a3:0000:0000:3f04:39e7:58d9"}),
								},
							},
						},
						err: errors.New("some AWS API Error"),
					},
				},
			},
			args: args{
				pods: []k8s.PodInfo{
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-1"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc01"),
						NodeName: "node-a",
						PodIP:    "2001:0db8:85a3:0000:0000:8493:9af3:a786",
					},
					{
						Key:      types.NamespacedName{Namespace: "default", Name: "pod-2"},
						UID:      types.UID("2d8740a6-f4b1-4074-a91c-f0084ec0bc02"),
						NodeName: "node-b",
						PodIP:    "2001:0db8:85a3:0000:0000:3f04:39e7:58d9",
					},
				},
			},
			wantErr: errors.New("some AWS API Error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeNetworkInterfacesAsListCalls {
				ec2Client.EXPECT().DescribeNetworkInterfacesAsList(gomock.Any(), gomock.Any()).Return(call.resp, call.err)
			}
			r := &defaultPodENIInfoResolver{
				ec2Client:                            ec2Client,
				vpcID:                                "vpc-0d6d9ee10bd062dcc",
				logger:                               &log.NullLogger{},
				describeNetworkInterfacesIPChunkSize: 2,
			}
			got, err := r.resolveViaVPCENIs(context.Background(), tt.args.pods)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_computePodENIInfoCacheKey(t *testing.T) {
	type args struct {
		pod k8s.PodInfo
	}
	tests := []struct {
		name string
		args args
		want podENIInfoCacheKey
	}{
		{
			name: "pods UID should be included as cacheKey",
			args: args{
				pod: k8s.PodInfo{
					Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
					UID: types.UID("uuid"),
				},
			},
			want: podENIInfoCacheKey{
				podKey: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				podUID: "uuid",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computePodENIInfoCacheKey(tt.args.pod)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_computePodsWithoutENIInfo(t *testing.T) {
	type args struct {
		pods            []k8s.PodInfo
		eniInfoByPodKey map[types.NamespacedName]ENIInfo
	}
	tests := []struct {
		name string
		args args
		want []k8s.PodInfo
	}{
		{
			name: "all pods are resolved",
			args: args{
				pods: []k8s.PodInfo{
					{
						Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
					},
					{
						Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-2"},
					},
					{
						Key: types.NamespacedName{Namespace: "ns-2", Name: "pod-1"},
					},
				},
				eniInfoByPodKey: map[types.NamespacedName]ENIInfo{
					types.NamespacedName{Namespace: "ns-1", Name: "pod-1"}: {
						NetworkInterfaceID: "eni-xx",
					},
					types.NamespacedName{Namespace: "ns-1", Name: "pod-2"}: {
						NetworkInterfaceID: "eni-xx",
					},
					types.NamespacedName{Namespace: "ns-2", Name: "pod-1"}: {
						NetworkInterfaceID: "eni-xx",
					},
				},
			},
			want: []k8s.PodInfo{},
		},
		{
			name: "some pods are resolved",
			args: args{
				pods: []k8s.PodInfo{
					{
						Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
					},
					{
						Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-3"},
					},
					{
						Key: types.NamespacedName{Namespace: "ns-2", Name: "pod-1"},
					},
				},
				eniInfoByPodKey: map[types.NamespacedName]ENIInfo{
					types.NamespacedName{Namespace: "ns-1", Name: "pod-1"}: {
						NetworkInterfaceID: "eni-xx",
					},
					types.NamespacedName{Namespace: "ns-1", Name: "pod-2"}: {
						NetworkInterfaceID: "eni-xx",
					},
					types.NamespacedName{Namespace: "ns-2", Name: "pod-1"}: {
						NetworkInterfaceID: "eni-xx",
					},
				},
			},
			want: []k8s.PodInfo{
				{
					Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-3"},
				},
			},
		},
		{
			name: "no pods are resolved",
			args: args{
				pods: []k8s.PodInfo{
					{
						Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
					},
					{
						Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-3"},
					},
					{
						Key: types.NamespacedName{Namespace: "ns-2", Name: "pod-1"},
					},
				},
				eniInfoByPodKey: map[types.NamespacedName]ENIInfo{},
			},
			want: []k8s.PodInfo{
				{
					Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				},
				{
					Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-3"},
				},
				{
					Key: types.NamespacedName{Namespace: "ns-2", Name: "pod-1"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computePodsWithoutENIInfo(tt.args.pods, tt.args.eniInfoByPodKey)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultPodENIInfoResolver_isPodSupportedByNodeENI(t *testing.T) {
	type args struct {
		pod     k8s.PodInfo
		nodeENI *ec2sdk.InstanceNetworkInterface
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "pod's IPv4 address is supported by ipv4Addresses in ENI",
			args: args{
				pod: k8s.PodInfo{
					PodIP: "192.168.100.23",
				},
				nodeENI: &ec2sdk.InstanceNetworkInterface{
					PrivateIpAddresses: []*ec2sdk.InstancePrivateIpAddress{
						{
							PrivateIpAddress: awssdk.String("192.168.100.22"),
						},
						{
							PrivateIpAddress: awssdk.String("192.168.100.23"),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pod's IPv4 address is not supported by ipv4Addresses in ENI",
			args: args{
				pod: k8s.PodInfo{
					PodIP: "192.168.100.21",
				},
				nodeENI: &ec2sdk.InstanceNetworkInterface{
					PrivateIpAddresses: []*ec2sdk.InstancePrivateIpAddress{
						{
							PrivateIpAddress: awssdk.String("192.168.100.22"),
						},
						{
							PrivateIpAddress: awssdk.String("192.168.100.23"),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod's IPv4 address is supported by ipv4Prefix in ENI",
			args: args{
				pod: k8s.PodInfo{
					PodIP: "192.168.172.140",
				},
				nodeENI: &ec2sdk.InstanceNetworkInterface{
					Ipv4Prefixes: []*ec2sdk.InstanceIpv4Prefix{
						{
							Ipv4Prefix: awssdk.String("192.168.197.64/28"),
						},
						{
							Ipv4Prefix: awssdk.String("192.168.172.128/28"),
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pod's IPv4 address is not supported by ipv4Prefix in ENI",
			args: args{
				pod: k8s.PodInfo{
					PodIP: "192.168.100.23",
				},
				nodeENI: &ec2sdk.InstanceNetworkInterface{
					Ipv4Prefixes: []*ec2sdk.InstanceIpv4Prefix{
						{
							Ipv4Prefix: awssdk.String("192.168.197.64/28"),
						},
						{
							Ipv4Prefix: awssdk.String("192.168.172.128/28"),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod's IPv4 address is invalid",
			args: args{
				pod: k8s.PodInfo{
					PodIP: "abcdefg",
				},
				nodeENI: &ec2sdk.InstanceNetworkInterface{
					Ipv4Prefixes: []*ec2sdk.InstanceIpv4Prefix{
						{
							Ipv4Prefix: awssdk.String("192.168.197.64/28"),
						},
						{
							Ipv4Prefix: awssdk.String("192.168.172.128/28"),
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &defaultPodENIInfoResolver{}
			got := r.isPodSupportedByNodeENI(tt.args.pod, tt.args.nodeENI)
			assert.Equal(t, tt.want, got)
		})
	}
}
