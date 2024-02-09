package eventhandlers

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func Test_enqueueRequestsForNodeEvent_shouldEnqueueTGBDueToNodeEvent(t *testing.T) {
	type args struct {
		nodeOldSuitableAsTrafficProxyForTGB bool
		nodeOldReadyCondStatus              corev1.ConditionStatus
		nodeNewSuitableAsTrafficProxyForTGB bool
		nodeNewReadyCondStatus              corev1.ConditionStatus
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "suitable node changed from ready to notReady",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionTrue,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionFalse,
			},
			want: true,
		},
		{
			name: "suitable node changed from ready to unknown",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionTrue,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionUnknown,
			},
			want: true,
		},
		{
			name: "suitable node changed from notReady to ready",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionFalse,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionTrue,
			},
			want: true,
		},
		{
			name: "suitable node changed from notReady to unknown",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionFalse,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionUnknown,
			},
			want: true,
		},
		{
			name: "suitable node changed from unknown to ready",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionUnknown,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionTrue,
			},
			want: true,
		},
		{
			name: "suitable node changed from unknown to notReady",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionUnknown,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionFalse,
			},
			want: true,
		},
		{
			name: "suitable node remains ready",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionTrue,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionTrue,
			},
			want: false,
		},
		{
			name: "suitable node remains notReady",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionFalse,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionFalse,
			},
			want: false,
		},
		{
			name: "suitable node remains unknown",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionUnknown,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionUnknown,
			},
			want: false,
		},
		{
			name: "non-suitable node changed from ready to notReady",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: false,
				nodeOldReadyCondStatus:              corev1.ConditionTrue,
				nodeNewSuitableAsTrafficProxyForTGB: false,
				nodeNewReadyCondStatus:              corev1.ConditionFalse,
			},
			want: false,
		},
		{
			name: "node became suitable while remains ready",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: false,
				nodeOldReadyCondStatus:              corev1.ConditionTrue,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionTrue,
			},
			want: true,
		},
		{
			name: "node became suitable while remains notReady",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: false,
				nodeOldReadyCondStatus:              corev1.ConditionFalse,
				nodeNewSuitableAsTrafficProxyForTGB: true,
				nodeNewReadyCondStatus:              corev1.ConditionFalse,
			},
			want: false,
		},
		{
			name: "node became non-suitable while remains ready",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionTrue,
				nodeNewSuitableAsTrafficProxyForTGB: false,
				nodeNewReadyCondStatus:              corev1.ConditionTrue,
			},
			want: true,
		},
		{
			name: "node became non-suitable while remains notReady",
			args: args{
				nodeOldSuitableAsTrafficProxyForTGB: true,
				nodeOldReadyCondStatus:              corev1.ConditionFalse,
				nodeNewSuitableAsTrafficProxyForTGB: false,
				nodeNewReadyCondStatus:              corev1.ConditionFalse,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &enqueueRequestsForNodeEvent{}
			got := h.shouldEnqueueTGBDueToNodeEvent(tt.args.nodeOldSuitableAsTrafficProxyForTGB, tt.args.nodeOldReadyCondStatus,
				tt.args.nodeNewSuitableAsTrafficProxyForTGB, tt.args.nodeNewReadyCondStatus)
			assert.Equal(t, tt.want, got)
		})
	}
}
