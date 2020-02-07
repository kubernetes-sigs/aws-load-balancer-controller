package lb

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/aws/aws-sdk-go/service/shield"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildIngress(shieldEnabled string) *extensions.Ingress {
	defaultBackend := extensions.IngressBackend{
		ServiceName: "default-backend",
		ServicePort: intstr.FromInt(80),
	}

	var shieldAnnos map[string]string
	if shieldEnabled == "true" || shieldEnabled == "false" {
		shieldAnnos = map[string]string{
			"alb.ingress.kubernetes.io/shield-advanced-protection": shieldEnabled,
		}
	} else {
		shieldAnnos = map[string]string{}
	}

	return &extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   apiv1.NamespaceDefault,
			Annotations: shieldAnnos,
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

func Test_defaultShieldController_Reconcile(t *testing.T) {
	for _, tc := range []struct {
		Name                        string
		ShieldAvailableResponse     bool
		ShieldAvailableError        error
		GetProtectionResponse       *shield.Protection
		GetProtectionError          error
		CreateProtectionResponse    *shield.CreateProtectionOutput
		CreateProtectionError       error
		DeleteProtectionResponse    *shield.DeleteProtectionOutput
		DeleteProtectionError       error
		Expected                    error
		ExpectedError               error
		LoadBalancerARN             string
		IngressAnnocation           *extensions.Ingress
		ShieldAvailableTimesCalled  int
		GetProtectionTimesCalled    int
		CreateProtectionTimesCalled int
		DeleteProtectionTimesCalled int
	}{
		{
			Name:                    "No operations, already protected",
			ShieldAvailableResponse: true,
			ShieldAvailableError:    nil,
			GetProtectionResponse: &shield.Protection{
				Id:          aws.String("ProtectionId"),
				Name:        aws.String(protectionName),
				ResourceArn: aws.String("arn:lb"),
			},
			GetProtectionError:          nil,
			Expected:                    nil,
			LoadBalancerARN:             "arn:lb",
			IngressAnnocation:           buildIngress("true"),
			ShieldAvailableTimesCalled:  1,
			CreateProtectionTimesCalled: 0,
			DeleteProtectionTimesCalled: 0,
		},
		{
			Name:                        "No operations, nothing to protect",
			ShieldAvailableResponse:     true,
			ShieldAvailableError:        nil,
			GetProtectionResponse:       nil,
			GetProtectionError:          nil,
			Expected:                    nil,
			LoadBalancerARN:             "arn:lb",
			IngressAnnocation:           buildIngress(""),
			ShieldAvailableTimesCalled:  0,
			CreateProtectionTimesCalled: 0,
			DeleteProtectionTimesCalled: 0,
		},
		{
			Name:                    "No operations, protection externally managed",
			ShieldAvailableResponse: true,
			ShieldAvailableError:    nil,
			GetProtectionResponse: &shield.Protection{
				Id:          aws.String("ProtectionId"),
				Name:        aws.String("protection enabled manually"),
				ResourceArn: aws.String("arn:lb"),
			},
			GetProtectionError:          nil,
			Expected:                    nil,
			LoadBalancerARN:             "arn:lb",
			IngressAnnocation:           buildIngress("false"),
			ShieldAvailableTimesCalled:  1,
			CreateProtectionTimesCalled: 0,
			DeleteProtectionTimesCalled: 0,
		},
		{
			Name:                        "Protection enabled, no active subscription",
			ShieldAvailableResponse:     false,
			ShieldAvailableError:        nil,
			GetProtectionResponse:       nil,
			GetProtectionError:          nil,
			Expected:                    errors.New("unable to create shield advanced protection for loadBalancer arn:lb, shield advanced subscription is not active"),
			LoadBalancerARN:             "arn:lb",
			IngressAnnocation:           buildIngress("true"),
			ShieldAvailableTimesCalled:  1,
			CreateProtectionTimesCalled: 0,
			DeleteProtectionTimesCalled: 0,
		},
		{
			Name:                        "Enable protection",
			ShieldAvailableResponse:     true,
			ShieldAvailableError:        nil,
			GetProtectionResponse:       nil,
			GetProtectionError:          nil,
			Expected:                    nil,
			LoadBalancerARN:             "arn:lb",
			IngressAnnocation:           buildIngress("true"),
			ShieldAvailableTimesCalled:  1,
			CreateProtectionTimesCalled: 1,
			DeleteProtectionTimesCalled: 0,
		},
		{
			Name:                    "Disable protection",
			ShieldAvailableResponse: true,
			ShieldAvailableError:    nil,
			GetProtectionResponse: &shield.Protection{
				Id:          aws.String("ProtectionId"),
				Name:        aws.String(protectionName),
				ResourceArn: aws.String("arn:lb"),
			},
			GetProtectionError:          nil,
			Expected:                    nil,
			LoadBalancerARN:             "arn:lb",
			IngressAnnocation:           buildIngress("false"),
			ShieldAvailableTimesCalled:  1,
			CreateProtectionTimesCalled: 0,
			DeleteProtectionTimesCalled: 1,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}

			cloud.On("ShieldAvailable", ctx).Return(tc.ShieldAvailableResponse, tc.ShieldAvailableError)
			cloud.On("GetProtection", ctx, aws.String(tc.LoadBalancerARN)).Return(tc.GetProtectionResponse, tc.GetProtectionError)
			cloud.On("DeleteProtection", ctx, aws.String("ProtectionId")).Return(tc.DeleteProtectionResponse, tc.DeleteProtectionError)
			cloud.On("CreateProtection", ctx, aws.String(tc.LoadBalancerARN), aws.String(protectionName)).Return(tc.CreateProtectionResponse, tc.CreateProtectionError)

			controller := NewShieldController(cloud)
			err := controller.Reconcile(ctx, tc.LoadBalancerARN, tc.IngressAnnocation)
			assert.Equal(t, tc.Expected, err)
			cloud.AssertNumberOfCalls(t, "ShieldAvailable", tc.ShieldAvailableTimesCalled)
			cloud.AssertNumberOfCalls(t, "CreateProtection", tc.CreateProtectionTimesCalled)
			cloud.AssertNumberOfCalls(t, "DeleteProtection", tc.DeleteProtectionTimesCalled)
		})
	}
}
