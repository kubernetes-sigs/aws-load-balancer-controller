package tg

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
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
			"alb-92ed2e2b69f211d4078",
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
			"foobar-92ed2e2b69f211d4078",
		}, {
			"with different load balancer ID",
			&NewDesiredTargetGroupOptions{
				Store:          store.NewDummy(),
				LoadBalancerID: "foo",
				Annotations:    annotations.NewServiceDummy(),
			},
			"alb-304ea936d403949af49",
		}, {
			"with different service name",
			&NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				SvcName:     "foo",
				Annotations: annotations.NewServiceDummy(),
			},
			"alb-304ea936d403949af49",
		}, {
			"with different service port",
			&NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				SvcPort:     intstr.FromString("foo"),
				Annotations: annotations.NewServiceDummy(),
			},
			"alb-2e68afc230e92775bec",
		}, {
			"with different target port",
			&NewDesiredTargetGroupOptions{
				Store:       store.NewDummy(),
				TargetPort:  4242,
				Annotations: annotations.NewServiceDummy(),
			},
			"alb-4f3fdc5fee7729ae94a",
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
			"alb-0799807a8ec9d79e779",
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
			"alb-6215494e1745d09fcb9",
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
