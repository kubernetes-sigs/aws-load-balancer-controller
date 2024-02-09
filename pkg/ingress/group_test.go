package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestGroupID_IsExplicit(t *testing.T) {
	tests := []struct {
		name    string
		groupID GroupID
		want    bool
	}{
		{
			name: "explicit group",
			groupID: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
			want: true,
		},
		{
			name: "implicit group",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.groupID.IsExplicit()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGroupID_String(t *testing.T) {
	tests := []struct {
		name    string
		groupID GroupID
		want    string
	}{
		{
			name: "explicit group",
			groupID: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
			want: "awesome-group",
		},
		{
			name: "implicit group",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
			want: "namespace/ingress",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.groupID.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewGroupIDForExplicitGroup(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		want      GroupID
	}{
		{
			name:      "explicit group",
			groupName: "awesome-group",
			want: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewGroupIDForExplicitGroup(tt.groupName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewGroupIDForImplicitGroup(t *testing.T) {
	tests := []struct {
		name   string
		ingKey types.NamespacedName
		want   GroupID
	}{
		{
			name: "implicit group",
			ingKey: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			},
			want: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewGroupIDForImplicitGroup(tt.ingKey)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEncodeGroupIDToReconcileRequest(t *testing.T) {
	tests := []struct {
		name    string
		groupID GroupID
		want    ctrl.Request
	}{
		{
			name: "explicit group",
			groupID: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
			want: ctrl.Request{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
		},
		{
			name: "implicit group",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
			want: ctrl.Request{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeGroupIDToReconcileRequest(tt.groupID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeGroupIDFromReconcileRequest(t *testing.T) {
	tests := []struct {
		name    string
		request ctrl.Request
		want    GroupID
	}{
		{
			name: "explicit group",
			request: ctrl.Request{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			want: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
		},
		{
			name: "implicit group",
			request: ctrl.Request{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			}},
			want: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeGroupIDFromReconcileRequest(tt.request)
			assert.Equal(t, tt.want, got)
		})
	}
}
