package k8s

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
)

func TestIsPodHasReadinessGate(t *testing.T) {
	type args struct {
		pod           *corev1.Pod
		conditionType corev1.PodConditionType
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "pod has readinessGate",
			args: args{
				pod: &corev1.Pod{
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "custom-condition",
							},
						},
					},
				},
				conditionType: "custom-condition",
			},
			want: true,
		},
		{
			name: "pod doesn't have readinessGate",
			args: args{
				pod: &corev1.Pod{
					Spec: corev1.PodSpec{
						ReadinessGates: []corev1.PodReadinessGate{
							{
								ConditionType: "another-condition",
							},
						},
					},
				},
				conditionType: "custom-condition",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPodHasReadinessGate(tt.args.pod, tt.args.conditionType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPodContainersReady(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "pod is containersReady",
			args: args{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "pod isn't containersReady when ContainersReady condition is false",
			args: args{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.ContainersReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "pod isn't containersReady when ContainersReady condition is missing",
			args: args{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPodContainersReady(tt.args.pod)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetPodCondition(t *testing.T) {
	type args struct {
		pod           *corev1.Pod
		conditionType corev1.PodConditionType
	}
	tests := []struct {
		name string
		args args
		want *corev1.PodCondition
	}{
		{
			name: "node condition found",
			args: args{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				conditionType: corev1.PodReady,
			},
			want: &corev1.PodCondition{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			},
		},
		{
			name: "node condition not found",
			args: args{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				conditionType: corev1.ContainersReady,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPodCondition(tt.args.pod, tt.args.conditionType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLookupContainerPort(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Ports: []corev1.ContainerPort{
						{
							Name:          "ssh",
							ContainerPort: 22,
						},
					},
				},
				{
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
						{
							ContainerPort: 9999,
						},
					},
				},
			},
		},
	}
	type args struct {
		pod  *corev1.Pod
		port intstr.IntOrString
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr error
	}{
		{
			name: "named pod within pod spec can be found",
			args: args{
				pod:  pod,
				port: intstr.FromString("ssh"),
			},
			want: 22,
		},
		{
			name: "named pod within pod spec(in another container) can be found",
			args: args{
				pod:  pod,
				port: intstr.FromString("http"),
			},
			want: 80,
		},
		{
			name: "named pod within pod spec cannot be found",
			args: args{
				pod:  pod,
				port: intstr.FromString("https"),
			},
			wantErr: errors.New("unable to find port https on pod ns/default"),
		},
		{
			name: "numerical pod within pod spec can be found",
			args: args{
				pod:  pod,
				port: intstr.FromInt(9999),
			},
			want: 9999,
		},
		{
			name: "numerical pod not within pod spec should still be found",
			args: args{
				pod:  pod,
				port: intstr.FromInt(18888),
			},
			want: 18888,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LookupContainerPort(tt.args.pod, tt.args.port)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestUpdatePodCondition(t *testing.T) {
	type args struct {
		pod       *corev1.Pod
		condition corev1.PodCondition
	}
	tests := []struct {
		name    string
		args    args
		wantPod *corev1.Pod
	}{
		{
			name: "pod with existing condition",
			args: args{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:    "some-condition",
								Status:  corev1.ConditionFalse,
								Reason:  "reasonA",
								Message: "messageA",
							},
						},
					},
				},
				condition: corev1.PodCondition{
					Type:    "some-condition",
					Status:  corev1.ConditionTrue,
					Reason:  "reasonB",
					Message: "messageB",
				},
			},
			wantPod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:    "some-condition",
							Status:  corev1.ConditionTrue,
							Reason:  "reasonB",
							Message: "messageB",
						},
					},
				},
			},
		},
		{
			name: "pod without condition",
			args: args{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{},
					},
				},
				condition: corev1.PodCondition{
					Type:    "some-condition",
					Status:  corev1.ConditionTrue,
					Reason:  "reasonB",
					Message: "messageB",
				},
			},
			wantPod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:    "some-condition",
							Status:  corev1.ConditionTrue,
							Reason:  "reasonB",
							Message: "messageB",
						},
					},
				},
			},
		},
		{
			name: "pod without some other condition",
			args: args{
				pod: &corev1.Pod{
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:    "other-condition",
								Status:  corev1.ConditionFalse,
								Reason:  "reasonA",
								Message: "messageA",
							},
						},
					},
				},
				condition: corev1.PodCondition{
					Type:    "some-condition",
					Status:  corev1.ConditionTrue,
					Reason:  "reasonB",
					Message: "messageB",
				},
			},
			wantPod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:    "other-condition",
							Status:  corev1.ConditionFalse,
							Reason:  "reasonA",
							Message: "messageA",
						},
						{
							Type:    "some-condition",
							Status:  corev1.ConditionTrue,
							Reason:  "reasonB",
							Message: "messageB",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := tt.args.pod.DeepCopy()
			UpdatePodCondition(pod, tt.args.condition)
			assert.Equal(t, tt.wantPod, pod)
		})
	}
}
