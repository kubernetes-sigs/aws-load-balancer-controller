package tg

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_generateID(t *testing.T) {
	var tests = []struct {
		name string
		opts *NewDesiredTargetGroupOptions
		want string
	}{
		{
			"with dummy defaults",
			&NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				Annotations: annotations.NewServiceDummy(),
			},
			"alb-1da0bc925ceedb35366",
		}, {
			"with different ALB name prefix",
			&NewDesiredTargetGroupOptions{
				Store: (func() store.Storer {
					s := store.NewDummy()
					s.SetConfig(&config.Configuration{
						ALBNamePrefix: "foobar",
					})
					return s
				})(),
				Annotations: annotations.NewServiceDummy(),
			},
			"foobar-1da0bc925ceedb35366",
		}, {
			"with different load balancer ID",
			&NewDesiredTargetGroupOptions{
				Store:          store.NewDummy(),
				LoadBalancerID: "foo",
				Annotations:    annotations.NewServiceDummy(),
			},
			"alb-2e68afc230e92775bec",
		}, {
			"with different service name",
			&NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				SvcName:     "foo",
				Annotations: annotations.NewServiceDummy(),
			},
			"alb-2e68afc230e92775bec",
		}, {
			"with different service port",
			&NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				SvcPort:     intstr.FromString("foo"),
				Annotations: annotations.NewServiceDummy(),
			},
			"alb-91d03822c744c2df56f",
		}, {
			"with different target port",
			&NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				Annotations: annotations.NewServiceDummy(),
			},
			"alb-1da0bc925ceedb35366",
		}, {
			"with different target group backend protocol",
			&NewDesiredTargetGroupOptions{
				Store: store.NewDummy(),
				Annotations: (func() *annotations.Service {
					ann := annotations.NewServiceDummy()
					ann.TargetGroup.BackendProtocol = aws.String(elbv2.ProtocolEnumHttps)
					return ann
				})(),
			},
			"alb-1a6b3ee515f8413fa85",
		}, {
			"with different target group type",
			&NewDesiredTargetGroupOptions{
				Store: store.NewDummy(),
				Annotations: (func() *annotations.Service {
					ann := annotations.NewServiceDummy()
					ann.TargetGroup.TargetType = aws.String(elbv2.TargetTypeEnumIp)
					return ann
				})(),
			},
			"alb-eb4e98337503d377426",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			have := test.opts.generateID()
			if test.want != have {
				t.Errorf("Expected '%s', got '%s'", test.want, have)
			}
		})
	}
}
