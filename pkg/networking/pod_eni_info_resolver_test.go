package networking

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"testing"
)

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
