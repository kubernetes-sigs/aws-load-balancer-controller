package backend

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	ctrl "sigs.k8s.io/controller-runtime"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1beta1"
)

func Test_defaultEndpointResolver_ResolvePodEndpoints(t *testing.T) {
	testNS := "test-ns"
	nodeA := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/i-abcdefga",
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
	nodeB := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-b",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/i-abcdefgb",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionUnknown,
				},
			},
		},
	}
	nodeC := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-c",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/i-abcdefgc",
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
	pod1 := k8s.PodInfo{ // pod ready on ready node
		Key: types.NamespacedName{Namespace: testNS, Name: "pod-1"},
		UID: "pod-uuid-1",
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
		NodeName: "node-a",
		PodIP:    "192.168.1.1",
	}
	pod2 := k8s.PodInfo{ // pod containerReady on unknown node
		Key: types.NamespacedName{Namespace: testNS, Name: "pod-2"},
		UID: "pod-uuid-2",
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
		NodeName: "node-b",
		PodIP:    "192.168.1.2",
	}

	pod3 := k8s.PodInfo{ // pod containerReady on not ready node
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
		NodeName: "node-c",
		PodIP:    "192.168.1.3",
	}

	pod4 := k8s.PodInfo{ // pod containerReady(with readinessGate) on ready node
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
		NodeName: "node-a",
		PodIP:    "192.168.1.4",
	}

	pod5 := k8s.PodInfo{ // pod containerReady(with readinessGate) on unknown node
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
				Status: corev1.ConditionTrue,
			},
		},
		NodeName: "node-b",
		PodIP:    "192.168.1.5",
	}

	pod6 := k8s.PodInfo{ // pod not containerReady(with readinessGate) on ready node
		Key: types.NamespacedName{Namespace: testNS, Name: "pod-6"},
		UID: "pod-uuid-6",
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
		NodeName: "node-a",
		PodIP:    "192.168.1.6",
	}

	pod7 := k8s.PodInfo{ // pod not containerReady(without readinessGate) on ready node
		Key: types.NamespacedName{Namespace: testNS, Name: "pod-7"},
		UID: "pod-uuid-7",
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
		NodeName: "node-a",
		PodIP:    "192.168.1.7",
	}
	pod8 := k8s.PodInfo{ // pod containerReady but terminating on ready node
		Key: types.NamespacedName{Namespace: testNS, Name: "pod-8"},
		UID: "pod-uuid-8",
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
		NodeName: "node-b",
		PodIP:    "192.168.1.8",
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

	svc1WithoutHTTPPort := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name: "https",
					Port: 443,
				},
			},
		},
	}
	ep1 := &corev1.Endpoints{
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
						Port: 8443,
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
						IP: pod2.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod2.Key.Namespace,
							Name:      pod2.Key.Name,
						},
					},
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
					{
						IP: pod6.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod6.Key.Namespace,
							Name:      pod6.Key.Name,
						},
					},
					{
						IP: pod7.PodIP,
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: pod7.Key.Namespace,
							Name:      pod7.Key.Name,
						},
					},
				},
			},
		},
	}
	eps1 := &discovery.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1-a",
			Labels: map[string]string{
				"kubernetes.io/service-name": "svc-1",
			},
		},
		Ports: []discovery.EndpointPort{
			{
				Name: awssdk.String("http"),
				Port: awssdk.Int32(8080),
			},
			{
				Name: awssdk.String("https"),
				Port: awssdk.Int32(8443),
			},
		},
		Endpoints: []discovery.Endpoint{
			{
				Addresses: []string{pod1.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod1.Key.Namespace,
					Name:      pod1.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(true),
					Serving:     awssdk.Bool(true),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod2.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod2.Key.Namespace,
					Name:      pod2.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod3.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod3.Key.Namespace,
					Name:      pod3.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod4.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod4.Key.Namespace,
					Name:      pod4.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod5.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod5.Key.Namespace,
					Name:      pod5.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod6.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod6.Key.Namespace,
					Name:      pod6.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod7.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod7.Key.Namespace,
					Name:      pod7.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod8.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod8.Key.Namespace,
					Name:      pod8.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(true),
					Terminating: awssdk.Bool(true),
				},
			},
		},
	}

	eps2 := &discovery.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1-a",
			Labels: map[string]string{
				"kubernetes.io/service-name": "svc-1",
			},
		},
		Ports: []discovery.EndpointPort{
			{
				Name: awssdk.String("http"),
				Port: awssdk.Int32(8080),
			},
			{
				Name: awssdk.String("https"),
				Port: awssdk.Int32(8443),
			},
		},
		Endpoints: []discovery.Endpoint{
			{
				Addresses: []string{pod2.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod2.Key.Namespace,
					Name:      pod2.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod3.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod3.Key.Namespace,
					Name:      pod3.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod5.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod5.Key.Namespace,
					Name:      pod5.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod6.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod6.Key.Namespace,
					Name:      pod6.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod7.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod7.Key.Namespace,
					Name:      pod7.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod8.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod8.Key.Namespace,
					Name:      pod8.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(true),
					Terminating: awssdk.Bool(true),
				},
			},
		},
	}

	eps3 := &discovery.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1-a",
			Labels: map[string]string{
				"kubernetes.io/service-name": "svc-1",
			},
		},
		Ports: []discovery.EndpointPort{
			{
				Name: awssdk.String("http"),
				Port: awssdk.Int32(8080),
			},
			{
				Name: awssdk.String("https"),
				Port: awssdk.Int32(8443),
			},
		},
		Endpoints: []discovery.Endpoint{
			{
				Addresses: []string{pod1.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod1.Key.Namespace,
					Name:      pod1.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(true),
					Serving:     awssdk.Bool(true),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod2.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod2.Key.Namespace,
					Name:      pod2.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod3.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod3.Key.Namespace,
					Name:      pod3.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod4.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod4.Key.Namespace,
					Name:      pod4.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod5.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod5.Key.Namespace,
					Name:      pod5.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod7.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod7.Key.Namespace,
					Name:      pod7.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
			},
			{
				Addresses: []string{pod8.PodIP},
				TargetRef: &corev1.ObjectReference{
					Kind:      "Pod",
					Namespace: pod8.Key.Namespace,
					Name:      pod8.Key.Name,
				},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(true),
					Terminating: awssdk.Bool(true),
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
		nodes          []*corev1.Node
		services       []*corev1.Service
		endpointsList  []*corev1.Endpoints
		endpointSlices []*discovery.EndpointSlice
	}
	type fields struct {
		podInfoRepoGetCalls  []podInfoRepoGetCall
		failOpenEnabled      bool
		endpointSliceEnabled bool
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
			name: "[with endpoints][with failOpen] choose every ready pod only when there are ready pods",
			env: env{
				nodes:         []*corev1.Node{nodeA, nodeB, nodeC},
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1},
			},
			fields: fields{
				failOpenEnabled:      true,
				endpointSliceEnabled: false,
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
					{
						key:    pod6.Key,
						pod:    pod6,
						exists: true,
					},
					{
						key:    pod7.Key,
						pod:    pod7,
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
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "[with endpointSlices][with failOpen] choose every ready pod only when there are ready pods",
			env: env{
				nodes:          []*corev1.Node{nodeA, nodeB, nodeC},
				services:       []*corev1.Service{svc1},
				endpointSlices: []*discovery.EndpointSlice{eps1},
			},
			fields: fields{
				failOpenEnabled:      true,
				endpointSliceEnabled: true,
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
					{
						key:    pod6.Key,
						pod:    pod6,
						exists: true,
					},
					{
						key:    pod7.Key,
						pod:    pod7,
						exists: true,
					},
					{
						key:    pod8.Key,
						pod:    pod8,
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
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "[with endpointSlices][without failOpen] choose every ready pod only when there are ready pods",
			env: env{
				nodes:          []*corev1.Node{nodeA, nodeB, nodeC},
				services:       []*corev1.Service{svc1},
				endpointSlices: []*discovery.EndpointSlice{eps1},
			},
			fields: fields{
				failOpenEnabled:      false,
				endpointSliceEnabled: true,
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
					{
						key:    pod6.Key,
						pod:    pod6,
						exists: true,
					},
					{
						key:    pod7.Key,
						pod:    pod7,
						exists: true,
					},
					{
						key:    pod8.Key,
						pod:    pod8,
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
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "[with endpointSlices][with failOpen] choose every unknown pod when there are no ready pods",
			env: env{
				nodes:          []*corev1.Node{nodeA, nodeB, nodeC},
				services:       []*corev1.Service{svc1},
				endpointSlices: []*discovery.EndpointSlice{eps2},
			},
			fields: fields{
				failOpenEnabled:      true,
				endpointSliceEnabled: true,
				podInfoRepoGetCalls: []podInfoRepoGetCall{
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
						key:    pod5.Key,
						pod:    pod5,
						exists: true,
					},
					{
						key:    pod6.Key,
						pod:    pod6,
						exists: true,
					},
					{
						key:    pod7.Key,
						pod:    pod7,
						exists: true,
					},
					{
						key:    pod8.Key,
						pod:    pod8,
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
					IP:   "192.168.1.2",
					Port: 8080,
					Pod:  pod2,
				},
				{
					IP:   "192.168.1.5",
					Port: 8080,
					Pod:  pod5,
				},
				{
					IP:   "192.168.1.8",
					Port: 8080,
					Pod:  pod8,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "[with endpointSlices][without failOpen] don't choose unknown pod when there are no ready pods",
			env: env{
				nodes:          []*corev1.Node{nodeA, nodeB, nodeC},
				services:       []*corev1.Service{svc1},
				endpointSlices: []*discovery.EndpointSlice{eps2},
			},
			fields: fields{
				failOpenEnabled:      false,
				endpointSliceEnabled: true,
				podInfoRepoGetCalls: []podInfoRepoGetCall{
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
						key:    pod5.Key,
						pod:    pod5,
						exists: true,
					},
					{
						key:    pod6.Key,
						pod:    pod6,
						exists: true,
					},
					{
						key:    pod7.Key,
						pod:    pod7,
						exists: true,
					},
					{
						key:    pod8.Key,
						pod:    pod8,
						exists: true,
					},
				},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   nil,
			},
			want:                                nil,
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "[with endpointSlices][with failOpen] choose every ready pod only when there are ready pods - some pod have readinessGate",
			env: env{
				nodes:          []*corev1.Node{nodeA, nodeB, nodeC},
				services:       []*corev1.Service{svc1},
				endpointSlices: []*discovery.EndpointSlice{eps1},
			},
			fields: fields{
				failOpenEnabled:      true,
				endpointSliceEnabled: true,
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
					{
						key:    pod6.Key,
						pod:    pod6,
						exists: true,
					},
					{
						key:    pod7.Key,
						pod:    pod7,
						exists: true,
					},
					{
						key:    pod8.Key,
						pod:    pod8,
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
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: true,
		},
		{
			name: "[with endpointSlices][with failOpen] choose every ready pod only when there are ready pods - no pod have readinessGate",
			env: env{
				nodes:          []*corev1.Node{nodeA, nodeB, nodeC},
				services:       []*corev1.Service{svc1},
				endpointSlices: []*discovery.EndpointSlice{eps3},
			},
			fields: fields{
				failOpenEnabled:      true,
				endpointSliceEnabled: true,
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
					{
						key:    pod7.Key,
						pod:    pod7,
						exists: true,
					},
					{
						key:    pod8.Key,
						pod:    pod8,
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
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "[with endpoints][with failOpen] choose every ready pod only when there are ready pods - ignore pods don't exists",
			env: env{
				nodes:         []*corev1.Node{nodeA, nodeB, nodeC},
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{ep1},
			},
			fields: fields{
				failOpenEnabled:      true,
				endpointSliceEnabled: false,
				podInfoRepoGetCalls: []podInfoRepoGetCall{
					{
						key:    pod1.Key,
						pod:    k8s.PodInfo{},
						exists: false,
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
					{
						key:    pod6.Key,
						pod:    pod6,
						exists: true,
					},
					{
						key:    pod7.Key,
						pod:    pod7,
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
					IP:   "192.168.1.4",
					Port: 8080,
					Pod:  pod4,
				},
			},
			wantContainsPotentialReadyEndpoints: false,
		},
		{
			name: "service not found",
			env: env{
				services:      []*corev1.Service{},
				endpointsList: []*corev1.Endpoints{},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   nil,
			},
			want:                                []PodEndpoint{},
			wantContainsPotentialReadyEndpoints: false,
			wantErr:                             fmt.Errorf("%w: %v", ErrNotFound, "services \"svc-1\" not found"),
		},
		{
			name: "service port not found",
			env: env{
				services:      []*corev1.Service{svc1WithoutHTTPPort},
				endpointsList: []*corev1.Endpoints{},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   nil,
			},
			wantErr: fmt.Errorf("%w: %v", ErrNotFound, "unable to find port http on service test-ns/svc-1"),
		},
		{
			name: "endpoints not found",
			env: env{
				services:      []*corev1.Service{svc1},
				endpointsList: []*corev1.Endpoints{},
			},
			fields: fields{
				podInfoRepoGetCalls: []podInfoRepoGetCall{},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   nil,
			},
			wantErr: fmt.Errorf("%w: %v", ErrNotFound, "endpoints \"svc-1\" not found"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			podInfoRepo := k8s.NewMockPodInfoRepo(ctrl)
			for _, call := range tt.fields.podInfoRepoGetCalls {
				podInfoRepo.EXPECT().Get(gomock.Any(), call.key).Return(call.pod, call.exists, call.err)
			}

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			ctx := context.Background()
			for _, node := range tt.env.nodes {
				assert.NoError(t, k8sClient.Create(ctx, node.DeepCopy()))
			}
			for _, svc := range tt.env.services {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}
			for _, endpoints := range tt.env.endpointsList {
				assert.NoError(t, k8sClient.Create(ctx, endpoints.DeepCopy()))
			}
			for _, eps := range tt.env.endpointSlices {
				assert.NoError(t, k8sClient.Create(ctx, eps.DeepCopy()))
			}

			r := &defaultEndpointResolver{
				k8sClient:            k8sClient,
				podInfoRepo:          podInfoRepo,
				failOpenEnabled:      tt.fields.failOpenEnabled,
				endpointSliceEnabled: tt.fields.endpointSliceEnabled,
				logger:               &log.NullLogger{},
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
					Status: corev1.ConditionUnknown,
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
					Status: corev1.ConditionUnknown,
				},
			},
		},
	}
	node5 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-5",
			Labels: map[string]string{
				"labelA": "valueA",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "aws:///us-west-2b/i-abcdefg5",
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
	svc1WithoutHTTPPort := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      "svc-1",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name: "https",
					Port: 443,
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

	type fields struct {
		failOpenEnabled bool
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
		fields  fields
		args    args
		want    []NodePortEndpoint
		wantErr error
	}{
		{
			name: "[with failOpen] choose every ready node only when there are ready nodes",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4, node5},
				services: []*corev1.Service{svc1},
			},
			fields: fields{
				failOpenEnabled: true,
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
			name: "[without failOpen] choose every ready node only when there are ready nodes",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4, node5},
				services: []*corev1.Service{svc1},
			},
			fields: fields{
				failOpenEnabled: false,
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
			name: "[with failOpen] choose every unknown node when there are no ready nodes",
			env: env{
				nodes:    []*corev1.Node{node3, node4, node5},
				services: []*corev1.Service{svc1},
			},
			fields: fields{
				failOpenEnabled: true,
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithNodeSelector(labels.Everything())},
			},
			want: []NodePortEndpoint{
				{
					InstanceID: "i-abcdefg3",
					Port:       18080,
					Node:       node3,
				},
				{
					InstanceID: "i-abcdefg4",
					Port:       18080,
					Node:       node4,
				},
			},
		},
		{
			name: "[without failOpen] don't choose unknown node when there are no ready nodes",
			env: env{
				nodes:    []*corev1.Node{node3, node4, node5},
				services: []*corev1.Service{svc1},
			},
			fields: fields{
				failOpenEnabled: false,
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithNodeSelector(labels.Everything())},
			},
			want: nil,
		},
		{
			name: "choose every ready node - matches labelSelector",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4, node5},
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
			name: "no node will be chosen by default",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4, node5},
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
		{
			name: "service not found",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4},
				services: []*corev1.Service{},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithNodeSelector(labels.Everything())},
			},
			wantErr: fmt.Errorf("%w: %v", ErrNotFound, "services \"svc-1\" not found"),
		},
		{
			name: "service port not found",
			env: env{
				nodes:    []*corev1.Node{node1, node2, node3, node4},
				services: []*corev1.Service{svc1WithoutHTTPPort},
			},
			args: args{
				svcKey: k8s.NamespacedName(svc1),
				port:   intstr.FromString("http"),
				opts:   []EndpointResolveOption{WithNodeSelector(labels.Everything())},
			},
			wantErr: fmt.Errorf("%w: %v", ErrNotFound, "unable to find port http on service test-ns/svc-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			for _, node := range tt.env.nodes {
				assert.NoError(t, k8sClient.Create(ctx, node.DeepCopy()))
			}
			for _, svc := range tt.env.services {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}

			r := &defaultEndpointResolver{
				k8sClient:       k8sClient,
				failOpenEnabled: tt.fields.failOpenEnabled,
				logger:          ctrl.Log,
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

func Test_defaultEndpointResolver_computeServiceEndpointsData(t *testing.T) {
	type env struct {
		endpoints      []*corev1.Endpoints
		endpointSlices []*discovery.EndpointSlice
	}
	type fields struct {
		endpointSliceEnabled bool
	}
	type args struct {
		svcKey types.NamespacedName
	}
	tests := []struct {
		name   string
		env    env
		fields fields

		args    args
		want    []EndpointsData
		wantErr error
	}{
		{
			name: "build endpoints from endpoints",
			fields: fields{
				endpointSliceEnabled: false,
			},
			env: env{
				endpoints: []*corev1.Endpoints{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "sample-ns",
							Name:      "sample-svc",
						},
						Subsets: []corev1.EndpointSubset{
							{
								Ports: []corev1.EndpointPort{
									{
										Name: "http",
										Port: 80,
									},
								},
								Addresses: []corev1.EndpointAddress{
									{
										IP: "192.168.1.1",
									},
								},
							},
							{
								Ports: []corev1.EndpointPort{
									{
										Name: "https",
										Port: 443,
									},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{
										IP: "192.168.1.1",
									},
								},
							},
						},
					},
				},
			},
			args: args{
				svcKey: types.NamespacedName{Namespace: "sample-ns", Name: "sample-svc"},
			},
			want: []EndpointsData{
				{
					Ports: []discovery.EndpointPort{
						{
							Name: awssdk.String("http"),
							Port: awssdk.Int32(80),
						},
					},
					Endpoints: []discovery.Endpoint{
						{
							Addresses: []string{"192.168.1.1"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(true),
								Serving:     awssdk.Bool(true),
								Terminating: awssdk.Bool(false),
							},
						},
					},
				},
				{
					Ports: []discovery.EndpointPort{
						{
							Name: awssdk.String("https"),
							Port: awssdk.Int32(443),
						},
					},
					Endpoints: []discovery.Endpoint{
						{
							Addresses: []string{"192.168.1.1"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(false),
								Serving:     awssdk.Bool(false),
								Terminating: awssdk.Bool(false),
							},
						},
					},
				},
			},
		},
		{
			name: "build endpoints from endpointSlices",
			fields: fields{
				endpointSliceEnabled: true,
			},
			env: env{
				endpointSlices: []*discovery.EndpointSlice{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "sample-ns",
							Name:      "sample-svc-1",
							Labels: map[string]string{
								"kubernetes.io/service-name": "sample-svc",
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: awssdk.String("http"),
								Port: awssdk.Int32(80),
							},
						},
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"192.168.1.1"},
								Conditions: discovery.EndpointConditions{
									Ready:       awssdk.Bool(true),
									Serving:     awssdk.Bool(true),
									Terminating: awssdk.Bool(false),
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "sample-ns",
							Name:      "sample-svc-2",
							Labels: map[string]string{
								"kubernetes.io/service-name": "sample-svc",
							},
						},
						Ports: []discovery.EndpointPort{
							{
								Name: awssdk.String("https"),
								Port: awssdk.Int32(443),
							},
						},
						Endpoints: []discovery.Endpoint{
							{
								Addresses: []string{"192.168.1.1"},
								Conditions: discovery.EndpointConditions{
									Ready:       awssdk.Bool(false),
									Serving:     awssdk.Bool(false),
									Terminating: awssdk.Bool(false),
								},
							},
						},
					},
				},
			},
			args: args{
				svcKey: types.NamespacedName{Namespace: "sample-ns", Name: "sample-svc"},
			},
			want: []EndpointsData{
				{
					Ports: []discovery.EndpointPort{
						{
							Name: awssdk.String("http"),
							Port: awssdk.Int32(80),
						},
					},
					Endpoints: []discovery.Endpoint{
						{
							Addresses: []string{"192.168.1.1"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(true),
								Serving:     awssdk.Bool(true),
								Terminating: awssdk.Bool(false),
							},
						},
					},
				},
				{
					Ports: []discovery.EndpointPort{
						{
							Name: awssdk.String("https"),
							Port: awssdk.Int32(443),
						},
					},
					Endpoints: []discovery.Endpoint{
						{
							Addresses: []string{"192.168.1.1"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(false),
								Serving:     awssdk.Bool(false),
								Terminating: awssdk.Bool(false),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			ctx := context.Background()
			for _, ep := range tt.env.endpoints {
				assert.NoError(t, k8sClient.Create(ctx, ep.DeepCopy()))
			}
			for _, eps := range tt.env.endpointSlices {
				assert.NoError(t, k8sClient.Create(ctx, eps.DeepCopy()))
			}

			r := &defaultEndpointResolver{
				k8sClient:            k8sClient,
				endpointSliceEnabled: tt.fields.endpointSliceEnabled,
			}
			got, err := r.computeServiceEndpointsData(context.Background(), tt.args.svcKey)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultEndpointResolver_findServiceAndServicePort(t *testing.T) {
	type env struct {
		services []*corev1.Service
	}
	type args struct {
		svcKey types.NamespacedName
		port   intstr.IntOrString
	}
	tests := []struct {
		name        string
		env         env
		args        args
		wantSvc     *corev1.Service
		wantSvcPort corev1.ServicePort
		wantErr     error
	}{
		{
			name: "found service and servicePort",
			env: env{
				services: []*corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "sample-ns",
							Name:      "sample-svc",
						},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{
								{
									Name: "http",
									Port: 80,
								},
							},
						},
					},
				},
			},
			args: args{
				svcKey: types.NamespacedName{Namespace: "sample-ns", Name: "sample-svc"},
				port:   intstr.FromString("http"),
			},
			wantSvc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "sample-ns",
					Name:      "sample-svc",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "http",
							Port: 80,
						},
					},
				},
			},
			wantSvcPort: corev1.ServicePort{
				Name: "http",
				Port: 80,
			},
		},
		{
			name: "service not found",
			env: env{
				services: []*corev1.Service{},
			},
			args: args{
				svcKey: types.NamespacedName{Namespace: "sample-ns", Name: "sample-svc"},
				port:   intstr.FromString("http"),
			},
			wantErr: errors.New("backend not found: services \"sample-svc\" not found"),
		},
		{
			name: "servicePort not found",
			env: env{
				services: []*corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "sample-ns",
							Name:      "sample-svc",
						},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{
								{
									Name: "https",
									Port: 443,
								},
							},
						},
					},
				},
			},
			args: args{
				svcKey: types.NamespacedName{Namespace: "sample-ns", Name: "sample-svc"},
				port:   intstr.FromString("http"),
			},
			wantErr: errors.New("backend not found: unable to find port http on service sample-ns/sample-svc"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
			ctx := context.Background()
			for _, svc := range tt.env.services {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}

			r := &defaultEndpointResolver{
				k8sClient: k8sClient,
			}
			gotSvc, gotSvcPort, err := r.findServiceAndServicePort(ctx, tt.args.svcKey, tt.args.port)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				opt := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
					cmpopts.SortSlices(func(lhs PodEndpoint, rhs PodEndpoint) bool {
						return lhs.IP < rhs.IP
					}),
				}
				assert.NoError(t, err)
				assert.True(t, cmp.Equal(tt.wantSvc, gotSvc, opt),
					"diff: %v", cmp.Diff(tt.wantSvc, gotSvc, opt))
				assert.Equal(t, tt.wantSvcPort, gotSvcPort)
			}
		})
	}
}

func Test_filterNodesByReadyConditionStatus(t *testing.T) {
	type args struct {
		nodes           []*corev1.Node
		readyCondStatus corev1.ConditionStatus
	}
	tests := []struct {
		name string
		args args
		want []*corev1.Node
	}{
		{
			name: "filter ready:true nodes - multiple found",
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2b/i-xxxxxa",
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2b/i-xxxxxb",
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2b/i-xxxxxc",
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionUnknown,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-4",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2b/i-xxxxxd",
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionFalse,
								},
							},
						},
					},
				},
				readyCondStatus: corev1.ConditionTrue,
			},
			want: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-xxxxxa",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-xxxxxb",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			name: "filter ready:unknown nodes - one found",
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2b/i-xxxxxc",
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionUnknown,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-4",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2b/i-xxxxxd",
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionFalse,
								},
							},
						},
					},
				},
				readyCondStatus: corev1.ConditionUnknown,
			},
			want: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-3",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-xxxxxc",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionUnknown,
							},
						},
					},
				},
			},
		},
		{
			name: "filter ready:true nodes - none found",
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2b/i-xxxxxc",
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionUnknown,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-4",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2b/i-xxxxxd",
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionFalse,
								},
							},
						},
					},
				},
				readyCondStatus: corev1.ConditionTrue,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterNodesByReadyConditionStatus(tt.args.nodes, tt.args.readyCondStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildEndpointsDataFromEndpoints(t *testing.T) {
	type args struct {
		eps *corev1.Endpoints
	}
	tests := []struct {
		name string
		args args
		want []EndpointsData
	}{
		{
			name: "multiple endpoints",
			args: args{
				eps: &corev1.Endpoints{
					Subsets: []corev1.EndpointSubset{
						{
							Ports: []corev1.EndpointPort{
								{
									Name: "http",
									Port: 80,
								},
								{
									Name: "https",
									Port: 443,
								},
							},
							Addresses: []corev1.EndpointAddress{
								{
									IP: "192.168.1.1",
								},
								{
									IP: "192.168.1.2",
								},
							},
							NotReadyAddresses: []corev1.EndpointAddress{
								{
									IP: "192.168.1.3",
								},
							},
						},
						{
							Ports: []corev1.EndpointPort{
								{
									Name: "http",
									Port: 8080,
								},
								{
									Name: "https",
									Port: 8443,
								},
							},
							Addresses: []corev1.EndpointAddress{
								{
									IP: "192.168.3.1",
								},
								{
									IP: "192.168.3.2",
								},
							},
							NotReadyAddresses: []corev1.EndpointAddress{
								{
									IP: "192.168.3.3",
								},
							},
						},
					},
				},
			},
			want: []EndpointsData{
				{
					Ports: []discovery.EndpointPort{
						{
							Name: awssdk.String("http"),
							Port: awssdk.Int32(80),
						},
						{
							Name: awssdk.String("https"),
							Port: awssdk.Int32(443),
						},
					},
					Endpoints: []discovery.Endpoint{
						{
							Addresses: []string{"192.168.1.1"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(true),
								Serving:     awssdk.Bool(true),
								Terminating: awssdk.Bool(false),
							},
						},
						{
							Addresses: []string{"192.168.1.2"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(true),
								Serving:     awssdk.Bool(true),
								Terminating: awssdk.Bool(false),
							},
						},
						{
							Addresses: []string{"192.168.1.3"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(false),
								Serving:     awssdk.Bool(false),
								Terminating: awssdk.Bool(false),
							},
						},
					},
				},
				{
					Ports: []discovery.EndpointPort{
						{
							Name: awssdk.String("http"),
							Port: awssdk.Int32(8080),
						},
						{
							Name: awssdk.String("https"),
							Port: awssdk.Int32(8443),
						},
					},
					Endpoints: []discovery.Endpoint{
						{
							Addresses: []string{"192.168.3.1"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(true),
								Serving:     awssdk.Bool(true),
								Terminating: awssdk.Bool(false),
							},
						},
						{
							Addresses: []string{"192.168.3.2"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(true),
								Serving:     awssdk.Bool(true),
								Terminating: awssdk.Bool(false),
							},
						},
						{
							Addresses: []string{"192.168.3.3"},
							Conditions: discovery.EndpointConditions{
								Ready:       awssdk.Bool(false),
								Serving:     awssdk.Bool(false),
								Terminating: awssdk.Bool(false),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildEndpointsDataFromEndpoints(tt.args.eps)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildEndpointsDataFromEndpointSliceList(t *testing.T) {
	type args struct {
		epsList *discovery.EndpointSliceList
	}
	tests := []struct {
		name string
		args args
		want []EndpointsData
	}{
		{
			name: "multiple endpointSlices",
			args: args{
				epsList: &discovery.EndpointSliceList{
					Items: []discovery.EndpointSlice{
						{
							Ports: []discovery.EndpointPort{
								{
									Name: awssdk.String("http"),
									Port: awssdk.Int32(80),
								},
								{
									Name: awssdk.String("https"),
									Port: awssdk.Int32(443),
								},
							},
							Endpoints: []discovery.Endpoint{
								{
									Addresses: []string{"192.168.1.1"},
								},
								{
									Addresses: []string{"192.168.1.2"},
								},
							},
						},
						{
							Ports: []discovery.EndpointPort{
								{
									Name: awssdk.String("http"),
									Port: awssdk.Int32(8080),
								},
								{
									Name: awssdk.String("https"),
									Port: awssdk.Int32(8443),
								},
							},
							Endpoints: []discovery.Endpoint{
								{
									Addresses: []string{"192.168.3.1"},
								},
								{
									Addresses: []string{"192.168.3.2"},
								},
							},
						},
					},
				},
			},
			want: []EndpointsData{
				{
					Ports: []discovery.EndpointPort{
						{
							Name: awssdk.String("http"),
							Port: awssdk.Int32(80),
						},
						{
							Name: awssdk.String("https"),
							Port: awssdk.Int32(443),
						},
					},
					Endpoints: []discovery.Endpoint{
						{
							Addresses: []string{"192.168.1.1"},
						},
						{
							Addresses: []string{"192.168.1.2"},
						},
					},
				},
				{
					Ports: []discovery.EndpointPort{
						{
							Name: awssdk.String("http"),
							Port: awssdk.Int32(8080),
						},
						{
							Name: awssdk.String("https"),
							Port: awssdk.Int32(8443),
						},
					},
					Endpoints: []discovery.Endpoint{
						{
							Addresses: []string{"192.168.3.1"},
						},
						{
							Addresses: []string{"192.168.3.2"},
						},
					},
				},
			},
		},
		{
			name: "no endpointSlices",
			args: args{
				epsList: &discovery.EndpointSliceList{Items: nil},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildEndpointsDataFromEndpointSliceList(tt.args.epsList)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildPodEndpoint(t *testing.T) {
	type args struct {
		pod    k8s.PodInfo
		epAddr string
		port   int32
	}
	tests := []struct {
		name string
		args args
		want PodEndpoint
	}{
		{
			name: "base case",
			args: args{
				pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "sample-node"},
				},
				epAddr: "192.168.1.1",
				port:   80,
			},
			want: PodEndpoint{
				IP:   "192.168.1.1",
				Port: 80,
				Pod: k8s.PodInfo{
					Key: types.NamespacedName{Name: "sample-node"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPodEndpoint(tt.args.pod, tt.args.epAddr, tt.args.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildNodePortEndpoint(t *testing.T) {
	type args struct {
		node       *corev1.Node
		instanceID string
		nodePort   int32
	}
	tests := []struct {
		name string
		args args
		want NodePortEndpoint
	}{
		{
			name: "base case",
			args: args{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sample-node",
					},
				},
				instanceID: "i-xxxxx",
				nodePort:   33382,
			},
			want: NodePortEndpoint{
				Node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sample-node",
					},
				},
				InstanceID: "i-xxxxx",
				Port:       33382,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildNodePortEndpoint(tt.args.node, tt.args.instanceID, tt.args.nodePort)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_convertCoreEndpointPortToDiscoveryEndpointPort(t *testing.T) {
	protocolTCP := corev1.ProtocolTCP
	type args struct {
		port corev1.EndpointPort
	}
	tests := []struct {
		name string
		args args
		want discovery.EndpointPort
	}{
		{
			name: "port with name",
			args: args{
				port: corev1.EndpointPort{
					Name:        "http",
					Port:        42,
					Protocol:    protocolTCP,
					AppProtocol: awssdk.String("grpc"),
				},
			},
			want: discovery.EndpointPort{
				Name:        awssdk.String("http"),
				Port:        awssdk.Int32(42),
				Protocol:    &protocolTCP,
				AppProtocol: awssdk.String("grpc"),
			},
		},
		{
			name: "port without name",
			args: args{
				port: corev1.EndpointPort{
					Port:        42,
					Protocol:    protocolTCP,
					AppProtocol: awssdk.String("grpc"),
				},
			},
			want: discovery.EndpointPort{
				Port:        awssdk.Int32(42),
				Protocol:    &protocolTCP,
				AppProtocol: awssdk.String("grpc"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertCoreEndpointPortToDiscoveryEndpointPort(tt.args.port)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_convertCoreEndpointAddressToDiscoveryEndpoint(t *testing.T) {
	type args struct {
		endpoint corev1.EndpointAddress
		ready    bool
	}
	tests := []struct {
		name string
		args args
		want discovery.Endpoint
	}{
		{
			name: "ready endpoint",
			args: args{
				endpoint: corev1.EndpointAddress{
					IP: "192.168.1.1",
					TargetRef: &corev1.ObjectReference{
						Kind: "Pod",
						Name: "sample-pod",
					},
					NodeName: awssdk.String("ip-172-20-36-42.us-west-2.compute.internal"),
					Hostname: "ip-172-20-36-42",
				},
				ready: true,
			},
			want: discovery.Endpoint{
				Addresses: []string{"192.168.1.1"},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(true),
					Serving:     awssdk.Bool(true),
					Terminating: awssdk.Bool(false),
				},
				TargetRef: &corev1.ObjectReference{
					Kind: "Pod",
					Name: "sample-pod",
				},
				NodeName: awssdk.String("ip-172-20-36-42.us-west-2.compute.internal"),
				Hostname: awssdk.String("ip-172-20-36-42"),
			},
		},
		{
			name: "ready endpoint - empty hostName",
			args: args{
				endpoint: corev1.EndpointAddress{
					IP: "192.168.1.1",
					TargetRef: &corev1.ObjectReference{
						Kind: "Pod",
						Name: "sample-pod",
					},
					NodeName: awssdk.String("ip-172-20-36-42.us-west-2.compute.internal"),
					Hostname: "",
				},
				ready: true,
			},
			want: discovery.Endpoint{
				Addresses: []string{"192.168.1.1"},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(true),
					Serving:     awssdk.Bool(true),
					Terminating: awssdk.Bool(false),
				},
				TargetRef: &corev1.ObjectReference{
					Kind: "Pod",
					Name: "sample-pod",
				},
				NodeName: awssdk.String("ip-172-20-36-42.us-west-2.compute.internal"),
				Hostname: nil,
			},
		},
		{
			name: "not endpoint",
			args: args{
				endpoint: corev1.EndpointAddress{
					IP: "192.168.1.1",
					TargetRef: &corev1.ObjectReference{
						Kind: "Pod",
						Name: "sample-pod",
					},
					NodeName: awssdk.String("ip-172-20-36-42.us-west-2.compute.internal"),
					Hostname: "ip-172-20-36-42",
				},
				ready: false,
			},
			want: discovery.Endpoint{
				Addresses: []string{"192.168.1.1"},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(false),
					Serving:     awssdk.Bool(false),
					Terminating: awssdk.Bool(false),
				},
				TargetRef: &corev1.ObjectReference{
					Kind: "Pod",
					Name: "sample-pod",
				},
				NodeName: awssdk.String("ip-172-20-36-42.us-west-2.compute.internal"),
				Hostname: awssdk.String("ip-172-20-36-42"),
			},
		},
		{
			name: "not endpoint - empty hostname",
			args: args{
				endpoint: corev1.EndpointAddress{
					IP: "192.168.1.1",
					TargetRef: &corev1.ObjectReference{
						Kind: "Pod",
						Name: "sample-pod",
					},
					NodeName: awssdk.String("ip-172-20-36-42.us-west-2.compute.internal"),
					Hostname: "",
				},
				ready: true,
			},
			want: discovery.Endpoint{
				Addresses: []string{"192.168.1.1"},
				Conditions: discovery.EndpointConditions{
					Ready:       awssdk.Bool(true),
					Serving:     awssdk.Bool(true),
					Terminating: awssdk.Bool(false),
				},
				TargetRef: &corev1.ObjectReference{
					Kind: "Pod",
					Name: "sample-pod",
				},
				NodeName: awssdk.String("ip-172-20-36-42.us-west-2.compute.internal"),
				Hostname: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertCoreEndpointAddressToDiscoveryEndpoint(tt.args.endpoint, tt.args.ready)
			assert.Equal(t, tt.want, got)
		})
	}
}
