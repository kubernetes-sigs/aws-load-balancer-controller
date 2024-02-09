package k8s

import (
	"errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

func Test_podInfoKeyFunc(t *testing.T) {
	type args struct {
		obj interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr error
	}{
		{
			name: "normal case",
			args: args{
				obj: &PodInfo{
					Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-a"},
				},
			},
			want: "ns-1/pod-a",
		},
		{
			name: "invalid type",
			args: args{
				obj: PodInfo{
					Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-a"},
				},
			},
			wantErr: errors.New("expect PodInfo object"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := podInfoKeyFunc(tt.args.obj)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_podInfoConversionFunc(t *testing.T) {
	type args struct {
		obj interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    interface{}
		wantErr error
	}{
		{
			name: "normal case",
			args: args{
				obj: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "pod-a",
						UID:       "pod-uuid",
					},
				},
			},
			want: &PodInfo{
				Key: types.NamespacedName{Namespace: "ns-1", Name: "pod-a"},
				UID: "pod-uuid",
			},
		},
		{
			name: "invalid type",
			args: args{
				obj: corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-1",
						Name:      "pod-a",
						UID:       "pod-uuid",
					},
				},
			},
			wantErr: errors.New("expect pod object"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := podInfoConversionFunc(tt.args.obj)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
