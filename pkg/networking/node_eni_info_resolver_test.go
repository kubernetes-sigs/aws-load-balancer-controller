package networking

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

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
