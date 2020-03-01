package handlers

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	mock_cache "github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks/controller-runtime/cache"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestEnqueueImpactedIngresses(t *testing.T) {
	const namespace = "namespace"
	const service = "service"
	IngressList := extensions.IngressList{
		Items: []extensions.Ingress{
			{
				TypeMeta: v1.TypeMeta{},
				ObjectMeta: v1.ObjectMeta{
					Name:      "relevant-ingress",
					Namespace: namespace,
				},
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							Host: "d1.example.com",
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/path1",
											Backend: extensions.IngressBackend{
												ServiceName: service,
												ServicePort: intstr.FromInt(80),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				TypeMeta: v1.TypeMeta{},
				ObjectMeta: v1.ObjectMeta{
					Name:      "relevant-ingress-with-annotation",
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/actions.weighted-routing": `{"Type":"forward","ForwardConfig":{"TargetGroups":[{"Weight":1,"ServiceName":"service","ServicePort":"80"},{"Weight":1,"ServiceName":"service2","ServicePort":"80"}]}}`,
					},
				},
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							Host: "d1.example.com",
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/path1",
											Backend: extensions.IngressBackend{
												ServiceName: "weighted-routing",
												ServicePort: intstr.FromString("use-annotation"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				TypeMeta: v1.TypeMeta{},
				ObjectMeta: v1.ObjectMeta{
					Name:      "not-relevant-ingress-different-service",
					Namespace: namespace,
				},
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							Host: "d1.example.com",
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Path: "/path1",
											Backend: extensions.IngressBackend{
												ServiceName: "service1",
												ServicePort: intstr.FromInt(80),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := mock_cache.NewMockCache(ctrl)
	mockCache.EXPECT().List(gomock.Any(), client.InNamespace(namespace), &extensions.IngressList{}).SetArg(2, IngressList)

	handler := EnqueueRequestsForEndpointsEvent{
		Cache: mockCache,
	}

	queueMock := &mocks.RateLimitingInterface{}

	queueMock.On("Add", reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      "relevant-ingress",
		},
	})

	queueMock.On("Add", reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      "relevant-ingress-with-annotation",
		},
	})

	handler.enqueueImpactedIngresses(&corev1.Endpoints{
		ObjectMeta: v1.ObjectMeta{
			Name:      service,
			Namespace: namespace,
		},
	},
		queueMock)
}
