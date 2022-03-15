package backend

import (
	"errors"
	corev1 "k8s.io/api/core/v1"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
)

func TestDefaultTrafficProxyNodeLabelSelector(t *testing.T) {
	// Test that the default is able to be converted into a label selector
	_, err := metav1.LabelSelectorAsSelector(&defaultTrafficProxyNodeLabelSelector)
	assert.NoError(t, err)
}

func TestGetTrafficProxyNodeSelector(t *testing.T) {
	// Set up the labels.Selector expected objects
	defaultSelector, _ := metav1.LabelSelectorAsSelector(&defaultTrafficProxyNodeLabelSelector)
	reqs, _ := defaultSelector.Requirements()

	customSelector := labels.NewSelector()
	req, _ := labels.NewRequirement("key", selection.Equals, []string{"value"})
	customSelector = customSelector.Add(*req)
	customSelector = customSelector.Add(reqs...) // add default selector rules as well

	tests := []struct {
		name               string
		targetGroupBinding *elbv2api.TargetGroupBinding
		want               labels.Selector
		wantErr            error
	}{
		{
			name:               "default node selector when not specified",
			targetGroupBinding: &elbv2api.TargetGroupBinding{},
			want:               defaultSelector,
		},
		{
			name: "selector from TargetGroupBinding",
			targetGroupBinding: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"key": "value",
						},
					},
				},
			},
			want: customSelector,
		},
		{
			name: "error with bad selector",
			targetGroupBinding: &elbv2api.TargetGroupBinding{
				Spec: elbv2api.TargetGroupBindingSpec{
					NodeSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{Key: "key", Operator: "BadOperatorValue", Values: []string{"value"}},
						},
					},
				},
			},
			wantErr: errors.New("\"BadOperatorValue\" is not a valid pod selector operator"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTrafficProxyNodeSelector(tt.targetGroupBinding)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestIsNodeSuitableAsTrafficProxy(t *testing.T) {
	type args struct {
		node *corev1.Node
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "node is ready and suitable for traffic",
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
					Spec: corev1.NodeSpec{
						Unschedulable: false,
					},
				},
			},
			want: true,
		},
		{
			name: "node is ready but tainted with ToBeDeletedByClusterAutoscaler",
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
					Spec: corev1.NodeSpec{
						Unschedulable: false,
						Taints: []corev1.Taint{
							{
								Key:   toBeDeletedByCATaint,
								Value: "True",
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNodeSuitableAsTrafficProxy(tt.args.node)
			assert.Equal(t, tt.want, got)
		})
	}
}
