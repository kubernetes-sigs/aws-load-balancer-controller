package backend

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
)

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
