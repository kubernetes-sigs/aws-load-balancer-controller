package ls

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GetIngressAnnotationsCall struct {
	Key          string
	IngressAnnos *annotations.Ingress
	Err          error
}

type ListListenersByLoadBalancerCall struct {
	Listeners []*elbv2.Listener
	Err       error
}

type LSControllerReconcileCall struct {
	Port     loadbalancer.PortData
	Instance *elbv2.Listener
	Err      error
}

type DeleteListenersByArnCall struct {
	LSArn string
	Err   error
}

func TestDefaultGroupController_Reconcile(t *testing.T) {
	lbArn := "lbArn"
	ingress := extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "ingress",
		},
	}
	targetGroup := tg.TargetGroupGroup{}
	for _, tc := range []struct {
		Name                            string
		GetIngressAnnotationsCall       *GetIngressAnnotationsCall
		ListListenersByLoadBalancerCall *ListListenersByLoadBalancerCall
		LSControllerReconcileCalls      []LSControllerReconcileCall
		DeleteListenersByArnCalls       []DeleteListenersByArnCall
		ExpectedErr                     error
	}{
		{
			Name: "Reconcile succeed by creating listeners",
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key: "namespace/ingress",
				IngressAnnos: &annotations.Ingress{
					LoadBalancer: &loadbalancer.Config{
						Ports: []loadbalancer.PortData{
							{
								Port:   80,
								Scheme: elbv2.ProtocolEnumHttp,
							},
							{
								Port:   443,
								Scheme: elbv2.ProtocolEnumHttps,
							},
						},
					},
				},
			},
			ListListenersByLoadBalancerCall: &ListListenersByLoadBalancerCall{
				Listeners: nil,
			},
			LSControllerReconcileCalls: []LSControllerReconcileCall{
				{
					Port: loadbalancer.PortData{
						Port:   80,
						Scheme: elbv2.ProtocolEnumHttp,
					},
					Instance: nil,
				},
				{
					Port: loadbalancer.PortData{
						Port:   443,
						Scheme: elbv2.ProtocolEnumHttps,
					},
					Instance: nil,
				},
			},
		},
		{
			Name: "Reconcile succeed by modify listeners",
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key: "namespace/ingress",
				IngressAnnos: &annotations.Ingress{
					LoadBalancer: &loadbalancer.Config{
						Ports: []loadbalancer.PortData{
							{
								Port:   80,
								Scheme: elbv2.ProtocolEnumHttp,
							},
							{
								Port:   443,
								Scheme: elbv2.ProtocolEnumHttps,
							},
						},
					},
				},
			},
			ListListenersByLoadBalancerCall: &ListListenersByLoadBalancerCall{
				Listeners: []*elbv2.Listener{
					{
						ListenerArn: aws.String("lsArn1"),
						Port:        aws.Int64(80),
					},
					{
						ListenerArn: aws.String("lsArn2"),
						Port:        aws.Int64(443),
					},
				},
			},
			LSControllerReconcileCalls: []LSControllerReconcileCall{
				{
					Port: loadbalancer.PortData{
						Port:   80,
						Scheme: elbv2.ProtocolEnumHttp,
					},
					Instance: &elbv2.Listener{
						ListenerArn: aws.String("lsArn1"),
						Port:        aws.Int64(80),
					},
				},
				{
					Port: loadbalancer.PortData{
						Port:   443,
						Scheme: elbv2.ProtocolEnumHttps,
					},
					Instance: &elbv2.Listener{
						ListenerArn: aws.String("lsArn2"),
						Port:        aws.Int64(443),
					},
				},
			},
		},
		{
			Name: "Reconcile succeed by create|delete|modify listeners",
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key: "namespace/ingress",
				IngressAnnos: &annotations.Ingress{
					LoadBalancer: &loadbalancer.Config{
						Ports: []loadbalancer.PortData{
							{
								Port:   80,
								Scheme: elbv2.ProtocolEnumHttp,
							},
							{
								Port:   8080,
								Scheme: elbv2.ProtocolEnumHttp,
							},
						},
					},
				},
			},
			ListListenersByLoadBalancerCall: &ListListenersByLoadBalancerCall{
				Listeners: []*elbv2.Listener{
					{
						ListenerArn: aws.String("lsArn1"),
						Port:        aws.Int64(80),
					},
					{
						ListenerArn: aws.String("lsArn2"),
						Port:        aws.Int64(443),
					},
				},
			},
			LSControllerReconcileCalls: []LSControllerReconcileCall{
				{
					Port: loadbalancer.PortData{
						Port:   80,
						Scheme: elbv2.ProtocolEnumHttp,
					},
					Instance: &elbv2.Listener{
						ListenerArn: aws.String("lsArn1"),
						Port:        aws.Int64(80),
					},
				},
				{
					Port: loadbalancer.PortData{
						Port:   8080,
						Scheme: elbv2.ProtocolEnumHttp,
					},
					Instance: nil,
				},
			},
			DeleteListenersByArnCalls: []DeleteListenersByArnCall{
				{
					LSArn: "lsArn2",
				},
			},
		},
		{
			Name: "Reconcile failed when get ingress annotations",
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key: "namespace/ingress",
				Err: errors.New("GetIngressAnnotationsCall"),
			},
			ExpectedErr: errors.New("GetIngressAnnotationsCall"),
		},
		{
			Name: "Reconcile failed when get listeners",
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key: "namespace/ingress",
				IngressAnnos: &annotations.Ingress{
					LoadBalancer: &loadbalancer.Config{
						Ports: []loadbalancer.PortData{
							{
								Port:   80,
								Scheme: elbv2.ProtocolEnumHttp,
							},
							{
								Port:   443,
								Scheme: elbv2.ProtocolEnumHttps,
							},
						},
					},
				},
			},
			ListListenersByLoadBalancerCall: &ListListenersByLoadBalancerCall{
				Err: errors.New("ListListenersByLoadBalancerCall"),
			},
			ExpectedErr: errors.New("ListListenersByLoadBalancerCall"),
		},
		{
			Name: "Reconcile failed when reconcile listener",
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key: "namespace/ingress",
				IngressAnnos: &annotations.Ingress{
					LoadBalancer: &loadbalancer.Config{
						Ports: []loadbalancer.PortData{
							{
								Port:   80,
								Scheme: elbv2.ProtocolEnumHttp,
							},
						},
					},
				},
			},
			ListListenersByLoadBalancerCall: &ListListenersByLoadBalancerCall{
				Listeners: nil,
			},
			LSControllerReconcileCalls: []LSControllerReconcileCall{
				{
					Port: loadbalancer.PortData{
						Port:   80,
						Scheme: elbv2.ProtocolEnumHttp,
					},
					Instance: nil,
					Err:      errors.New("LSControllerReconcileCalls"),
				},
			},
			ExpectedErr: errors.New("LSControllerReconcileCalls"),
		},
		{
			Name: "Reconcile failed when deleting listener",
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key: "namespace/ingress",
				IngressAnnos: &annotations.Ingress{
					LoadBalancer: &loadbalancer.Config{
						Ports: []loadbalancer.PortData{
							{
								Port:   80,
								Scheme: elbv2.ProtocolEnumHttp,
							},
						},
					},
				},
			},
			ListListenersByLoadBalancerCall: &ListListenersByLoadBalancerCall{
				Listeners: []*elbv2.Listener{
					{
						ListenerArn: aws.String("lsArn1"),
						Port:        aws.Int64(80),
					},
					{
						ListenerArn: aws.String("lsArn2"),
						Port:        aws.Int64(443),
					},
				},
			},
			LSControllerReconcileCalls: []LSControllerReconcileCall{
				{
					Port: loadbalancer.PortData{
						Port:   80,
						Scheme: elbv2.ProtocolEnumHttp,
					},
					Instance: &elbv2.Listener{
						ListenerArn: aws.String("lsArn1"),
						Port:        aws.Int64(80),
					},
				},
			},
			DeleteListenersByArnCalls: []DeleteListenersByArnCall{
				{
					LSArn: "lsArn2",
					Err:   errors.New("DeleteListenersByArnCall"),
				},
			},
			ExpectedErr: errors.New("DeleteListenersByArnCall"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.ListListenersByLoadBalancerCall != nil {
				cloud.On("ListListenersByLoadBalancer", ctx, lbArn).Return(tc.ListListenersByLoadBalancerCall.Listeners, tc.ListListenersByLoadBalancerCall.Err)
			}
			for _, call := range tc.DeleteListenersByArnCalls {
				cloud.On("DeleteListenersByArn", call.LSArn).Return(call.Err)
			}

			mockStore := &store.MockStorer{}
			if tc.GetIngressAnnotationsCall != nil {
				mockStore.On("GetIngressAnnotations", tc.GetIngressAnnotationsCall.Key).Return(tc.GetIngressAnnotationsCall.IngressAnnos, tc.GetIngressAnnotationsCall.Err)
			}
			mockLSController := &MockController{}
			for _, call := range tc.LSControllerReconcileCalls {
				mockLSController.On("Reconcile", mock.Anything, ReconcileOptions{
					LBArn:        lbArn,
					Ingress:      &ingress,
					IngressAnnos: tc.GetIngressAnnotationsCall.IngressAnnos,
					TGGroup:      targetGroup,
					Port:         call.Port,
					Instance:     call.Instance,
				}).Return(call.Err)
			}

			controller := &defaultGroupController{
				cloud:        cloud,
				store:        mockStore,
				lsController: mockLSController,
			}

			err := controller.Reconcile(context.Background(), lbArn, &ingress, targetGroup)
			assert.Equal(t, tc.ExpectedErr, err)
			cloud.AssertExpectations(t)
			mockStore.AssertExpectations(t)
			mockLSController.AssertExpectations(t)
		})
	}
}

func TestDefaultGroupController_Delete(t *testing.T) {
	lbArn := "lbArn"
	for _, tc := range []struct {
		Name                            string
		ListListenersByLoadBalancerCall *ListListenersByLoadBalancerCall
		DeleteListenersByArnCalls       []DeleteListenersByArnCall
		ExpectedErr                     error
	}{
		{
			Name: "Delete succeed",
			ListListenersByLoadBalancerCall: &ListListenersByLoadBalancerCall{
				Listeners: []*elbv2.Listener{
					{
						ListenerArn: aws.String("lsArn1"),
						Port:        aws.Int64(80),
					},
					{
						ListenerArn: aws.String("lsArn2"),
						Port:        aws.Int64(443),
					},
				},
			},
			DeleteListenersByArnCalls: []DeleteListenersByArnCall{
				{
					LSArn: "lsArn1",
				},
				{
					LSArn: "lsArn2",
				},
			},
		},
		{
			Name: "Delete failed when deleting listener",
			ListListenersByLoadBalancerCall: &ListListenersByLoadBalancerCall{
				Listeners: []*elbv2.Listener{
					{
						ListenerArn: aws.String("lsArn1"),
						Port:        aws.Int64(80),
					},
				},
			},
			DeleteListenersByArnCalls: []DeleteListenersByArnCall{
				{
					LSArn: "lsArn1",
					Err:   errors.New("DeleteListenersByArnCall"),
				},
			},
			ExpectedErr: errors.New("DeleteListenersByArnCall"),
		},
	} {
		ctx := context.Background()
		cloud := &mocks.CloudAPI{}
		if tc.ListListenersByLoadBalancerCall != nil {
			cloud.On("ListListenersByLoadBalancer", ctx, lbArn).Return(tc.ListListenersByLoadBalancerCall.Listeners, tc.ListListenersByLoadBalancerCall.Err)
		}
		for _, call := range tc.DeleteListenersByArnCalls {
			cloud.On("DeleteListenersByArn", call.LSArn).Return(call.Err)
		}

		mockStore := &store.MockStorer{}
		mockLSController := &MockController{}
		controller := &defaultGroupController{
			cloud:        cloud,
			store:        mockStore,
			lsController: mockLSController,
		}

		err := controller.Delete(context.Background(), lbArn)
		assert.Equal(t, tc.ExpectedErr, err)
		cloud.AssertExpectations(t)
		mockStore.AssertExpectations(t)
		mockLSController.AssertExpectations(t)
	}
}
