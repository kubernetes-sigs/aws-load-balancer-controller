package lb

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/service/wafv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildWAFV2TestIngress(wafIngressAnnotations map[string]string) *extensions.Ingress {
	defaultBackend := extensions.IngressBackend{
		ServiceName: "default-backend",
		ServicePort: intstr.FromInt(80),
	}

	return &extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   apiv1.NamespaceDefault,
			Annotations: wafIngressAnnotations,
		},
		Spec: extensions.IngressSpec{
			Backend: &extensions.IngressBackend{
				ServiceName: "default-backend",
				ServicePort: intstr.FromInt(80),
			},
			Rules: []extensions.IngressRule{
				{
					Host: "foo.bar.com",
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path:    "/foo",
									Backend: defaultBackend,
								},
							},
						},
					},
				},
			},
		},
	}
}

func Test_defaultWAFV2Controller_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		Name                             string
		GetWAFV2WebACLSummaryResponse    *wafv2.WebACL
		GetWAFV2WebACLSummaryError       error
		AssociateWAFV2Response           *wafv2.AssociateWebACLOutput
		AssociateWAFV2Error              error
		DisassociateWAFV2Response        *wafv2.DisassociateWebACLOutput
		DisassociateWAFV2Error           error
		Expected                         error
		ExpectedError                    error
		LoadBalancerARN                  string
		IngressAnnotations               *extensions.Ingress
		DesiredWebACLARN                 string
		GetWAFV2WebACLSummaryTimesCalled int
		AssociateWAFV2TimesCalled        int
		DisassociateWAFV2TimesCalled     int
	}{
		{
			Name: "No annotation, confirm nothing is attached",
			GetWAFV2WebACLSummaryResponse: &wafv2.WebACL{
				ARN: nil,
			},
			GetWAFV2WebACLSummaryError: nil,
			AssociateWAFV2Response:     &wafv2.AssociateWebACLOutput{},
			AssociateWAFV2Error:        nil,
			Expected:                   nil,
			LoadBalancerARN:            "arn:lb",
			IngressAnnotations: buildWAFV2TestIngress(
				map[string]string{},
			),
			DesiredWebACLARN:                 "",
			GetWAFV2WebACLSummaryTimesCalled: 1,
			AssociateWAFV2TimesCalled:        0,
			DisassociateWAFV2TimesCalled:     0,
		},
		{
			Name: "Empty WAFv2 annotation",
			GetWAFV2WebACLSummaryResponse: &wafv2.WebACL{
				ARN: nil,
			},
			GetWAFV2WebACLSummaryError: nil,
			AssociateWAFV2Response:     &wafv2.AssociateWebACLOutput{},
			AssociateWAFV2Error:        nil,
			Expected:                   nil,
			LoadBalancerARN:            "arn:lb",
			IngressAnnotations: buildWAFV2TestIngress(
				map[string]string{
					"alb.ingress.kubernetes.io/wafv2-acl-arn": "",
				},
			),
			DesiredWebACLARN:                 "",
			GetWAFV2WebACLSummaryTimesCalled: 1,
			AssociateWAFV2TimesCalled:        0,
			DisassociateWAFV2TimesCalled:     0,
		},
		{
			Name: "No annotation, detach WAFv2",
			GetWAFV2WebACLSummaryResponse: &wafv2.WebACL{
				ARN: aws.String("arn:aws:wafv2:us-east-1:000000000000:regional/webacl/name/0aa00000-00a0-00a0-a0a0-0a0000a0000a"),
			},
			GetWAFV2WebACLSummaryError: nil,
			AssociateWAFV2Response:     &wafv2.AssociateWebACLOutput{},
			AssociateWAFV2Error:        nil,
			Expected:                   nil,
			LoadBalancerARN:            "arn:lb",
			IngressAnnotations: buildWAFV2TestIngress(
				map[string]string{},
			),
			DesiredWebACLARN:                 "",
			GetWAFV2WebACLSummaryTimesCalled: 1,
			AssociateWAFV2TimesCalled:        0,
			DisassociateWAFV2TimesCalled:     1,
		},
		{
			Name: "Empty annotation, dissassociate WAFv2",
			GetWAFV2WebACLSummaryResponse: &wafv2.WebACL{
				ARN: aws.String("arn:aws:wafv2:us-east-1:000000000000:regional/webacl/name/0aa00000-00a0-00a0-a0a0-0a0000a0000a"),
			},
			GetWAFV2WebACLSummaryError: nil,
			AssociateWAFV2Response:     &wafv2.AssociateWebACLOutput{},
			AssociateWAFV2Error:        nil,
			Expected:                   nil,
			LoadBalancerARN:            "arn:lb",
			IngressAnnotations: buildWAFV2TestIngress(
				map[string]string{
					"alb.ingress.kubernetes.io/wafv2-acl-arn": "",
				},
			),
			DesiredWebACLARN:                 "",
			GetWAFV2WebACLSummaryTimesCalled: 1,
			AssociateWAFV2TimesCalled:        0,
			DisassociateWAFV2TimesCalled:     1,
		},
		{
			Name: "Annotation, associate WAFv2",
			GetWAFV2WebACLSummaryResponse: &wafv2.WebACL{
				ARN: nil,
			},
			GetWAFV2WebACLSummaryError: nil,
			AssociateWAFV2Response:     &wafv2.AssociateWebACLOutput{},
			AssociateWAFV2Error:        nil,
			Expected:                   nil,
			LoadBalancerARN:            "arn:lb",
			IngressAnnotations: buildWAFV2TestIngress(
				map[string]string{
					"alb.ingress.kubernetes.io/wafv2-acl-arn": "arn:aws:wafv2:us-east-1:000000000000:regional/webacl/name/0aa00000-00a0-00a0-a0a0-0a0000a0000a",
				},
			),
			DesiredWebACLARN:                 "arn:aws:wafv2:us-east-1:000000000000:regional/webacl/name/0aa00000-00a0-00a0-a0a0-0a0000a0000a",
			GetWAFV2WebACLSummaryTimesCalled: 1,
			AssociateWAFV2TimesCalled:        1,
			DisassociateWAFV2TimesCalled:     0,
		},
		{
			Name: "Annotation, change WAFv2",
			GetWAFV2WebACLSummaryResponse: &wafv2.WebACL{
				ARN: aws.String("arn:aws:wafv2:us-east-1:000000000000:regional/webacl/name/0bb00000-00b0-00b0-b0b0-0b0000a0000b"),
			},
			GetWAFV2WebACLSummaryError: nil,
			Expected:                   nil,
			AssociateWAFV2Response:     &wafv2.AssociateWebACLOutput{},
			AssociateWAFV2Error:        nil,
			LoadBalancerARN:            "arn:lb",
			IngressAnnotations: buildWAFV2TestIngress(
				map[string]string{
					"alb.ingress.kubernetes.io/wafv2-acl-arn": "arn:aws:wafv2:us-east-1:000000000000:regional/webacl/name/0aa00000-00a0-00a0-a0a0-0a0000a0000a",
				},
			),
			DesiredWebACLARN:                 "arn:aws:wafv2:us-east-1:000000000000:regional/webacl/name/0aa00000-00a0-00a0-a0a0-0a0000a0000a",
			GetWAFV2WebACLSummaryTimesCalled: 1,
			AssociateWAFV2TimesCalled:        1,
			DisassociateWAFV2TimesCalled:     0,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}

			cloud.On("GetWAFV2WebACLSummary", ctx, aws.String(tc.LoadBalancerARN)).Return(tc.GetWAFV2WebACLSummaryResponse, tc.GetWAFV2WebACLSummaryError)
			cloud.On("AssociateWAFV2", ctx, aws.String(tc.LoadBalancerARN), aws.String(tc.DesiredWebACLARN)).Return(tc.AssociateWAFV2Response, tc.AssociateWAFV2Error)
			cloud.On("DisassociateWAFV2", ctx, aws.String(tc.LoadBalancerARN)).Return(tc.DisassociateWAFV2Response, tc.DisassociateWAFV2Error)

			controller := NewWAFV2Controller(cloud)
			err := controller.Reconcile(ctx, tc.LoadBalancerARN, tc.IngressAnnotations)
			assert.Equal(t, tc.Expected, err)
			cloud.AssertNumberOfCalls(t, "GetWAFV2WebACLSummary", tc.GetWAFV2WebACLSummaryTimesCalled)
			cloud.AssertNumberOfCalls(t, "AssociateWAFV2", tc.AssociateWAFV2TimesCalled)
			cloud.AssertNumberOfCalls(t, "DisassociateWAFV2", tc.DisassociateWAFV2TimesCalled)
		})
	}
}
