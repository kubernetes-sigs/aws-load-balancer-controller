package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
)

func Test_typedInformerDeletedObject(t *testing.T) {
	pod := &PodInfo{Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-1"}}
	tests := []struct {
		name   string
		obj    any
		want   *PodInfo
		wantOk bool
	}{
		{
			name:   "plain object of type T",
			obj:    pod,
			want:   pod,
			wantOk: true,
		},
		{
			name:   "DeletedFinalStateUnknown tombstone wrapping type T",
			obj:    toolscache.DeletedFinalStateUnknown{Key: "ns-1/pod-1", Obj: pod},
			want:   pod,
			wantOk: true,
		},
		{
			name:   "DeletedFinalStateUnknown tombstone wrapping an unrelated type",
			obj:    toolscache.DeletedFinalStateUnknown{Key: "ns-1/pod-1", Obj: "not-a-podinfo"},
			want:   nil,
			wantOk: false,
		},
		{
			name:   "unrelated type, not a tombstone",
			obj:    "not-a-podinfo",
			want:   nil,
			wantOk: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := typedInformerDeletedObject[*PodInfo](tt.obj)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
