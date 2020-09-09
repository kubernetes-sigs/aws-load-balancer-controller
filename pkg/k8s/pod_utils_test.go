package k8s

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
