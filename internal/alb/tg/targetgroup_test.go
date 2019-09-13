package tg

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	annoTags "github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/targetgroup"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type GetIngressAnnotationsCall struct {
	Key          string
	IngressAnnos *annotations.Ingress
	Err          error
}

type GetServiceAnnotationsCall struct {
	Key          string
	IngressAnnos *annotations.Ingress
	ServiceAnnos *annotations.Service
	Err          error
}

type GetServiceCall struct {
	Key     string
	service *corev1.Service
	Err     error
}
type NameTGCall struct {
	Namespace   string
	IngressName string
	ServiceName string
	ServicePort string
	TargetType  string
	Protocol    string
	TGName      string
}

type TagTGGroupCall struct {
	Namespace   string
	IngressName string
	Tags        map[string]string
}

type TagTGCall struct {
	ServiceName string
	ServicePort string
	Tags        map[string]string
}

type GetTargetGroupByNameCall struct {
	TGName   string
	Instance *elbv2.TargetGroup
	Err      error
}

type ModifyTargetGroupCall struct {
	Input    *elbv2.ModifyTargetGroupInput
	Instance *elbv2.TargetGroup
	Err      error
}

type CreateTargetGroupCall struct {
	Input    *elbv2.CreateTargetGroupInput
	Instance *elbv2.TargetGroup
	Err      error
}

type TagsReconcileCall struct {
	Arn  string
	Tags map[string]string
	Err  error
}

type AttributesReconcileCall struct {
	TGArn      string
	Attributes []*elbv2.TargetGroupAttribute
	Err        error
}

type TargetsReconcileCall struct {
	Targets       *Targets
	ResultTargets []*elbv2.TargetDescription
	Err           error
}

func TestDefaultController_Reconcile(t *testing.T) {
	ingress := extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress",
			Namespace: "namespace",
		},
	}
	ingressBackend := extensions.IngressBackend{
		ServiceName: "service",
		ServicePort: intstr.FromInt(443),
	}
	for _, tc := range []struct {
		Name                      string
		Ingress                   extensions.Ingress
		Backend                   extensions.IngressBackend
		GetIngressAnnotationsCall *GetIngressAnnotationsCall
		GetServiceAnnotationsCall *GetServiceAnnotationsCall
		GetServiceCall            *GetServiceCall
		NameTGCall                *NameTGCall
		TagTGCall                 *TagTGCall
		TagTGGroupCall            *TagTGGroupCall
		GetTargetGroupByNameCall  *GetTargetGroupByNameCall
		ModifyTargetGroupCall     *ModifyTargetGroupCall
		CreateTargetGroupCall     *CreateTargetGroupCall
		TagsReconcileCall         *TagsReconcileCall
		AttributesReconcileCall   *AttributesReconcileCall
		TargetsReconcileCall      *TargetsReconcileCall
		ExpectedTG                TargetGroup
		ExpectedError             error
	}{
		{
			Name:    "Reconcile succeeds by creating instance",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			TagTGCall: &TagTGCall{
				ServiceName: "service",
				ServicePort: "443",
				Tags:        map[string]string{"tg-tag": "tg-tag-value"},
			},
			TagTGGroupCall: &TagTGGroupCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				Tags:        map[string]string{"group-tag": "group-tag-value"},
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName:   "k8s-tgName",
				Instance: nil,
			},
			CreateTargetGroupCall: &CreateTargetGroupCall{
				Input: &elbv2.CreateTargetGroupInput{
					Name:                       aws.String("k8s-tgName"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
					Port:                       aws.Int64(targetGroupDefaultPort),
				},
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
			},
			TagsReconcileCall: &TagsReconcileCall{
				Arn:  "MyTargetGroupArn",
				Tags: map[string]string{"tg-tag": "tg-tag-value", "group-tag": "group-tag-value"},
			},
			AttributesReconcileCall: &AttributesReconcileCall{
				TGArn: "MyTargetGroupArn",
				Attributes: []*elbv2.TargetGroupAttribute{
					{
						Key:   aws.String("stickiness.enabled"),
						Value: aws.String("true"),
					},
				},
			},
			TargetsReconcileCall: &TargetsReconcileCall{
				Targets: &Targets{
					TgArn:      "MyTargetGroupArn",
					TargetType: "ip",
					Ingress:    &ingress,
					Backend:    &ingressBackend,
				},
				ResultTargets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
			ExpectedTG: TargetGroup{
				Arn:        "MyTargetGroupArn",
				TargetType: "ip",
				Targets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
		},
		{
			Name:    "Reconcile succeeds when looking up a service port by name for target-type=instance. ",
			Ingress: ingress,
			Backend: ingressBackend,
			GetServiceCall: &GetServiceCall{
				Key: "namespace/service",
				service: &corev1.Service{
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name:     "foo",
								NodePort: 9090,
							},
						},
					},
				},
			},
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("foo"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("instance"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "instance",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			TagTGCall: &TagTGCall{
				ServiceName: "service",
				ServicePort: "443",
				Tags:        map[string]string{"tg-tag": "tg-tag-value"},
			},
			TagTGGroupCall: &TagTGGroupCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				Tags:        map[string]string{"group-tag": "group-tag-value"},
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName:   "k8s-tgName",
				Instance: nil,
			},
			CreateTargetGroupCall: &CreateTargetGroupCall{
				Input: &elbv2.CreateTargetGroupInput{
					Name:                       aws.String("k8s-tgName"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("9090"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("instance"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
					Port:                       aws.Int64(targetGroupDefaultPort),
				},
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
			},
			TagsReconcileCall: &TagsReconcileCall{
				Arn:  "MyTargetGroupArn",
				Tags: map[string]string{"tg-tag": "tg-tag-value", "group-tag": "group-tag-value"},
			},
			AttributesReconcileCall: &AttributesReconcileCall{
				TGArn: "MyTargetGroupArn",
				Attributes: []*elbv2.TargetGroupAttribute{
					{
						Key:   aws.String("stickiness.enabled"),
						Value: aws.String("true"),
					},
				},
			},
			TargetsReconcileCall: &TargetsReconcileCall{
				Targets: &Targets{
					TgArn:      "MyTargetGroupArn",
					TargetType: "instance",
					Ingress:    &ingress,
					Backend:    &ingressBackend,
				},
				ResultTargets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
			ExpectedTG: TargetGroup{
				Arn:        "MyTargetGroupArn",
				TargetType: "instance",
				Targets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
		},
		{
			Name:    "Reconcile succeeds when looking up a service port by name for target-type=instance. ",
			Ingress: ingress,
			Backend: ingressBackend,
			GetServiceCall: &GetServiceCall{
				Key: "namespace/service",
				service: &corev1.Service{
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name:       "foo",
								NodePort:   9090,
								TargetPort: intstr.FromInt(9091),
							},
						},
					},
				},
			},
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("foo"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			TagTGCall: &TagTGCall{
				ServiceName: "service",
				ServicePort: "443",
				Tags:        map[string]string{"tg-tag": "tg-tag-value"},
			},
			TagTGGroupCall: &TagTGGroupCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				Tags:        map[string]string{"group-tag": "group-tag-value"},
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName:   "k8s-tgName",
				Instance: nil,
			},
			CreateTargetGroupCall: &CreateTargetGroupCall{
				Input: &elbv2.CreateTargetGroupInput{
					Name:                       aws.String("k8s-tgName"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("9091"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
					Port:                       aws.Int64(targetGroupDefaultPort),
				},
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
			},
			TagsReconcileCall: &TagsReconcileCall{
				Arn:  "MyTargetGroupArn",
				Tags: map[string]string{"tg-tag": "tg-tag-value", "group-tag": "group-tag-value"},
			},
			AttributesReconcileCall: &AttributesReconcileCall{
				TGArn: "MyTargetGroupArn",
				Attributes: []*elbv2.TargetGroupAttribute{
					{
						Key:   aws.String("stickiness.enabled"),
						Value: aws.String("true"),
					},
				},
			},
			TargetsReconcileCall: &TargetsReconcileCall{
				Targets: &Targets{
					TgArn:      "MyTargetGroupArn",
					TargetType: "ip",
					Ingress:    &ingress,
					Backend:    &ingressBackend,
				},
				ResultTargets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
			ExpectedTG: TargetGroup{
				Arn:        "MyTargetGroupArn",
				TargetType: "ip",
				Targets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
		},
		{
			Name:    "Reconcile succeeds by reconcile non-modified existing instance",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			TagTGCall: &TagTGCall{
				ServiceName: "service",
				ServicePort: "443",
				Tags:        map[string]string{"tg-tag": "tg-tag-value"},
			},
			TagTGGroupCall: &TagTGGroupCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				Tags:        map[string]string{"group-tag": "group-tag-value"},
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName: "k8s-tgName",
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
			},
			TagsReconcileCall: &TagsReconcileCall{
				Arn:  "MyTargetGroupArn",
				Tags: map[string]string{"tg-tag": "tg-tag-value", "group-tag": "group-tag-value"},
			},
			AttributesReconcileCall: &AttributesReconcileCall{
				TGArn: "MyTargetGroupArn",
				Attributes: []*elbv2.TargetGroupAttribute{
					{
						Key:   aws.String("stickiness.enabled"),
						Value: aws.String("true"),
					},
				},
			},
			TargetsReconcileCall: &TargetsReconcileCall{
				Targets: &Targets{
					TgArn:      "MyTargetGroupArn",
					TargetType: "ip",
					Ingress:    &ingress,
					Backend:    &ingressBackend,
				},
				ResultTargets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
			ExpectedTG: TargetGroup{
				Arn:        "MyTargetGroupArn",
				TargetType: "ip",
				Targets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
		},
		{
			Name:    "Reconcile succeeds by reconcile modified existing instance",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			TagTGCall: &TagTGCall{
				ServiceName: "service",
				ServicePort: "443",
				Tags:        map[string]string{"tg-tag": "tg-tag-value"},
			},
			TagTGGroupCall: &TagTGGroupCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				Tags:        map[string]string{"group-tag": "group-tag-value"},
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName: "k8s-tgName",
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/pong"),
					HealthCheckPort:            aws.String("8088"),
					HealthCheckProtocol:        aws.String("HTTPS"),
					HealthCheckIntervalSeconds: aws.Int64(100),
					HealthCheckTimeoutSeconds:  aws.Int64(600),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("8080")},
					HealthyThresholdCount:      aws.Int64(80),
					UnhealthyThresholdCount:    aws.Int64(50),
				},
			},
			ModifyTargetGroupCall: &ModifyTargetGroupCall{
				Input: &elbv2.ModifyTargetGroupInput{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8088"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
			},
			TagsReconcileCall: &TagsReconcileCall{
				Arn:  "MyTargetGroupArn",
				Tags: map[string]string{"tg-tag": "tg-tag-value", "group-tag": "group-tag-value"},
			},
			AttributesReconcileCall: &AttributesReconcileCall{
				TGArn: "MyTargetGroupArn",
				Attributes: []*elbv2.TargetGroupAttribute{
					{
						Key:   aws.String("stickiness.enabled"),
						Value: aws.String("true"),
					},
				},
			},
			TargetsReconcileCall: &TargetsReconcileCall{
				Targets: &Targets{
					TgArn:      "MyTargetGroupArn",
					TargetType: "ip",
					Ingress:    &ingress,
					Backend:    &ingressBackend,
				},
				ResultTargets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
			ExpectedTG: TargetGroup{
				Arn:        "MyTargetGroupArn",
				TargetType: "ip",
				Targets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
			},
		},
		{
			Name:    "Reconcile failed when fetching existing instance",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName: "k8s-tgName",
				Err:    errors.New("GetTargetGroupByName"),
			},
			ExpectedError: errors.New("failed to find existing targetGroup due to GetTargetGroupByName"),
		},
		{
			Name:    "Reconcile succeeds when creating instance",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName:   "k8s-tgName",
				Instance: nil,
			},
			CreateTargetGroupCall: &CreateTargetGroupCall{
				Input: &elbv2.CreateTargetGroupInput{
					Name:                       aws.String("k8s-tgName"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
					Port:                       aws.Int64(targetGroupDefaultPort),
				},
				Err: errors.New("CreateTargetGroup"),
			},
			ExpectedError: errors.New("failed to create targetGroup due to CreateTargetGroup"),
		},
		{
			Name:    "Reconcile failed when modify targetGroup instance",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName: "k8s-tgName",
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/pong"),
					HealthCheckPort:            aws.String("8088"),
					HealthCheckProtocol:        aws.String("HTTPS"),
					HealthCheckIntervalSeconds: aws.Int64(100),
					HealthCheckTimeoutSeconds:  aws.Int64(600),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("8080")},
					HealthyThresholdCount:      aws.Int64(80),
					UnhealthyThresholdCount:    aws.Int64(50),
				},
			},
			ModifyTargetGroupCall: &ModifyTargetGroupCall{
				Input: &elbv2.ModifyTargetGroupInput{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
				Err: errors.New("ModifyTargetGroup"),
			},
			ExpectedError: errors.New("failed to modify targetGroup due to ModifyTargetGroup"),
		},
		{
			Name:    "Reconcile failed when reconcile tags",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			TagTGCall: &TagTGCall{
				ServiceName: "service",
				ServicePort: "443",
				Tags:        map[string]string{"tg-tag": "tg-tag-value"},
			},
			TagTGGroupCall: &TagTGGroupCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				Tags:        map[string]string{"group-tag": "group-tag-value"},
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName: "k8s-tgName",
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
			},
			TagsReconcileCall: &TagsReconcileCall{
				Arn:  "MyTargetGroupArn",
				Tags: map[string]string{"tg-tag": "tg-tag-value", "group-tag": "group-tag-value"},
				Err:  errors.New("TagsReconcileCall"),
			},
			ExpectedError: errors.New("failed to reconcile targetGroup tags due to TagsReconcileCall"),
		},
		{
			Name:    "Reconcile failed when reconcile attributes",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			TagTGCall: &TagTGCall{
				ServiceName: "service",
				ServicePort: "443",
				Tags:        map[string]string{"tg-tag": "tg-tag-value"},
			},
			TagTGGroupCall: &TagTGGroupCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				Tags:        map[string]string{"group-tag": "group-tag-value"},
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName: "k8s-tgName",
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
			},
			TagsReconcileCall: &TagsReconcileCall{
				Arn:  "MyTargetGroupArn",
				Tags: map[string]string{"tg-tag": "tg-tag-value", "group-tag": "group-tag-value"},
			},
			AttributesReconcileCall: &AttributesReconcileCall{
				TGArn: "MyTargetGroupArn",
				Attributes: []*elbv2.TargetGroupAttribute{
					{
						Key:   aws.String("stickiness.enabled"),
						Value: aws.String("true"),
					},
				},
				Err: errors.New("AttributesReconcileCall"),
			},
			ExpectedError: errors.New("failed to reconcile targetGroup attributes due to AttributesReconcileCall"),
		},
		{
			Name:    "Reconcile failed when reconcile targets",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				ServiceAnnos: &annotations.Service{
					HealthCheck: &healthcheck.Config{
						Path:            aws.String("/ping"),
						Port:            aws.String("8080"),
						Protocol:        aws.String("HTTP"),
						IntervalSeconds: aws.Int64(10),
						TimeoutSeconds:  aws.Int64(60),
					},
					TargetGroup: &targetgroup.Config{
						BackendProtocol:         aws.String("HTTP"),
						TargetType:              aws.String("ip"),
						SuccessCodes:            aws.String("80"),
						HealthyThresholdCount:   aws.Int64(8),
						UnhealthyThresholdCount: aws.Int64(5),
						Attributes: []*elbv2.TargetGroupAttribute{
							{
								Key:   aws.String("stickiness.enabled"),
								Value: aws.String("true"),
							},
						},
					},
				},
			},
			NameTGCall: &NameTGCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				ServiceName: "service",
				ServicePort: "443",
				TargetType:  "ip",
				Protocol:    "HTTP",
				TGName:      "k8s-tgName",
			},
			TagTGCall: &TagTGCall{
				ServiceName: "service",
				ServicePort: "443",
				Tags:        map[string]string{"tg-tag": "tg-tag-value"},
			},
			TagTGGroupCall: &TagTGGroupCall{
				Namespace:   "namespace",
				IngressName: "ingress",
				Tags:        map[string]string{"group-tag": "group-tag-value"},
			},
			GetTargetGroupByNameCall: &GetTargetGroupByNameCall{
				TGName: "k8s-tgName",
				Instance: &elbv2.TargetGroup{
					TargetGroupArn:             aws.String("MyTargetGroupArn"),
					HealthCheckPath:            aws.String("/ping"),
					HealthCheckPort:            aws.String("8080"),
					HealthCheckProtocol:        aws.String("HTTP"),
					HealthCheckIntervalSeconds: aws.Int64(10),
					HealthCheckTimeoutSeconds:  aws.Int64(60),
					Protocol:                   aws.String("HTTP"),
					TargetType:                 aws.String("ip"),
					Matcher:                    &elbv2.Matcher{HttpCode: aws.String("80")},
					HealthyThresholdCount:      aws.Int64(8),
					UnhealthyThresholdCount:    aws.Int64(5),
				},
			},
			TagsReconcileCall: &TagsReconcileCall{
				Arn:  "MyTargetGroupArn",
				Tags: map[string]string{"tg-tag": "tg-tag-value", "group-tag": "group-tag-value"},
			},
			AttributesReconcileCall: &AttributesReconcileCall{
				TGArn: "MyTargetGroupArn",
				Attributes: []*elbv2.TargetGroupAttribute{
					{
						Key:   aws.String("stickiness.enabled"),
						Value: aws.String("true"),
					},
				},
			},
			TargetsReconcileCall: &TargetsReconcileCall{
				Targets: &Targets{
					TgArn:      "MyTargetGroupArn",
					TargetType: "ip",
					Ingress:    &ingress,
					Backend:    &ingressBackend,
				},
				ResultTargets: []*elbv2.TargetDescription{
					{
						Id:   aws.String("instance-id"),
						Port: aws.Int64(8888),
					},
				},
				Err: errors.New("TargetsReconcileCall"),
			},
			ExpectedError: errors.New("failed to reconcile targetGroup targets due to TargetsReconcileCall"),
		},
		{
			Name:    "GetIngressAnnotations returns error",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key: "namespace/ingress",
				Err: errors.New("GetIngressAnnotations"),
			},
			ExpectedError: errors.New("failed to load ingressAnnotation due to GetIngressAnnotations"),
		},
		{
			Name:    "GetServiceAnnotations returns error",
			Ingress: ingress,
			Backend: ingressBackend,
			GetIngressAnnotationsCall: &GetIngressAnnotationsCall{
				Key:          "namespace/ingress",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
			},
			GetServiceAnnotationsCall: &GetServiceAnnotationsCall{
				Key:          "namespace/service",
				IngressAnnos: &annotations.Ingress{Tags: &annoTags.Config{}},
				Err:          errors.New("GetServiceAnnotations"),
			},
			ExpectedError: errors.New("failed to load serviceAnnotation due to GetServiceAnnotations"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.GetTargetGroupByNameCall != nil {
				cloud.On("GetTargetGroupByName", ctx, tc.GetTargetGroupByNameCall.TGName).Return(tc.GetTargetGroupByNameCall.Instance, tc.GetTargetGroupByNameCall.Err)
			}
			if tc.ModifyTargetGroupCall != nil {
				cloud.On("ModifyTargetGroupWithContext", ctx, tc.ModifyTargetGroupCall.Input).Return(&elbv2.ModifyTargetGroupOutput{
					TargetGroups: []*elbv2.TargetGroup{tc.ModifyTargetGroupCall.Instance},
				}, tc.ModifyTargetGroupCall.Err)
			}
			if tc.CreateTargetGroupCall != nil {
				cloud.On("CreateTargetGroupWithContext", ctx, tc.CreateTargetGroupCall.Input).Return(&elbv2.CreateTargetGroupOutput{
					TargetGroups: []*elbv2.TargetGroup{tc.CreateTargetGroupCall.Instance},
				}, tc.CreateTargetGroupCall.Err)
			}

			mockStore := &store.MockStorer{}
			if tc.GetIngressAnnotationsCall != nil {
				mockStore.On("GetIngressAnnotations", tc.GetIngressAnnotationsCall.Key).Return(tc.GetIngressAnnotationsCall.IngressAnnos, tc.GetIngressAnnotationsCall.Err)
			}
			if tc.GetServiceAnnotationsCall != nil {
				mockStore.On("GetServiceAnnotations", tc.GetServiceAnnotationsCall.Key, tc.GetServiceAnnotationsCall.IngressAnnos).Return(tc.GetServiceAnnotationsCall.ServiceAnnos, tc.GetServiceAnnotationsCall.Err)
			}

			if tc.GetServiceCall != nil {
				mockStore.On("GetService", tc.GetServiceCall.Key).Return(tc.GetServiceCall.service, tc.GetServiceCall.Err)
			}

			mockNameTagGen := &MockNameTagGenerator{}
			if tc.NameTGCall != nil {
				mockNameTagGen.On("NameTG", tc.NameTGCall.Namespace, tc.NameTGCall.IngressName, tc.NameTGCall.ServiceName, tc.NameTGCall.ServicePort, tc.NameTGCall.TargetType, tc.NameTGCall.Protocol).Return(tc.NameTGCall.TGName)
			}
			if tc.TagTGCall != nil {
				mockNameTagGen.On("TagTG", tc.TagTGGroupCall.Namespace, tc.TagTGGroupCall.IngressName, tc.TagTGCall.ServiceName, tc.TagTGCall.ServicePort).Return(tc.TagTGCall.Tags)
			}
			if tc.TagTGGroupCall != nil {
				mockNameTagGen.On("TagTGGroup", tc.TagTGGroupCall.Namespace, tc.TagTGGroupCall.IngressName).Return(tc.TagTGGroupCall.Tags)
			}

			mockTagsController := &tags.MockController{}
			if tc.TagsReconcileCall != nil {
				mockTagsController.On("ReconcileELB", mock.Anything, tc.TagsReconcileCall.Arn, tc.TagsReconcileCall.Tags).Return(tc.TagsReconcileCall.Err)
			}

			mockAttrsController := &MockAttributesController{}
			if tc.AttributesReconcileCall != nil {
				mockAttrsController.On("Reconcile", mock.Anything, tc.AttributesReconcileCall.TGArn, tc.AttributesReconcileCall.Attributes).Return(tc.AttributesReconcileCall.Err)
			}

			mockTargetsController := &MockTargetsController{}
			if tc.TargetsReconcileCall != nil {
				mockTargetsController.On("Reconcile", mock.Anything, tc.TargetsReconcileCall.Targets).Return(tc.TargetsReconcileCall.Err).Run(func(args mock.Arguments) {
					targets := args.Get(1).(*Targets)
					targets.Targets = tc.TargetsReconcileCall.ResultTargets
				})
			}

			controller := &defaultController{
				cloud:      cloud,
				store:      mockStore,
				nameTagGen: mockNameTagGen,

				tagsController:    mockTagsController,
				attrsController:   mockAttrsController,
				targetsController: mockTargetsController,
			}

			tg, err := controller.Reconcile(context.Background(), &tc.Ingress, tc.Backend)
			assert.Equal(t, tc.ExpectedTG, tg)
			assert.Equal(t, tc.ExpectedError, err)
			cloud.AssertExpectations(t)
			mockStore.AssertExpectations(t)
			mockNameTagGen.AssertExpectations(t)
			mockTagsController.AssertExpectations(t)
			mockAttrsController.AssertExpectations(t)
			mockTargetsController.AssertExpectations(t)
		})
	}
}
