package k8s

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
)

func TestLookupServicePort(t *testing.T) {
	svc1 := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "svc-1",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					NodePort: 18080,
				},
				{
					Name:     "https",
					Port:     443,
					NodePort: 18443,
				},
			},
		},
	}
	type args struct {
		svc  *corev1.Service
		port intstr.IntOrString
	}
	tests := []struct {
		name    string
		args    args
		want    corev1.ServicePort
		wantErr error
	}{
		{
			name: "find servicePort by port name",
			args: args{
				svc:  svc1,
				port: intstr.FromString("https"),
			},
			want: corev1.ServicePort{
				Name:     "https",
				Port:     443,
				NodePort: 18443,
			},
		},
		{
			name: "find servicePort by port value",
			args: args{
				svc:  svc1,
				port: intstr.FromInt(80),
			},
			want: corev1.ServicePort{
				Name:     "http",
				Port:     80,
				NodePort: 18080,
			},
		},
		{
			name: "cannot find servicePort by port name",
			args: args{
				svc:  svc1,
				port: intstr.FromString("ssh"),
			},
			wantErr: errors.New("unable to find port ssh on service test-ns/svc-1"),
		},
		{
			name: "cannot find servicePort by port value",
			args: args{
				svc:  svc1,
				port: intstr.FromInt(22),
			},
			wantErr: errors.New("unable to find port 22 on service test-ns/svc-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LookupServicePort(tt.args.svc, tt.args.port)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
