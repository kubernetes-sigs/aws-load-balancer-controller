package tg

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/stretchr/testify/assert"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_generateID(t *testing.T) {
	var tests = []struct {
		name string
		opts *NewDesiredTargetGroupOptions
		want string
		err  error
	}{
		{
			name: "with dummy defaults",
			opts: &NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				Annotations: annotations.NewServiceDummy(),
				Backend:     &extensions.IngressBackend{ServiceName: "", ServicePort: intstr.FromInt(0)},
			},
			want: "alb-1da0bc925ceedb35366",
		}, {
			name: "with different ALB name prefix",
			opts: &NewDesiredTargetGroupOptions{
				Store: (func() store.Storer {
					s := store.NewDummy()
					s.SetConfig(&config.Configuration{
						ALBNamePrefix: "foobar",
					})
					return s
				})(),
				Annotations: annotations.NewServiceDummy(),
				Backend:     &extensions.IngressBackend{ServiceName: "", ServicePort: intstr.FromInt(0)},
			},
			want: "foobar-1da0bc925ceedb35366",
		}, {
			name: "with different load balancer ID",
			opts: &NewDesiredTargetGroupOptions{
				Store:          store.NewDummy(),
				LoadBalancerID: "foo",
				Annotations:    annotations.NewServiceDummy(),
				Backend:        &extensions.IngressBackend{ServiceName: "", ServicePort: intstr.FromInt(0)},
			},
			want: "alb-2e68afc230e92775bec",
		}, {
			name: "with different service name",
			opts: &NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				Backend:     &extensions.IngressBackend{ServiceName: "Foo", ServicePort: intstr.FromInt(0)},
				Annotations: annotations.NewServiceDummy(),
			},
			want: "alb-1cf41ea0a746736d707",
		}, {
			name: "with different service port",
			opts: &NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				Backend:     &extensions.IngressBackend{ServiceName: "", ServicePort: intstr.FromString("foo")},
				Annotations: annotations.NewServiceDummy(),
			},
			want: "alb-91d03822c744c2df56f",
		}, {
			name: "with different target group backend protocol",
			opts: &NewDesiredTargetGroupOptions{
				Store: store.NewDummy(),
				Annotations: (func() *annotations.Service {
					ann := annotations.NewServiceDummy()
					ann.TargetGroup.BackendProtocol = aws.String(elbv2.ProtocolEnumHttps)
					return ann
				})(),
				Backend: &extensions.IngressBackend{ServiceName: "", ServicePort: intstr.FromInt(0)},
			},
			want: "alb-1a6b3ee515f8413fa85",
		}, {
			name: "with different target group type",
			opts: &NewDesiredTargetGroupOptions{
				Store: store.NewDummy(),
				Annotations: (func() *annotations.Service {
					ann := annotations.NewServiceDummy()
					ann.TargetGroup.TargetType = aws.String(elbv2.TargetTypeEnumIp)
					return ann
				})(),
				Backend: &extensions.IngressBackend{ServiceName: "", ServicePort: intstr.FromInt(0)},
			},
			want: "alb-eb4e98337503d377426",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			have, err := test.opts.generateID()
			assert.Equal(t, test.err, err)
			if test.want != have {
				t.Errorf("Expected '%s', got '%s'", test.want, have)
			}
		})
	}
}
