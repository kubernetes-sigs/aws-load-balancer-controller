package backend

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	mock_k8s "sigs.k8s.io/aws-load-balancer-controller/mocks/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	ctrl "sigs.k8s.io/controller-runtime"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultEndpointResolver_ResolvePodEndpoints(t *testing.T) {
	testNS := "test-ns"
	pod1 := k8s.PodInfo{
		Key:            types.NamespacedName{Namespace: testNS, Name: "pod-1"},
		UID:            "pod-uuid-1",
		ReadinessGates: nil,
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
			{
				Type:   corev1.ContainersReady,
				Status: corev1.ConditionTrue,
			},
		},
		PodIP: "192.168.1.1",
	}
	pod2 := k8s.PodInfo{
		Key:            types.NamespacedName{Namespace: testNS, Name: "pod-2"},
		UID:            "pod-uuid-2",
		ReadinessGates: nil,
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
			{
				Type:   corev1.ContainersReady,
				Status: corev1.ConditionTrue,
			},
		},
		PodIP: "192.168.1.2",
	}
	pod3 := k8s.PodInfo{
		Key: types.NamespacedName{Namespace: testNS, Name: "pod-3"},
		UID: "pod-uuid-3",
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			},
			{
				Type:   corev1.ContainersReady,
				Status: corev1.ConditionTrue,
			},
		},
		PodIP: "192.168.1.3",
	}
	pod4 := k8s.PodInfo{
		Key: types.NamespacedName{Namespace: testNS, Name: "pod-4"},
		UID: "pod-uuid-4",
		ReadinessGates: []corev1.PodReadinessGate{
			{
				ConditionType: "custom-condition",
			},
		},
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			},
			{
				Type:   corev1.ContainersReady,
				Status: corev1.ConditionTrue,
			},
		},
		PodIP: "192.168.1.4",
	}
	pod5 := k8s.PodInfo{
		Key: types.NamespacedName{Namespace: testNS, Name: "pod-5"},
		UID: "pod-uuid-5",
		ReadinessGates: []corev1.PodReadinessGate{
			{
				ConditionType: "custom-condition",
			},
		},
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			},
			{
				Type:   corev1.ContainersReady,
				Status: corev1.ConditionFalse,
			},
		},
		PodIP: "192.168.1.5",
	}
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
				{
					Name: "https",
					Port: 443,
				},
			},
		},
	}
	ep1A := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Ports: []corev1.EndpointPort{
					{
						Name: "http",
						Port: 8080,
					},
				},
				Addresses: []corev1.EndpointAddress{
					{
						IP: pod1.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod1.Key.Namespace,
							Name:      pod1.Key.Name,
						},
					},
					{
						IP: pod2.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod2.Key.Namespace,
							Name:      pod2.Key.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod3.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod3.Key.Namespace,
							Name:      pod3.Key.Name,
						},
					},
					{
						IP: pod4.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod4.Key.Namespace,
							Name:      pod4.Key.Name,
						},
					},
				},
			},
		},
	}
	ep1B := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Ports: []corev1.EndpointPort{
					{
						Name: "http",
						Port: 8080,
					},
				},
				Addresses: []corev1.EndpointAddress{
					{
						IP: pod1.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod1.Key.Namespace,
							Name:      pod1.Key.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod3.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod3.Key.Namespace,
							Name:      pod3.Key.Name,
						},
					},
				},
			},
			{
				Ports: []corev1.EndpointPort{
					{
						Name: "http",
						Port: 8080,
					},
				},
				Addresses: []corev1.EndpointAddress{
					{
						IP: pod2.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod2.Key.Namespace,
							Name:      pod2.Key.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod4.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod4.Key.Namespace,
							Name:      pod4.Key.Name,
						},
					},
				},
			},
		},
	}
	ep1C := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Ports: []corev1.EndpointPort{
					{
						Name: "http",
						Port: 8080,
					},
					{
						Name: "https",
						Port: 8080,
					},
				},
				Addresses: []corev1.EndpointAddress{
					{
						IP: pod1.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod1.Key.Namespace,
							Name:      pod1.Key.Name,
						},
					},
					{
						IP: pod2.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod2.Key.Namespace,
							Name:      pod2.Key.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod3.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod3.Key.Namespace,
							Name:      pod3.Key.Name,
						},
					},
					{
						IP: pod4.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod4.Key.Namespace,
							Name:      pod4.Key.Name,
						},
					},
				},
			},
		},
	}
	ep1D := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Ports: []corev1.EndpointPort{
					{
						Name: "http",
						Port: 8080,
					},
				},
				Addresses: []corev1.EndpointAddress{
					{
						IP: pod1.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod1.Key.Namespace,
							Name:      pod1.Key.Name,
						},
					},
					{
						IP: pod2.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod2.Key.Namespace,
							Name:      pod2.Key.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod3.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod3.Key.Namespace,
							Name:      pod3.Key.Name,
						},
					},
					{
						IP: pod4.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod4.Key.Namespace,
							Name:      pod4.Key.Name,
						},
					},
					{
						IP: pod5.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod5.Key.Namespace,
							Name:      pod5.Key.Name,
						},
					},
				},
			},
		},
	}

	type podInfoRepoGetCall struct {
		key    types.NamespacedName
		pod    k8s.PodInfo
		exists bool
		err    error
	}
	type env struct {
		services      []*corev1.Service
		endpointsList []*corev1.Endpoints
	}
	type fields struct {
		podInfoRepoGetCalls []podInfoRepoGetCall
	}
	type args struct {
		svcKey types.NamespacedName
		port   intstr.IntOrString
		opts   []EndpointResolveOption
	}
	tests := []struct {
		name                                string
		env                                 env
		fields                              fields
		args                                args
		want                                []PodEndpoint
		wantContainsPotentialReadyEndpoints bool
		wantErr                             error
	}{
		{
			name: "only ready pods will be included by default",
			env: env{
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1A},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{
					{
						key:    pod1.Key,
						pod:    pod1,
						exists: true,
					},
					{
						key:    pod2.Key,
						pod:    pod2,
						exists: true,
					},
				},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   nil,
			},
			want: []PodEndpoint{
				{
					IP:   "192.168.1.1",
					Port: 8080,
					Pod:  pod1,
				},
				{
					IP:   "192.168.1.2",
					Port: 8080,
					Pod:  pod2,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "unready only be included if it have readinessGate and containerReady",
			env: env{
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1A},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{
					{
						key:    pod1.Key,
						pod:    pod1,
						exists: true,
					},
					{
						key:    pod2.Key,
						pod:    pod2,
						exists: true,
					},
					{
						key:    pod3.Key,
						pod:    pod3,
						exists: true,
					},
					{
						key:    pod4.Key,
						pod:    pod4,
						exists: true,
					},
				},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithPodReadinessGate("custom-condition")},
			},
			want: []PodEndpoint{
				{
					IP:   "192.168.1.1",
					Port: 8080,
					Pod:  pod1,
				},
				{
					IP:   "192.168.1.2",
					Port: 8080,
					Pod:  pod2,
				},
				{
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "endpoints with multiple subsets should work as expected",
			env: env{
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1B},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{
					{
						key:    pod1.Key,
						pod:    pod1,
						exists: true,
					},
					{
						key:    pod2.Key,
						pod:    pod2,
						exists: true,
					},
					{
						key:    pod3.Key,
						pod:    pod3,
						exists: true,
					},
					{
						key:    pod4.Key,
						pod:    pod4,
						exists: true,
					},
				},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithPodReadinessGate("custom-condition")},
			},
			want: []PodEndpoint{
				{
					IP:   "192.168.1.1",
					Port: 8080,
					Pod:  pod1,
				},
				{
					IP:   "192.168.1.2",
					Port: 8080,
					Pod:  pod2,
				},
				{
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "endpoints with multiple ports should work as expected",
			env: env{
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1C},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{
					{
						key:    pod1.Key,
						pod:    pod1,
						exists: true,
					},
					{
						key:    pod2.Key,
						pod:    pod2,
						exists: true,
					},
					{
						key:    pod3.Key,
						pod:    pod3,
						exists: true,
					},
					{
						key:    pod4.Key,
						pod:    pod4,
						exists: true,
					},
				},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithPodReadinessGate("custom-condition")},
			},
			want: []PodEndpoint{
				{
					IP:   "192.168.1.1",
					Port: 8080,
					Pod:  pod1,
				},
				{
					IP:   "192.168.1.2",
					Port: 8080,
					Pod:  pod2,
				},
				{
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "unready but not found pod will be ignored, but signal potentialReadyEndpoints",
			env: env{
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1A},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{
					{
						key:    pod1.Key,
						pod:    pod1,
						exists: true,
					},
					{
						key:    pod2.Key,
						pod:    pod2,
						exists: true,
					},
					{
						key:    pod3.Key,
						exists: false,
					},
					{
						key:    pod4.Key,
						pod:    pod4,
						exists: true,
					},
				},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithPodReadinessGate("custom-condition")},
			},
			want: []PodEndpoint{
				{
					IP:   "192.168.1.1",
					Port: 8080,
					Pod:  pod1,
				},
				{
					IP:   "192.168.1.2",
					Port: 8080,
					Pod:  pod2,
				},
				{
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: true,
		},
		{
			name: "unready only be included if it have readinessGate and containerReady - not containerReady will signal containsPotentialReadyEndpoints",
			env: env{
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1D},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{
					{
						key:    pod1.Key,
						pod:    pod1,
						exists: true,
					},
					{
						key:    pod2.Key,
						pod:    pod2,
						exists: true,
					},
					{
						key:    pod3.Key,
						pod:    pod3,
						exists: true,
					},
					{
						key:    pod4.Key,
						pod:    pod4,
						exists: true,
					},
					{
						key:    pod5.Key,
						pod:    pod5,
						exists: true,
					},
				},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithPodReadinessGate("custom-condition")},
			},
			want: []PodEndpoint{
				{
					IP:   "192.168.1.1",
					Port: 8080,
					Pod:  pod1,
				},
				{
					IP:   "192.168.1.2",
					Port: 8080,
					Pod:  pod2,
				},
				{
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			podInfoRepo := mock_k8s.NewMockPodInfoRepo(ctrl)
			for _, call := range tt.fields.podInfoRepoGetCalls {
				podInfoRepo.EXPECT().Get(gomock.Any(), call.key).Return(call.pod, call.exists, call.err)
			}

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)

			ctx := context.Background()
			for _, svc := range tt.env.services {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}
			for _, endpoints := range tt.env.endpointsList {
				assert.NoError(t, k8sClient.Create(ctx, endpoints.DeepCopy()))
			}

			r := &defaultEndpointResolver{
				k8sClient:   k8sClient,
				podInfoRepo: podInfoRepo,
				logger:      &log.NullLogger{},
			}
			got, gotContainsPotentialReadyEndpoints, err := r.ResolvePodEndpoints(ctx, tt.args.svcKey, tt.args.port, tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opt := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
					cmpopts.SortSlices(func(lhs PodEndpoint, rhs PodEndpoint) bool {
						return lhs.IP < rhs.IP
					}),
				}
				assert.True(t, cmp.Equal(tt.want, got, opt),
					"diff: %v", cmp.Diff(tt.want, got, opt))
				assert.Equal(t, tt.wantContainsPotentialReadyEndpoints, gotContainsPotentialReadyEndpoints)
			}
		})
	}
}

func Test_defaultEndpointResolver_ResolveNodePortEndpoints(t *testing.T) {
	testNS := "test-ns"
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				"labelA": "valueA",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/i-abcdefg1",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-2",
			Labels: map[string]string{
				"labelA": "valueB",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/i-abcdefg2",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	node3 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-3",
			Labels: map[string]string{
				"labelA": "valueA",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/i-abcdefg3",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	node4 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-4",
			Labels: map[string]string{
				"labelA": "valueB",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/i-abcdefg4",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					NodePort: 18080,
				},
				{
					Name:     "https",
					Port:     443,
					NodePort: 18443,
				},
			},
		},
	}
	svc2 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-2",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					NodePort: 18080,
				},
				{
					Name:     "https",
					Port:     443,
					NodePort: 18443,
				},
			},
		},
	}

	type env struct {
		nodes    []*corev1.Node
		services []*corev1.Service
	}
	type args struct {
		svcKey types.NamespacedName
		port   intstr.IntOrString
		opts   []EndpointResolveOption
	}
	tests := []struct {
		name    string
		env     env
		args    args
		want    []NodePortEndpoint
		wantErr error
	}{
		{
			name: "no node will be chosen by default",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4},
				services: []*corev1.Service{svc1},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   nil,
			},
			want: nil,
		},
		{
			name: "choose every ready node",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4},
				services: []*corev1.Service{svc1},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithNodeSelector(labels.Everything())},
			},
			want: []NodePortEndpoint{
				{
					InstanceID: "i-abcdefg1",
					Port:       18080,
					Node:       node1,
				},
				{
					InstanceID: "i-abcdefg2",
					Port:       18080,
					Node:       node2,
				},
			},
		},
		{
			name: "choose every ready node that matches labelSelector",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4},
				services: []*corev1.Service{svc1},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithNodeSelector(labels.Set{"labelA": "valueA"}.AsSelectorPreValidated())},
			},
			want: []NodePortEndpoint{
				{
					InstanceID: "i-abcdefg1",
					Port:       18080,
					Node:       node1,
				},
			},
		},
		{
			name: "clusterIP service is not supported",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4},
				services: []*corev1.Service{svc2},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc2),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithNodeSelector(labels.Set{"labelA": "valueA"}.AsSelectorPreValidated())},
			},
			wantErr: errors.New("service type must be either 'NodePort' or 'LoadBalancer': test-ns/svc-2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, node := range tt.env.nodes {
				assert.NoError(t, k8sClient.Create(ctx, node.DeepCopy()))
			}
			for _, svc := range tt.env.services {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}

			r := &defaultEndpointResolver{
				k8sClient: k8sClient,
				logger:    ctrl.Log,
			}

			got, err := r.ResolveNodePortEndpoints(ctx, tt.args.svcKey, tt.args.port, tt.args.opts...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opt := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
					cmpopts.SortSlices(func(lhs NodePortEndpoint, rhs NodePortEndpoint) bool {
						return lhs.InstanceID < rhs.InstanceID
					}),
				}
				assert.True(t, cmp.Equal(tt.want, got, opt),
					"diff: %v", cmp.Diff(tt.want, got, opt))
			}
		})
	}
}
