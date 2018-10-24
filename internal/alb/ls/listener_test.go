package ls

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/rs"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/listener"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
)

type CreateListenerCall struct {
	Input    elbv2.CreateListenerInput
	Instance *elbv2.Listener
	Err      error
}

type ModifyListenerCall struct {
	Input    elbv2.ModifyListenerInput
	Instance *elbv2.Listener
	Err      error
}

type RulesReconcileCall struct {
	Instance *elbv2.Listener
	Err      error
}

func TestDefaultController_Reconcile(t *testing.T) {
	LBArn := "MyLBArn"
	for _, tc := range []struct {
		Name         string
		Ingress      extensions.Ingress
		IngressAnnos annotations.Ingress
		Port         loadbalancer.PortData
		TGGroup      tg.TargetGroupGroup
		Instance     *elbv2.Listener

		CreateListenerCall *CreateListenerCall
		ModifyListenerCall *ModifyListenerCall
		RulesReconcileCall *RulesReconcileCall
		ExpectedError      error
	}{
		{
			Name: "Reconcile succeed by creating http listener for default backend",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
			Port: loadbalancer.PortData{
				Port:   80,
				Scheme: elbv2.ProtocolEnumHttp,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					}: {
						Arn: "tgArn",
					},
				},
			},

			CreateListenerCall: &CreateListenerCall{
				Input: elbv2.CreateListenerInput{
					LoadBalancerArn: aws.String(LBArn),
					Certificates:    nil,
					SslPolicy:       nil,
					Protocol:        aws.String(elbv2.ProtocolEnumHttp),
					Port:            aws.Int64(80),
					DefaultActions: []*elbv2.Action{
						{
							TargetGroupArn: aws.String("tgArn"),
							Type:           aws.String(elbv2.ActionTypeEnumForward),
						},
					},
				},
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
			},

			RulesReconcileCall: &RulesReconcileCall{
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
			},
		},
		{
			Name: "Reconcile succeed by creating http listener for 404 backend",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{},
			},
			IngressAnnos: annotations.Ingress{},
			Port: loadbalancer.PortData{
				Port:   80,
				Scheme: elbv2.ProtocolEnumHttp,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{},
			},

			CreateListenerCall: &CreateListenerCall{
				Input: elbv2.CreateListenerInput{
					LoadBalancerArn: aws.String(LBArn),
					Certificates:    nil,
					SslPolicy:       nil,
					Protocol:        aws.String(elbv2.ProtocolEnumHttp),
					Port:            aws.Int64(80),
					DefaultActions: []*elbv2.Action{
						{
							FixedResponseConfig: &elbv2.FixedResponseActionConfig{
								ContentType: aws.String("text/plain"),
								StatusCode:  aws.String("404"),
							},
							Type: aws.String(elbv2.ActionTypeEnumFixedResponse),
						},
					},
				},
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
			},

			RulesReconcileCall: &RulesReconcileCall{
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
			},
		},
		{
			Name: "Reconcile succeed by creating https listener for default backend",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{
				Listener: &listener.Config{
					CertificateArn: aws.String("certificateArn"),
					SslPolicy:      aws.String("sslPolicy"),
				},
			},
			Port: loadbalancer.PortData{
				Port:   443,
				Scheme: elbv2.ProtocolEnumHttps,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					}: {
						Arn: "tgArn",
					},
				},
			},
			CreateListenerCall: &CreateListenerCall{
				Input: elbv2.CreateListenerInput{
					LoadBalancerArn: aws.String(LBArn),
					Certificates: []*elbv2.Certificate{
						{
							CertificateArn: aws.String("certificateArn"),
							IsDefault:      aws.Bool(true),
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					Protocol:  aws.String(elbv2.ProtocolEnumHttps),
					Port:      aws.Int64(443),
					DefaultActions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: aws.String("tgArn"),
						},
					},
				},
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
			},
			RulesReconcileCall: &RulesReconcileCall{
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
			},
		},
		{
			Name: "Reconcile succeed reconcile non-modified existing instance",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{
				Listener: &listener.Config{
					CertificateArn: aws.String("certificateArn"),
					SslPolicy:      aws.String("sslPolicy"),
				},
			},
			Port: loadbalancer.PortData{
				Port:   443,
				Scheme: elbv2.ProtocolEnumHttps,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					}: {
						Arn: "tgArn",
					},
				},
			},

			Instance: &elbv2.Listener{
				ListenerArn: aws.String("lsArn"),
				Port:        aws.Int64(443),
				Protocol:    aws.String(elbv2.ProtocolEnumHttps),
				Certificates: []*elbv2.Certificate{
					{
						CertificateArn: aws.String("certificateArn"),
						IsDefault:      aws.Bool(true),
					},
				},
				SslPolicy: aws.String("sslPolicy"),
				DefaultActions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: aws.String("tgArn"),
					},
				},
			},

			RulesReconcileCall: &RulesReconcileCall{
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
					Port:        aws.Int64(443),
					Protocol:    aws.String(elbv2.ProtocolEnumHttps),
					Certificates: []*elbv2.Certificate{
						{
							CertificateArn: aws.String("certificateArn"),
							IsDefault:      aws.Bool(true),
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: aws.String("tgArn"),
						},
					},
				},
			},
		},
		{
			Name: "Reconcile succeed reconcile modified existing instance",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{
				Listener: &listener.Config{
					CertificateArn: aws.String("certificateArn"),
					SslPolicy:      aws.String("sslPolicy"),
				},
			},
			Port: loadbalancer.PortData{
				Port:   443,
				Scheme: elbv2.ProtocolEnumHttps,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					}: {
						Arn: "tgArn",
					},
				},
			},

			Instance: &elbv2.Listener{
				ListenerArn:  aws.String("lsArn"),
				Port:         aws.Int64(80),
				Protocol:     aws.String(elbv2.ProtocolEnumHttp),
				Certificates: nil,
				SslPolicy:    nil,
				DefaultActions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: aws.String("tgArn2"),
					},
				},
			},

			ModifyListenerCall: &ModifyListenerCall{
				Input: elbv2.ModifyListenerInput{
					ListenerArn: aws.String("lsArn"),
					Port:        aws.Int64(443),
					Protocol:    aws.String(elbv2.ProtocolEnumHttps),
					Certificates: []*elbv2.Certificate{
						{
							CertificateArn: aws.String("certificateArn"),
							IsDefault:      aws.Bool(true),
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: aws.String("tgArn"),
						},
					},
				},
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
					Port:        aws.Int64(443),
					Protocol:    aws.String(elbv2.ProtocolEnumHttps),
					Certificates: []*elbv2.Certificate{
						{
							CertificateArn: aws.String("certificateArn"),
							IsDefault:      aws.Bool(true),
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: aws.String("tgArn"),
						},
					},
				},
			},

			RulesReconcileCall: &RulesReconcileCall{
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
					Port:        aws.Int64(443),
					Protocol:    aws.String(elbv2.ProtocolEnumHttps),
					Certificates: []*elbv2.Certificate{
						{
							CertificateArn: aws.String("certificateArn"),
							IsDefault:      aws.Bool(true),
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: aws.String("tgArn"),
						},
					},
				},
			},
		},
		{
			Name: "Reconcile failed when modify existing instance",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{
				Listener: &listener.Config{
					CertificateArn: aws.String("certificateArn"),
					SslPolicy:      aws.String("sslPolicy"),
				},
			},
			Port: loadbalancer.PortData{
				Port:   443,
				Scheme: elbv2.ProtocolEnumHttps,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					}: {
						Arn: "tgArn",
					},
				},
			},

			Instance: &elbv2.Listener{
				ListenerArn:  aws.String("lsArn"),
				Port:         aws.Int64(80),
				Protocol:     aws.String(elbv2.ProtocolEnumHttp),
				Certificates: nil,
				SslPolicy:    nil,
				DefaultActions: []*elbv2.Action{
					{
						Type:           aws.String(elbv2.ActionTypeEnumForward),
						TargetGroupArn: aws.String("tgArn2"),
					},
				},
			},

			ModifyListenerCall: &ModifyListenerCall{
				Input: elbv2.ModifyListenerInput{
					ListenerArn: aws.String("lsArn"),
					Port:        aws.Int64(443),
					Protocol:    aws.String(elbv2.ProtocolEnumHttps),
					Certificates: []*elbv2.Certificate{
						{
							CertificateArn: aws.String("certificateArn"),
							IsDefault:      aws.Bool(true),
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: aws.String("tgArn"),
						},
					},
				},
				Err: errors.New("ModifyListenerCall"),
			},
			ExpectedError: errors.New("failed to reconcile listener due to ModifyListenerCall"),
		},
		{
			Name: "Reconcile failed when finding action by annotation",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "serviceByAnnotation",
						ServicePort: intstr.FromString(action.UseActionAnnotation),
					},
				},
			},
			IngressAnnos: annotations.Ingress{
				Action: &action.Config{},
			},
			Port: loadbalancer.PortData{
				Port:   80,
				Scheme: elbv2.ProtocolEnumHttp,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{},
			},
			ExpectedError: errors.New("failed to build listener config due to backend with `servicePort: use-annotation` was configured with `serviceName: serviceByAnnotation` but an action annotation for serviceByAnnotation is not set"),
		},
		{
			Name: "Reconcile failed when find targetGroup backend",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
			Port: loadbalancer.PortData{
				Port:   80,
				Scheme: elbv2.ProtocolEnumHttp,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{},
			},
			ExpectedError: errors.New("failed to build listener config due to unable to find targetGroup for backend service:8080"),
		},
		{
			Name: "Reconcile failed when creating ingress",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
			Port: loadbalancer.PortData{
				Port:   80,
				Scheme: elbv2.ProtocolEnumHttp,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					}: {
						Arn: "tgArn",
					},
				},
			},

			CreateListenerCall: &CreateListenerCall{
				Input: elbv2.CreateListenerInput{
					LoadBalancerArn: aws.String(LBArn),
					Certificates:    nil,
					SslPolicy:       nil,
					Protocol:        aws.String(elbv2.ProtocolEnumHttp),
					Port:            aws.Int64(80),
					DefaultActions: []*elbv2.Action{
						{
							TargetGroupArn: aws.String("tgArn"),
							Type:           aws.String(elbv2.ActionTypeEnumForward),
						},
					},
				},
				Err: errors.New("CreateListenerCall"),
			},

			ExpectedError: errors.New("failed to create listener due to CreateListenerCall"),
		},
		{
			Name: "Reconcile failed when reconcile rules",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
			Port: loadbalancer.PortData{
				Port:   80,
				Scheme: elbv2.ProtocolEnumHttp,
			},
			TGGroup: tg.TargetGroupGroup{
				TGByBackend: map[extensions.IngressBackend]tg.TargetGroup{
					{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8080),
					}: {
						Arn: "tgArn",
					},
				},
			},

			CreateListenerCall: &CreateListenerCall{
				Input: elbv2.CreateListenerInput{
					LoadBalancerArn: aws.String(LBArn),
					Certificates:    nil,
					SslPolicy:       nil,
					Protocol:        aws.String(elbv2.ProtocolEnumHttp),
					Port:            aws.Int64(80),
					DefaultActions: []*elbv2.Action{
						{
							TargetGroupArn: aws.String("tgArn"),
							Type:           aws.String(elbv2.ActionTypeEnumForward),
						},
					},
				},
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
			},

			RulesReconcileCall: &RulesReconcileCall{
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
				Err: errors.New("RulesReconcileCall"),
			},
			ExpectedError: errors.New("failed to reconcile rules due to RulesReconcileCall"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			mockELBV2 := &mocks.ELBV2API{}
			if tc.CreateListenerCall != nil {
				mockELBV2.On("CreateListener", &tc.CreateListenerCall.Input).Return(
					&elbv2.CreateListenerOutput{
						Listeners: []*elbv2.Listener{tc.CreateListenerCall.Instance},
					}, tc.CreateListenerCall.Err)
			}
			if tc.ModifyListenerCall != nil {
				mockELBV2.On("ModifyListener", &tc.ModifyListenerCall.Input).Return(
					&elbv2.ModifyListenerOutput{
						Listeners: []*elbv2.Listener{tc.ModifyListenerCall.Instance},
					}, tc.ModifyListenerCall.Err)
			}

			mockStore := &store.MockStorer{}
			mockRulesController := &rs.MockController{}
			if tc.RulesReconcileCall != nil {
				mockRulesController.On("Reconcile", mock.Anything, tc.RulesReconcileCall.Instance, &tc.Ingress, &tc.IngressAnnos, tc.TGGroup).Return(tc.RulesReconcileCall.Err)
			}

			controller := &defaultController{
				elbv2:           mockELBV2,
				store:           mockStore,
				rulesController: mockRulesController,
			}
			err := controller.Reconcile(context.Background(), ReconcileOptions{
				LBArn:        LBArn,
				Ingress:      &tc.Ingress,
				IngressAnnos: &tc.IngressAnnos,
				Port:         tc.Port,
				TGGroup:      tc.TGGroup,
				Instance:     tc.Instance,
			})
			assert.Equal(t, tc.ExpectedError, err)
			mockELBV2.AssertExpectations(t)
			mockStore.AssertExpectations(t)
			mockRulesController.AssertExpectations(t)
		})
	}
}
