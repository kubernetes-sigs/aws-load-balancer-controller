package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The pod informer transform must tolerate being applied twice. With the
// WatchListClient client-go feature, the reflector transforms objects in its
// temporary store during streaming initial sync, then the destination FIFO
// applies the transform again on Replace. A transform that rejects its own
// output aborts the streaming sync and forces a fallback to a full LIST.
func Test_podInfoConverter_idempotent(t *testing.T) {
	b := newPodInfoBuilder("")
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "pod", UID: "uid"}}

	first, err := b.podInfoConverter(pod)
	assert.NoError(t, err)
	podInfo, ok := first.(*PodInfo)
	assert.True(t, ok)

	second, err := b.podInfoConverter(first)
	assert.NoError(t, err)
	assert.Same(t, podInfo, second)
}
