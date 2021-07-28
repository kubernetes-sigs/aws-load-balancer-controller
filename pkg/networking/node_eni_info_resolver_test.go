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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultNodeENIInfoResolver_Resolve(t *testing.T) {
	nodeA := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
			UID:  types.UID("ab397451-bdd3-43be-b606-8e79609e7f1a"),
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69a",
		},
	}
	nodeB := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-b",
			UID:  types.UID("ab397451-bdd3-43be-b606-8e79609e7f1b"),
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69b",
		},
	}
	nodeC := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-c",
			UID:  types.UID("ab397451-bdd3-43be-b606-8e79609e7f1c"),
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69c",
		},
	}
	nodeD := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-d",
			UID:  types.UID("ab397451-bdd3-43be-b606-8e79609e7f1d"),
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69d",
		},
	}
	instanceA := &ec2sdk.Instance{
		InstanceId: awssdk.String("i-0fa2d0064e848c69a"),
		NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
			{
				NetworkInterfaceId: awssdk.String("eni-a-1"),
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
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-c-1"),
					},
				},
			},
		},
	}
	instanceD := &ec2sdk.Instance{
		InstanceId: awssdk.String("i-0fa2d0064e848c69d"),
		NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
			{
				NetworkInterfaceId: awssdk.String("eni-d-1"),
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
				Groups: []*ec2sdk.GroupIdentifier{
					{
						GroupId: awssdk.String("sg-d-1"),
					},
				},
			},
		},
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
		nodes []*corev1.Node
	}
	type resolveCall struct {
		args    args
		want    map[types.NamespacedName]ENIInfo
		wantErr error
	}
	tests := []struct {
		name             string
		fields           fields
		wantResolveCalls []resolveCall
	}{
		{
			name: "successfully resolve twice without cache hit",
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
							types.NamespacedName{Name: "node-b"}: instanceB,
						},
					},
					{
						nodes: []*corev1.Node{nodeC, nodeD},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-c"}: instanceC,
							types.NamespacedName{Name: "node-d"}: instanceD,
						},
					},
				},
			},
			wantResolveCalls: []resolveCall{
				{
					args: args{
						nodes: []*corev1.Node{nodeA, nodeB},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Name: "node-a"}: {
							NetworkInterfaceID: "eni-a-1",
							SecurityGroups:     []string{"sg-a-1"},
						},
						types.NamespacedName{Name: "node-b"}: {
							NetworkInterfaceID: "eni-b-1",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
				{
					args: args{
						nodes: []*corev1.Node{nodeC, nodeD},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Name: "node-c"}: {
							NetworkInterfaceID: "eni-c-1",
							SecurityGroups:     []string{"sg-c-1"},
						},
						types.NamespacedName{Name: "node-d"}: {
							NetworkInterfaceID: "eni-d-1",
							SecurityGroups:     []string{"sg-d-1"},
						},
					},
				},
			},
		},
		{
			name: "successfully resolve twice with cache partially hit",
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
							types.NamespacedName{Name: "node-b"}: instanceB,
						},
					},
					{
						nodes: []*corev1.Node{nodeC},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-c"}: instanceC,
						},
					},
				},
			},
			wantResolveCalls: []resolveCall{
				{
					args: args{
						nodes: []*corev1.Node{nodeA, nodeB},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Name: "node-a"}: {
							NetworkInterfaceID: "eni-a-1",
							SecurityGroups:     []string{"sg-a-1"},
						},
						types.NamespacedName{Name: "node-b"}: {
							NetworkInterfaceID: "eni-b-1",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
				{
					args: args{
						nodes: []*corev1.Node{nodeB, nodeC},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Name: "node-b"}: {
							NetworkInterfaceID: "eni-b-1",
							SecurityGroups:     []string{"sg-b-1"},
						},
						types.NamespacedName{Name: "node-c"}: {
							NetworkInterfaceID: "eni-c-1",
							SecurityGroups:     []string{"sg-c-1"},
						},
					},
				},
			},
		},
		{
			name: "successfully resolve twice with cache fully hit",
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
			wantResolveCalls: []resolveCall{
				{
					args: args{
						nodes: []*corev1.Node{nodeA, nodeB},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Name: "node-a"}: {
							NetworkInterfaceID: "eni-a-1",
							SecurityGroups:     []string{"sg-a-1"},
						},
						types.NamespacedName{Name: "node-b"}: {
							NetworkInterfaceID: "eni-b-1",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
				{
					args: args{
						nodes: []*corev1.Node{nodeB},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Name: "node-b"}: {
							NetworkInterfaceID: "eni-b-1",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
			},
		},
		{
			name: "failed to resolve some node's ENIInfo",
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: instanceA,
							types.NamespacedName{Name: "node-b"}: instanceB,
						},
					},
					{
						nodes:                 []*corev1.Node{nodeC},
						nodeInstanceByNodeKey: nil,
					},
				},
			},
			wantResolveCalls: []resolveCall{
				{
					args: args{
						nodes: []*corev1.Node{nodeA, nodeB},
					},
					want: map[types.NamespacedName]ENIInfo{
						types.NamespacedName{Name: "node-a"}: {
							NetworkInterfaceID: "eni-a-1",
							SecurityGroups:     []string{"sg-a-1"},
						},
						types.NamespacedName{Name: "node-b"}: {
							NetworkInterfaceID: "eni-b-1",
							SecurityGroups:     []string{"sg-b-1"},
						},
					},
				},
				{
					args: args{
						nodes: []*corev1.Node{nodeB, nodeC},
					},
					wantErr: errors.New("cannot resolve node ENI for nodes: [/node-c]"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			nodeInfoProvider := NewMockNodeInfoProvider(ctrl)
			for _, call := range tt.fields.fetchNodeInstancesCalls {
				nodeInfoProvider.EXPECT().FetchNodeInstances(gomock.Any(), call.nodes).Return(call.nodeInstanceByNodeKey, call.err)
			}
			r := NewDefaultNodeENIInfoResolver(nodeInfoProvider, &log.NullLogger{})
			for _, call := range tt.wantResolveCalls {
				got, err := r.Resolve(context.Background(), call.args.nodes)
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

func Test_defaultNodeENIInfoResolver_resolveViaInstanceID(t *testing.T) {
	nodeA := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
			UID:  types.UID("ab397451-bdd3-43be-b606-8e79609e7f1a"),
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69a",
		},
	}
	nodeB := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-b",
			UID:  types.UID("ab397451-bdd3-43be-b606-8e79609e7f1b"),
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69b",
		},
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
		nodes []*corev1.Node
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[types.NamespacedName]ENIInfo
		wantErr error
	}{
		{
			name: "found instances for all nodes",
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: {
								InstanceId: awssdk.String("i-0fa2d0064e848c69a"),
								NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
									{
										NetworkInterfaceId: awssdk.String("eni-a-1"),
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
							},
							types.NamespacedName{Name: "node-b"}: {
								InstanceId: awssdk.String("i-0fa2d0064e848c69b"),
								NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
									{
										NetworkInterfaceId: awssdk.String("eni-b-1"),
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
							},
						},
					},
				},
			},
			args: args{
				nodes: []*corev1.Node{nodeA, nodeB},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Name: "node-a"}: {
					NetworkInterfaceID: "eni-a-1",
					SecurityGroups:     []string{"sg-a-1"},
				},
				types.NamespacedName{Name: "node-b"}: {
					NetworkInterfaceID: "eni-b-1",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "found instances for some nodes",
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-b"}: {
								InstanceId: awssdk.String("i-0fa2d0064e848c69b"),
								NetworkInterfaces: []*ec2sdk.InstanceNetworkInterface{
									{
										NetworkInterfaceId: awssdk.String("eni-b-1"),
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
							},
						},
					},
				},
			},
			args: args{
				nodes: []*corev1.Node{nodeA, nodeB},
			},
			want: map[types.NamespacedName]ENIInfo{
				types.NamespacedName{Name: "node-b"}: {
					NetworkInterfaceID: "eni-b-1",
					SecurityGroups:     []string{"sg-b-1"},
				},
			},
		},
		{
			name: "fetchNodeInstance failed",
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA, nodeB},
						err:   errors.New("instance i-0fa2d0064e848c69a is not found"),
					},
				},
			},
			args: args{
				nodes: []*corev1.Node{nodeA, nodeB},
			},
			wantErr: errors.New("instance i-0fa2d0064e848c69a is not found"),
		},
		{
			name: "instance don't have primary ENI",
			fields: fields{
				fetchNodeInstancesCalls: []fetchNodeInstancesCall{
					{
						nodes: []*corev1.Node{nodeA},
						nodeInstanceByNodeKey: map[types.NamespacedName]*ec2sdk.Instance{
							types.NamespacedName{Name: "node-a"}: {
								InstanceId: awssdk.String("i-0fa2d0064e848c69a"),
							},
						},
					},
				},
			},
			args: args{
				nodes: []*corev1.Node{nodeA},
			},
			wantErr: errors.New("[this should never happen] no primary ENI found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			nodeInfoProvider := NewMockNodeInfoProvider(ctrl)
			for _, call := range tt.fields.fetchNodeInstancesCalls {
				nodeInfoProvider.EXPECT().FetchNodeInstances(gomock.Any(), call.nodes).Return(call.nodeInstanceByNodeKey, call.err)
			}
			r := &defaultNodeENIInfoResolver{
				nodeInfoProvider: nodeInfoProvider,
				logger:           &log.NullLogger{},
			}
			got, err := r.resolveViaInstanceID(context.Background(), tt.args.nodes)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_findInstancePrimaryENI(t *testing.T) {
	type args struct {
		enis []*ec2sdk.InstanceNetworkInterface
	}
	tests := []struct {
		name    string
		args    args
		want    *ec2sdk.InstanceNetworkInterface
		wantErr error
	}{
		{
			name: "one eni",
			args: args{
				enis: []*ec2sdk.InstanceNetworkInterface{
					{
						NetworkInterfaceId: awssdk.String("eni-1"),
						Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
							DeviceIndex: awssdk.Int64(0),
						},
					},
				},
			},
			want: &ec2sdk.InstanceNetworkInterface{
				NetworkInterfaceId: awssdk.String("eni-1"),
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
			},
		},
		{
			name: "two eni",
			args: args{
				enis: []*ec2sdk.InstanceNetworkInterface{
					{
						NetworkInterfaceId: awssdk.String("eni-1"),
						Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
							DeviceIndex: awssdk.Int64(1),
						},
					},
					{
						NetworkInterfaceId: awssdk.String("eni-2"),
						Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
							DeviceIndex: awssdk.Int64(0),
						},
					},
				},
			},
			want: &ec2sdk.InstanceNetworkInterface{
				NetworkInterfaceId: awssdk.String("eni-2"),
				Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
					DeviceIndex: awssdk.Int64(0),
				},
			},
		},
		{
			name: "no primary ENI",
			args: args{
				enis: []*ec2sdk.InstanceNetworkInterface{
					{
						NetworkInterfaceId: awssdk.String("eni-1"),
						Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
							DeviceIndex: awssdk.Int64(1),
						},
					},
					{
						NetworkInterfaceId: awssdk.String("eni-2"),
						Attachment: &ec2sdk.InstanceNetworkInterfaceAttachment{
							DeviceIndex: awssdk.Int64(2),
						},
					},
				},
			},
			wantErr: errors.New("[this should never happen] no primary ENI found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findInstancePrimaryENI(tt.args.enis)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_computeNodeENIInfoCacheKey(t *testing.T) {
	type args struct {
		node *corev1.Node
	}
	tests := []struct {
		name string
		args args
		want nodeENIInfoCacheKey
	}{
		{
			name: "node UID should be included as cacheKey",
			args: args{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "node-1",
						UID:       "uuid",
					},
				},
			},
			want: nodeENIInfoCacheKey{
				nodeKey: types.NamespacedName{Namespace: "ns-1", Name: "node-1"},
				nodeUID: "uuid",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeNodeENIInfoCacheKey(tt.args.node)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_computeNodesWithoutENIInfo(t *testing.T) {
	type args struct {
		nodes            []*corev1.Node
		eniInfoByNodeKey map[types.NamespacedName]ENIInfo
	}
	tests := []struct {
		name string
		args args
		want []*corev1.Node
	}{
		{
			name: "all nodes are resolved",
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "node-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "node-2",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-2",
							Name:      "node-1",
						},
					},
				},
				eniInfoByNodeKey: map[types.NamespacedName]ENIInfo{
					types.NamespacedName{Namespace: "ns-1", Name: "node-1"}: {
						NetworkInterfaceID: "eni-xx",
					},
					types.NamespacedName{Namespace: "ns-1", Name: "node-2"}: {
						NetworkInterfaceID: "eni-xx",
					},
					types.NamespacedName{Namespace: "ns-2", Name: "node-1"}: {
						NetworkInterfaceID: "eni-xx",
					},
				},
			},
			want: []*corev1.Node{},
		},
		{
			name: "some nodes are resolved",
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "node-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "node-3",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-2",
							Name:      "node-1",
						},
					},
				},
				eniInfoByNodeKey: map[types.NamespacedName]ENIInfo{
					types.NamespacedName{Namespace: "ns-1", Name: "node-1"}: {
						NetworkInterfaceID: "eni-xx",
					},
					types.NamespacedName{Namespace: "ns-1", Name: "node-2"}: {
						NetworkInterfaceID: "eni-xx",
					},
					types.NamespacedName{Namespace: "ns-2", Name: "node-1"}: {
						NetworkInterfaceID: "eni-xx",
					},
				},
			},
			want: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "node-3",
					},
				},
			},
		},
		{
			name: "no nodes are resolved",
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "node-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "node-3",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-2",
							Name:      "node-1",
						},
					},
				},
				eniInfoByNodeKey: map[types.NamespacedName]ENIInfo{},
			},
			want: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "node-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "node-3",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-2",
						Name:      "node-1",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeNodesWithoutENIInfo(tt.args.nodes, tt.args.eniInfoByNodeKey)
			assert.Equal(t, tt.want, got)
		})
	}
}
