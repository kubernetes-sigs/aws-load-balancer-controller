package k8s

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestIsNamespaceTerminating(t *testing.T) {
	deletedAt := metav1.NewTime(time.Date(2026, 11, 17, 18, 4, 45, 0, time.UTC))
	tests := []struct {
		name      string
		phase     corev1.NamespacePhase // empty: the namespace does not exist
		createdAt metav1.Time
		getErr    error
		want      bool
		wantErr   string
	}{
		{name: "active namespace", phase: corev1.NamespaceActive, want: false},
		{name: "terminating namespace", phase: corev1.NamespaceTerminating, createdAt: metav1.NewTime(deletedAt.Add(-time.Minute)), want: true},
		{name: "namespace not found", want: false},
		{name: "namespace lookup fails", getErr: errors.New("cache not ready"), wantErr: "cache not ready"},
		{
			// A CI job can create an object and delete its namespace in the same second. A namespace is
			// recreated at least 5 s after its deletion, so this object is in the terminating namespace.
			name: "object created in the same second as the namespace deletion", phase: corev1.NamespaceTerminating,
			createdAt: deletedAt, want: true,
		},
		{
			// The cached namespace is the old one with the same name.
			name: "object created after the cached namespace started terminating", phase: corev1.NamespaceTerminating,
			createdAt: metav1.NewTime(deletedAt.Add(time.Minute)), want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := testclient.NewClientBuilder()
			if tt.phase != "" {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns"}, Status: corev1.NamespaceStatus{Phase: tt.phase}}
				if tt.phase == corev1.NamespaceTerminating {
					// The fake client needs a finalizer to keep an object with a deletionTimestamp.
					ns.DeletionTimestamp = &deletedAt
					ns.Finalizers = []string{"kubernetes"}
				}
				builder = builder.WithObjects(ns)
			}
			if tt.getErr != nil {
				builder = builder.WithInterceptorFuncs(interceptor.Funcs{
					Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
						return tt.getErr
					},
				})
			}
			svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "svc", CreationTimestamp: tt.createdAt}}
			got, err := IsNamespaceTerminating(context.Background(), builder.Build(), svc)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
