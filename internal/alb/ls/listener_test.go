package ls

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/auth"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	mock_auth "github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks/aws-alb-ingress-controller/ingress/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

type DescribeListenerCertificatesCall struct {
	LSArn        string
	Certificates []*elbv2.Certificate
	Err          error
}

type AddListenerCertificatesCall struct {
	Input *elbv2.AddListenerCertificatesInput
	Err   error
}

type RemoveListenerCertificatesCall struct {
	Input *elbv2.RemoveListenerCertificatesInput
	Err   error
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
		AuthConfig   auth.Config

		CreateListenerCall               *CreateListenerCall
		ModifyListenerCall               *ModifyListenerCall
		DescribeListenerCertificatesCall *DescribeListenerCertificatesCall
		AddListenerCertificatesCalls     []AddListenerCertificatesCall
		RemoveListenerCertificatesCalls  []RemoveListenerCertificatesCall

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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
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
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
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
							Order: aws.Int64(1),
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
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/ssl-policy":      "sslPolicy",
						"alb.ingress.kubernetes.io/certificate-arn": "certificateArn",
					},
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
			},

			CreateListenerCall: &CreateListenerCall{
				Input: elbv2.CreateListenerInput{
					LoadBalancerArn: aws.String(LBArn),
					Certificates: []*elbv2.Certificate{
						{
							CertificateArn: aws.String("certificateArn"),
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					Protocol:  aws.String(elbv2.ProtocolEnumHttps),
					Port:      aws.Int64(443),
					DefaultActions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
						},
					},
				},
				Instance: &elbv2.Listener{
					ListenerArn: aws.String("lsArn"),
				},
			},
			DescribeListenerCertificatesCall: &DescribeListenerCertificatesCall{
				LSArn: "lsArn",
				Certificates: []*elbv2.Certificate{
					{
						CertificateArn: aws.String("certificateArn"),
						IsDefault:      aws.Bool(true),
					},
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
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/ssl-policy":      "sslPolicy",
						"alb.ingress.kubernetes.io/certificate-arn": "certificateArn",
					},
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
			},

			Instance: &elbv2.Listener{
				ListenerArn: aws.String("lsArn"),
				Port:        aws.Int64(443),
				Protocol:    aws.String(elbv2.ProtocolEnumHttps),
				Certificates: []*elbv2.Certificate{
					{
						CertificateArn: aws.String("certificateArn"),
					},
				},
				SslPolicy: aws.String("sslPolicy"),
				DefaultActions: []*elbv2.Action{
					{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{TargetGroupArn: aws.String("tgArn"),
									Weight: aws.Int64(1),
								},
							},
						},
					},
				},
			},
			DescribeListenerCertificatesCall: &DescribeListenerCertificatesCall{
				LSArn: "lsArn",
				Certificates: []*elbv2.Certificate{
					{
						CertificateArn: aws.String("certificateArn"),
						IsDefault:      aws.Bool(true),
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
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{TargetGroupArn: aws.String("tgArn"),
										Weight: aws.Int64(1),
									},
								},
							},
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
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/ssl-policy":      "sslPolicy",
						"alb.ingress.kubernetes.io/certificate-arn": "certificateArn",
					},
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
			},

			Instance: &elbv2.Listener{
				ListenerArn:  aws.String("lsArn"),
				Port:         aws.Int64(80),
				Protocol:     aws.String(elbv2.ProtocolEnumHttp),
				Certificates: nil,
				SslPolicy:    nil,
				DefaultActions: []*elbv2.Action{
					{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: aws.String("tgArn2"),
									Weight:         aws.Int64(1),
								},
							},
						},
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
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{
										TargetGroupArn: aws.String("tgArn"),
										Weight:         aws.Int64(1),
									},
								},
							},
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
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{
										TargetGroupArn: aws.String("tgArn"),
										Weight:         aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
			DescribeListenerCertificatesCall: &DescribeListenerCertificatesCall{
				LSArn: "lsArn",
				Certificates: []*elbv2.Certificate{
					{
						CertificateArn: aws.String("certificateArn"),
						IsDefault:      aws.Bool(true),
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
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{
										TargetGroupArn: aws.String("tgArn"),
										Weight:         aws.Int64(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "Reconcile succeed by modify extra certificates",
			Ingress: extensions.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "namespace",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/ssl-policy":      "sslPolicy",
						"alb.ingress.kubernetes.io/certificate-arn": "certificateArn,certificateArn4,certificateArn5",
					},
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
			},

			Instance: &elbv2.Listener{
				ListenerArn: aws.String("lsArn"),
				Port:        aws.Int64(443),
				Protocol:    aws.String(elbv2.ProtocolEnumHttps),
				Certificates: []*elbv2.Certificate{
					{
						CertificateArn: aws.String("certificateArn"),
					},
				},
				SslPolicy: aws.String("sslPolicy"),
				DefaultActions: []*elbv2.Action{
					{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: aws.String("tgArn"),
									Weight:         aws.Int64(1),
								},
							},
						},
					},
				},
			},
			DescribeListenerCertificatesCall: &DescribeListenerCertificatesCall{
				LSArn: "lsArn",
				Certificates: []*elbv2.Certificate{
					{
						CertificateArn: aws.String("certificateArn"),
						IsDefault:      aws.Bool(true),
					},
					{
						CertificateArn: aws.String("certificateArn2"),
						IsDefault:      aws.Bool(false),
					},
					{
						CertificateArn: aws.String("certificateArn3"),
						IsDefault:      aws.Bool(false),
					},
				},
			},
			AddListenerCertificatesCalls: []AddListenerCertificatesCall{
				{
					Input: &elbv2.AddListenerCertificatesInput{
						ListenerArn: aws.String("lsArn"),
						Certificates: []*elbv2.Certificate{
							{
								CertificateArn: aws.String("certificateArn4"),
							},
						},
					},
				},
				{
					Input: &elbv2.AddListenerCertificatesInput{
						ListenerArn: aws.String("lsArn"),
						Certificates: []*elbv2.Certificate{
							{
								CertificateArn: aws.String("certificateArn5"),
							},
						},
					},
				},
			},
			RemoveListenerCertificatesCalls: []RemoveListenerCertificatesCall{
				{
					Input: &elbv2.RemoveListenerCertificatesInput{
						ListenerArn: aws.String("lsArn"),
						Certificates: []*elbv2.Certificate{
							{
								CertificateArn: aws.String("certificateArn2"),
							},
						},
					},
				},
				{
					Input: &elbv2.RemoveListenerCertificatesInput{
						ListenerArn: aws.String("lsArn"),
						Certificates: []*elbv2.Certificate{
							{
								CertificateArn: aws.String("certificateArn3"),
							},
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
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{
										TargetGroupArn: aws.String("tgArn"),
										Weight:         aws.Int64(1),
									},
								},
							},
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
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/ssl-policy":      "sslPolicy",
						"alb.ingress.kubernetes.io/certificate-arn": "certificateArn",
					},
				},
				Spec: extensions.IngressSpec{
					Backend: &extensions.IngressBackend{
						ServiceName: "service",
						ServicePort: intstr.FromInt(8443),
					},
				},
			},
			IngressAnnos: annotations.Ingress{},
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
			},

			Instance: &elbv2.Listener{
				ListenerArn:  aws.String("lsArn"),
				Port:         aws.Int64(80),
				Protocol:     aws.String(elbv2.ProtocolEnumHttp),
				Certificates: nil,
				SslPolicy:    nil,
				DefaultActions: []*elbv2.Action{
					{
						Order: aws.Int64(1),
						Type:  aws.String(elbv2.ActionTypeEnumForward),
						ForwardConfig: &elbv2.ForwardActionConfig{
							TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
								Enabled: aws.Bool(false),
							},
							TargetGroups: []*elbv2.TargetGroupTuple{
								{
									TargetGroupArn: aws.String("tgArn2"),
									Weight:         aws.Int64(1),
								},
							},
						},
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
						},
					},
					SslPolicy: aws.String("sslPolicy"),
					DefaultActions: []*elbv2.Action{
						{
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{
										TargetGroupArn: aws.String("tgArn"),
										Weight:         aws.Int64(1),
									},
								},
							},
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
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
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{
										TargetGroupArn: aws.String("tgArn"),
										Weight:         aws.Int64(1),
									},
								},
							},
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
			AuthConfig: auth.Config{
				Type: auth.TypeNone,
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
							Order: aws.Int64(1),
							Type:  aws.String(elbv2.ActionTypeEnumForward),
							ForwardConfig: &elbv2.ForwardActionConfig{
								TargetGroupStickinessConfig: &elbv2.TargetGroupStickinessConfig{
									Enabled: aws.Bool(false),
								},
								TargetGroups: []*elbv2.TargetGroupTuple{
									{
										TargetGroupArn: aws.String("tgArn"),
										Weight:         aws.Int64(1),
									},
								},
							},
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
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.CreateListenerCall != nil {
				cloud.On("CreateListenerWithContext", ctx, &tc.CreateListenerCall.Input).Return(
					&elbv2.CreateListenerOutput{
						Listeners: []*elbv2.Listener{tc.CreateListenerCall.Instance},
					}, tc.CreateListenerCall.Err)
			}
			if tc.ModifyListenerCall != nil {
				cloud.On("ModifyListenerWithContext", ctx, &tc.ModifyListenerCall.Input).Return(
					&elbv2.ModifyListenerOutput{
						Listeners: []*elbv2.Listener{tc.ModifyListenerCall.Instance},
					}, tc.ModifyListenerCall.Err)
			}
			if tc.DescribeListenerCertificatesCall != nil {
				cloud.On("DescribeListenerCertificates", ctx, tc.DescribeListenerCertificatesCall.LSArn).Return(
					tc.DescribeListenerCertificatesCall.Certificates, tc.DescribeListenerCertificatesCall.Err)
			}
			for _, call := range tc.AddListenerCertificatesCalls {
				cloud.On("AddListenerCertificates", ctx, call.Input).Return(nil, call.Err)
			}
			for _, call := range tc.RemoveListenerCertificatesCalls {
				cloud.On("RemoveListenerCertificates", ctx, call.Input).Return(nil, call.Err)
			}
			mockAuthModule := mock_auth.NewMockModule(ctrl)
			mockAuthModule.EXPECT().NewConfig(gomock.Any(), &tc.Ingress, gomock.Any(), gomock.Any()).Return(tc.AuthConfig, nil)

			mockRulesController := &MockRulesController{}
			if tc.RulesReconcileCall != nil {
				mockRulesController.On("Reconcile", mock.Anything, tc.RulesReconcileCall.Instance, &tc.Ingress, &tc.IngressAnnos, tc.TGGroup).Return(tc.RulesReconcileCall.Err)
			}

			controller := &defaultController{
				cloud:           cloud,
				authModule:      mockAuthModule,
				rulesController: mockRulesController,
			}
			err := controller.Reconcile(ctx, ReconcileOptions{
				LBArn:        LBArn,
				Ingress:      &tc.Ingress,
				IngressAnnos: &tc.IngressAnnos,
				Port:         tc.Port,
				TGGroup:      tc.TGGroup,
				Instance:     tc.Instance,
			})
			assert.Equal(t, tc.ExpectedError, err)
			cloud.AssertExpectations(t)
			mockRulesController.AssertExpectations(t)
		})
	}
}

func Test_uniqueHosts(t *testing.T) {
	var tests = []struct {
		expected int
		input    *extensions.Ingress
	}{
		{0, &extensions.Ingress{}},
		{2, &extensions.Ingress{
			Spec: extensions.IngressSpec{
				TLS: []extensions.IngressTLS{
					{
						Hosts: []string{"a", "b"},
					},
				},
			},
		}},
		{3, &extensions.Ingress{
			Spec: extensions.IngressSpec{
				TLS: []extensions.IngressTLS{
					{
						Hosts: []string{
							"a",
							"b",
						},
					},
				},
				Rules: []extensions.IngressRule{
					{
						Host: "a",
					}, {
						Host: "c",
					},
				},
			},
		}},
		{1, &extensions.Ingress{
			Spec: extensions.IngressSpec{
				Rules: []extensions.IngressRule{
					{
						Host: "a",
					}, {
						Host: "a",
					},
				},
			},
		}},
	}

	for _, test := range tests {
		if len(uniqueHosts(test.input)) != test.expected {
			t.Fail()
		}
	}
}
