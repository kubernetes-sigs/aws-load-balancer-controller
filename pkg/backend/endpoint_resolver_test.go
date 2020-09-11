package backend

import (
	"context"
	"errors"
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
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/equality"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	ctrl "sigs.k8s.io/controller-runtime"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func Test_defaultEndpointResolver_ResolvePodEndpoints(t *testing.T) {
	testNS := "test-ns"
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "pod-1",
		},
		Spec: corev1.PodSpec{
			ReadinessGates: nil,
		},
		Status: corev1.PodStatus{
			PodIP: "192.168.1.1",
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
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "pod-2",
		},
		Spec: corev1.PodSpec{
			ReadinessGates: nil,
		},
		Status: corev1.PodStatus{
			PodIP: "192.168.1.2",
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
		},
	}
	pod3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "pod-3",
		},
		Spec: corev1.PodSpec{
			ReadinessGates: nil,
		},
		Status: corev1.PodStatus{
			PodIP: "192.168.1.3",
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
		},
	}
	pod4 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "pod-4",
		},
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{
				{
					ConditionType: "custom-condition",
				},
			},
		},
		Status: corev1.PodStatus{
			PodIP: "192.168.1.4",
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
		},
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
						IP: pod1.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod1.Namespace,
							Name:      pod1.Name,
						},
					},
					{
						IP: pod2.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod2.Namespace,
							Name:      pod2.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod3.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod3.Namespace,
							Name:      pod3.Name,
						},
					},
					{
						IP: pod4.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod4.Namespace,
							Name:      pod4.Name,
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
						IP: pod1.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod1.Namespace,
							Name:      pod1.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod3.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod3.Namespace,
							Name:      pod3.Name,
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
						IP: pod2.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod2.Namespace,
							Name:      pod2.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod4.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod4.Namespace,
							Name:      pod4.Name,
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
						IP: pod1.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod1.Namespace,
							Name:      pod1.Name,
						},
					},
					{
						IP: pod2.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod2.Namespace,
							Name:      pod2.Name,
						},
					},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{
						IP: pod3.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod3.Namespace,
							Name:      pod3.Name,
						},
					},
					{
						IP: pod4.Status.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod4.Namespace,
							Name:      pod4.Name,
						},
					},
				},
			},
		},
	}

	type env struct {
		pods          []*corev1.Pod
		services      []*corev1.Service
		endpointsList []*corev1.Endpoints
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
		want    []PodEndpoint
		wantErr error
	}{
		{
			name: "only ready pods will be included by default",
			env: env{
				pods:          []*corev1.Pod{pod1, pod2, pod3, pod4},
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1A},
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
		},
		{
			name: "unready but containerReady pod will also be included",
			env: env{
				pods:          []*corev1.Pod{pod1, pod2, pod3, pod4},
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1A},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithUnreadyPodInclusionCriterion(k8s.IsPodContainersReady)},
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
					IP:   "192.168.1.3",
					Port: 8080,
					Pod:  pod3,
				},
				{
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
		},
		{
			name: "unready but containerReady pod will also be included when contains specific readinessGate",
			env: env{
				pods:          []*corev1.Pod{pod1, pod2, pod3, pod4},
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1A},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts: []EndpointResolveOption{
					WithUnreadyPodInclusionCriterion(k8s.IsPodContainersReady),
					WithUnreadyPodInclusionCriterion(func(pod *corev1.Pod) bool {
						return k8s.IsPodHasReadinessGate(pod, "custom-condition")
					}),
				},
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
		},
		{
			name: "endpoints with multiple subsets should work as expected",
			env: env{
				pods:          []*corev1.Pod{pod1, pod2, pod3, pod4},
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1B},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithUnreadyPodInclusionCriterion(k8s.IsPodContainersReady)},
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
					IP:   "192.168.1.3",
					Port: 8080,
					Pod:  pod3,
				},
				{
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
		},
		{
			name: "endpoints with multiple ports should work as expected",
			env: env{
				pods:          []*corev1.Pod{pod1, pod2, pod3, pod4},
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1C},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithUnreadyPodInclusionCriterion(k8s.IsPodContainersReady)},
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
					IP:   "192.168.1.3",
					Port: 8080,
					Pod:  pod3,
				},
				{
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, pod := range tt.env.pods {
				assert.NoError(t, k8sClient.Create(ctx, pod.DeepCopy()))
			}
			for _, svc := range tt.env.services {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}
			for _, endpoints := range tt.env.endpointsList {
				assert.NoError(t, k8sClient.Create(ctx, endpoints.DeepCopy()))
			}

			r := &defaultEndpointResolver{
				k8sClient: k8sClient,
				logger:    ctrl.Log,
			}
			got, err := r.ResolvePodEndpoints(ctx, tt.args.svcKey, tt.args.port, tt.args.opts...)
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
	_ = svc2

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

func Test_isPodMeetCriteria(t *testing.T) {
	type args struct {
		pod      *corev1.Pod
		criteria []PodPredicate
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "pod is containerReady and with no readinessGate - meet criteria",
			args: args{
				pod: &corev1.Pod{
					Spec: corev1.PodSpec{},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				criteria: []PodPredicate{k8s.IsPodContainersReady},
			},
			want: true,
		},
		{
			name: "pod is containerReady and with no readinessGate - doesn't meet criteria",
			args: args{
				pod: &corev1.Pod{
					Spec: corev1.PodSpec{},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				criteria: []PodPredicate{k8s.IsPodContainersReady, func(pod *corev1.Pod) bool {
					return k8s.IsPodHasReadinessGate(pod, "alb.ingress.k8s.aws/condition")
				}},
			},
			want: false,
		},
		{
			name: "pod is containerReady and with readinessGate - meet criteria",
			args: args{
				pod: &corev1.Pod{
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "alb.ingress.k8s.aws/condition",
							},
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				criteria: []PodPredicate{k8s.IsPodContainersReady, func(pod *corev1.Pod) bool {
					return k8s.IsPodHasReadinessGate(pod, "alb.ingress.k8s.aws/condition")
				}},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPodMeetCriteria(tt.args.pod, tt.args.criteria)
			assert.Equal(t, tt.want, got)
		})
	}
}
