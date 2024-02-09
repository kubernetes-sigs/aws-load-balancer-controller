package k8s

import (
	"errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
)

func TestPodInfo_HasAnyOfReadinessGates(t *testing.T) {
	type args struct {
		conditionTypes []corev1.PodConditionType
	}
	tests := []struct {
		name string
		pod  PodInfo
		args args
		want bool
	}{
		{
			name: "empty readinessGates passed-in",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				ReadinessGates: []corev1.PodReadinessGate{
					{
						ConditionType: "ingress.k8s.aws/cond-1",
					},
				},
			},
			args: args{
				conditionTypes: []corev1.PodConditionType{},
			},
			want: false,
		},
		{
			name: "contains the readinessGates passed-in",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				ReadinessGates: []corev1.PodReadinessGate{
					{
						ConditionType: "ingress.k8s.aws/cond-1",
					},
				},
			},
			args: args{
				conditionTypes: []corev1.PodConditionType{
					"ingress.k8s.aws/cond-1",
				},
			},
			want: true,
		},
		{
			name: "doesn't contain the readinessGates passed-in",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				ReadinessGates: []corev1.PodReadinessGate{
					{
						ConditionType: "ingress.k8s.aws/cond-1",
					},
				},
			},
			args: args{
				conditionTypes: []corev1.PodConditionType{
					"ingress.k8s.aws/cond-2",
				},
			},
			want: false,
		},
		{
			name: "contains one of the readinessGates passed-in",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				ReadinessGates: []corev1.PodReadinessGate{
					{
						ConditionType: "ingress.k8s.aws/cond-1",
					},
				},
			},
			args: args{
				conditionTypes: []corev1.PodConditionType{
					"ingress.k8s.aws/cond-1",
					"ingress.k8s.aws/cond-2",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pod.HasAnyOfReadinessGates(tt.args.conditionTypes)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPodInfo_IsContainersReady(t *testing.T) {
	tests := []struct {
		name string
		pod  PodInfo
		want bool
	}{
		{
			name: "pod have true containers ready condition",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
			want: true,
		},
		{
			name: "pod have false containers ready condition",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
			want: false,
		},
		{
			name: "pod don't have containers ready condition",
			pod: PodInfo{
				Key:        types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				Conditions: []corev1.PodCondition{},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pod.IsContainersReady()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPodInfo_GetPodCondition(t *testing.T) {
	type args struct {
		conditionType corev1.PodConditionType
	}
	tests := []struct {
		name       string
		pod        PodInfo
		args       args
		want       corev1.PodCondition
		wantExists bool
	}{
		{
			name: "condition exists",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
			args: args{
				conditionType: corev1.ContainersReady,
			},
			want: corev1.PodCondition{
				Type:   corev1.ContainersReady,
				Status: corev1.ConditionFalse,
			},
			wantExists: true,
		},
		{
			name: "condition doesn't exists",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
			args: args{
				conditionType: corev1.PodReady,
			},
			want:       corev1.PodCondition{},
			wantExists: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotExists := tt.pod.GetPodCondition(tt.args.conditionType)
			assert.Equal(t, tt.wantExists, gotExists)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPodInfo_LookupContainerPort(t *testing.T) {
	type args struct {
		port intstr.IntOrString
	}
	tests := []struct {
		name    string
		pod     PodInfo
		args    args
		want    int64
		wantErr error
	}{
		{
			name: "str container port exists",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				ContainerPorts: []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 8080,
					},
					{
						Name:          "https",
						ContainerPort: 8443,
					},
				},
			},
			args: args{
				port: intstr.FromString("https"),
			},
			want: 8443,
		},
		{
			name: "str container port don't exists",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				ContainerPorts: []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 8080,
					},
					{
						Name:          "https",
						ContainerPort: 8443,
					},
				},
			},
			args: args{
				port: intstr.FromString("ssh"),
			},
			wantErr: errors.New("unable to find port ssh on pod ns-1/pod-1"),
		},
		{
			name: "int container port exists",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				ContainerPorts: []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 8080,
					},
					{
						Name:          "https",
						ContainerPort: 8443,
					},
				},
			},
			args: args{
				port: intstr.FromInt(8080),
			},
			want: 8080,
		},
		{
			name: "int container port don't exists",
			pod: PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"},
				ContainerPorts: []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 8080,
					},
					{
						Name:          "https",
						ContainerPort: 8443,
					},
				},
			},
			args: args{
				port: intstr.FromInt(9090),
			},
			want: 9090,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.pod.LookupContainerPort(tt.args.port)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_buildPodInfo(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want PodInfo
	}{
		{
			name: "standard case",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-ns",
						Name:      "pod-1",
						UID:       "pod-uuid",
					},
					Spec: corev1.PodSpec{
						NodeName: "ip-192-168-13-198.us-west-2.compute.internal",
						Containers: []corev1.Container{
							{
								Ports: []corev1.ContainerPort{
									{
										Name:          "ssh",
										ContainerPort: 22,
									},
									{
										Name:          "http",
										ContainerPort: 8080,
									},
								},
							},
							{
								Ports: []corev1.ContainerPort{
									{
										Name:          "https",
										ContainerPort: 8443,
									},
								},
							},
						},
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "ingress.k8s.aws/cond-1",
							},
							{
								ConditionType: "ingress.k8s.aws/cond-2",
							},
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
							{
								Type:   "ingress.k8s.aws/cond-2",
								Status: corev1.ConditionTrue,
							},
						},
						PodIP: "192.168.1.1",
					},
				},
			},
			want: PodInfo{
				Key: types.NamespacedName{Namespace: "my-ns", Name: "pod-1"},
				UID: "pod-uuid",
				ContainerPorts: []corev1.ContainerPort{
					{
						Name:          "ssh",
						ContainerPort: 22,
					},
					{
						Name:          "http",
						ContainerPort: 8080,
					},
					{
						Name:          "https",
						ContainerPort: 8443,
					},
				},
				ReadinessGates: []corev1.PodReadinessGate{
					{
						ConditionType: "ingress.k8s.aws/cond-1",
					},
					{
						ConditionType: "ingress.k8s.aws/cond-2",
					},
				},
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   "ingress.k8s.aws/cond-2",
						Status: corev1.ConditionTrue,
					},
				},
				NodeName: "ip-192-168-13-198.us-west-2.compute.internal",
				PodIP:    "192.168.1.1",
			},
		},
		{
			name: "standard case - with ENIInfo",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-ns",
						Name:      "pod-1",
						UID:       "pod-uuid",
						Annotations: map[string]string{
							"vpc.amazonaws.com/pod-eni": `[{"eniId":"eni-06a712e1622fda4a0","ifAddress":"02:34:a5:25:0b:63","privateIp":"192.168.219.103","vlanId":3,"subnetCidr":"192.168.192.0/19"}]`,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "ip-192-168-13-198.us-west-2.compute.internal",
						Containers: []corev1.Container{
							{
								Ports: []corev1.ContainerPort{
									{
										Name:          "ssh",
										ContainerPort: 22,
									},
									{
										Name:          "http",
										ContainerPort: 8080,
									},
								},
							},
							{
								Ports: []corev1.ContainerPort{
									{
										Name:          "https",
										ContainerPort: 8443,
									},
								},
							},
						},
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "ingress.k8s.aws/cond-1",
							},
							{
								ConditionType: "ingress.k8s.aws/cond-2",
							},
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
							{
								Type:   "ingress.k8s.aws/cond-2",
								Status: corev1.ConditionTrue,
							},
						},
						PodIP: "192.168.1.1",
					},
				},
			},
			want: PodInfo{
				Key: types.NamespacedName{Namespace: "my-ns", Name: "pod-1"},
				UID: "pod-uuid",
				ContainerPorts: []corev1.ContainerPort{
					{
						Name:          "ssh",
						ContainerPort: 22,
					},
					{
						Name:          "http",
						ContainerPort: 8080,
					},
					{
						Name:          "https",
						ContainerPort: 8443,
					},
				},
				ReadinessGates: []corev1.PodReadinessGate{
					{
						ConditionType: "ingress.k8s.aws/cond-1",
					},
					{
						ConditionType: "ingress.k8s.aws/cond-2",
					},
				},
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.ContainersReady,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   "ingress.k8s.aws/cond-2",
						Status: corev1.ConditionTrue,
					},
				},
				NodeName: "ip-192-168-13-198.us-west-2.compute.internal",
				PodIP:    "192.168.1.1",
				ENIInfos: []PodENIInfo{
					{
						ENIID:     "eni-06a712e1622fda4a0",
						PrivateIP: "192.168.219.103",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPodInfo(tt.args.pod)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_buildPodENIInfo(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	tests := []struct {
		name    string
		args    args
		want    []PodENIInfo
		wantErr error
	}{
		{
			name: "pod-eni annotation exists and valid",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"vpc.amazonaws.com/pod-eni": `[{"eniId":"eni-06a712e1622fda4a0","ifAddress":"02:34:a5:25:0b:63","privateIp":"192.168.219.103","vlanId":3,"subnetCidr":"192.168.192.0/19"}]`,
						},
					},
				},
			},
			want: []PodENIInfo{
				{
					ENIID:     "eni-06a712e1622fda4a0",
					PrivateIP: "192.168.219.103",
				},
			},
		},
		{
			name: "pod-eni annotation didn't exist",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
			},
			want: nil,
		},
		{
			name: "pod-eni annotation invalid",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"vpc.amazonaws.com/pod-eni": ``,
						},
					},
				},
			},
			wantErr: errors.New("unexpected end of JSON input"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPodENIInfos(tt.args.pod)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
