package k8s

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func TestIsNodeReady(t *testing.T) {
	type args struct {
		node *corev1.Node
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "node is ready",
			args: args{
				node: &corev1.Node{
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
			want: true,
		},
		{
			name: "node is not ready when nodeReady condition is false",
			args: args{
				node: &corev1.Node{
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
			want: false,
		},
		{
			name: "node is not ready when nodeReady condition is missing",
			args: args{
				node: &corev1.Node{
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNodeReady(tt.args.node)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetNodeCondition(t *testing.T) {
	type args struct {
		node          *corev1.Node
		conditionType corev1.NodeConditionType
	}
	tests := []struct {
		name string
		args args
		want *corev1.NodeCondition
	}{
		{
			name: "node condition found",
			args: args{
				node: &corev1.Node{
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				conditionType: corev1.NodeReady,
			},
			want: &corev1.NodeCondition{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			},
		},
		{
			name: "node condition not found",
			args: args{
				node: &corev1.Node{
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				conditionType: corev1.NodeMemoryPressure,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetNodeCondition(tt.args.node, tt.args.conditionType)
			assert.Equal(t, tt.want, got)
		})
	}
}
