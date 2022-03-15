package k8s

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

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

func TestExtractNodeInstanceID(t *testing.T) {
	type args struct {
		node *corev1.Node
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr error
	}{
		{
			name: "node without providerID",
			args: args{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-node-name",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "",
					},
				},
			},
			wantErr: errors.New("providerID is not specified for node: my-node-name"),
		},
		{
			name: "node by EC2 instance",
			args: args{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-node-name",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws:///us-west-2b/i-abcdefg0",
					},
				},
			},
			want: "i-abcdefg0",
		},
		{
			name: "node by EKS Fargate",
			args: args{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "fargate-ip-192-168-138-30.us-west-2.compute.internal",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws:///us-west-2b/368270442a-793d42d32c704bb793ca88a6a14ddd6e/fargate-ip-192-168-138-30.us-west-2.compute.internal",
					},
				},
			},
			wantErr: errors.New("providerID aws:///us-west-2b/368270442a-793d42d32c704bb793ca88a6a14ddd6e/fargate-ip-192-168-138-30.us-west-2.compute.internal is invalid for EC2 instances, node: fargate-ip-192-168-138-30.us-west-2.compute.internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractNodeInstanceID(tt.args.node)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
