package tg

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/targetgroup"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"testing"
)

func Test_generateID(t *testing.T) {
	var tests = []struct {
		name string
		opts *NewDesiredTargetGroupOptions
		want string
	}{
		{
			"plain",
			&NewDesiredTargetGroupOptions{
				Store: store.NewDummy(),
				Annotations: annotations.NewServiceDummy(),
			},
			"alb-92ed2e2b69f211d4078",
		}, {
			"with ALB name prefix",
			&NewDesiredTargetGroupOptions{
				Store: store.NewDummy().SetConfig(&config.Configuration{
					ALBNamePrefix: "foobar",
				}),
				Annotations: annotations.NewServiceDummy(),
			},
			"foobar-92ed2e2b69f211d4078",
		}, {
			"with lb scheme internet-facing",
			&NewDesiredTargetGroupOptions{
				Store: &store.Dummy{},
				Annotations: &annotations.Service{
					LoadBalancer: &loadbalancer.Config{
						Scheme: aws.String(elbv2.LoadBalancerSchemeEnumInternetFacing),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol: aws.String(elbv2.ProtocolEnumHttps),
						TargetType:      aws.String(elbv2.TargetTypeEnumIp),
					},
				},
			},
			"alb-87a2e9e36f52bfd9690",
		}, {
			"with lb scheme internal",
			&NewDesiredTargetGroupOptions{
				Store: &store.Dummy{},
				Annotations: &annotations.Service{
					LoadBalancer: &loadbalancer.Config{
						Scheme: aws.String(elbv2.LoadBalancerSchemeEnumInternal),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol: aws.String(elbv2.ProtocolEnumHttps),
						TargetType:      aws.String(elbv2.TargetTypeEnumIp),
					},
				},
			},
			"alb-f03287dfdae0195a939",
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
