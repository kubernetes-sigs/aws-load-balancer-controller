package elbv2

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2modelk8s "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

func Test_defaultTargetGroupBindingManager_Create(t *testing.T) {
	instanceTargetType := elbv2api.TargetTypeInstance
	ipv4AddressType := elbv2api.IPAddressTypeIPV4
	testCases := []struct {
		name     string
		spec     elbv2modelk8s.TargetGroupBindingResourceSpec
		expected elbv2api.TargetGroupBinding
	}{
		{
			name: "just spec, no labels or annotation",
			spec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb",
						Namespace: "tgb-ns",
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetGroupARN: coremodel.LiteralStringToken("arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748"),
						TargetType:     &instanceTargetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromString("my-port"),
						},
						IPAddressType: elbv2api.TargetGroupIPAddressTypeIPv4,
					},
				},
			},
			expected: elbv2api.TargetGroupBinding{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"ingress.k8s.aws/stack-name":      "test-stack",
						"ingress.k8s.aws/stack-namespace": "test-ns",
					},
					ResourceVersion: "1",
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
		},
		{
			name: "just spec, labels. no annotation",
			spec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb",
						Namespace: "tgb-ns",
						Labels: map[string]string{
							"foo": "bar",
							"baz": "bat",
						},
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetGroupARN: coremodel.LiteralStringToken("arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748"),
						TargetType:     &instanceTargetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromString("my-port"),
						},
						IPAddressType: elbv2api.TargetGroupIPAddressTypeIPv4,
					},
				},
			},
			expected: elbv2api.TargetGroupBinding{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"ingress.k8s.aws/stack-name":      "test-stack",
						"ingress.k8s.aws/stack-namespace": "test-ns",
						"foo":                             "bar",
						"baz":                             "bat",
					},
					ResourceVersion: "1",
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
		},
		{
			name: "spec, labels, annotation",
			spec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb",
						Namespace: "tgb-ns",
						Labels: map[string]string{
							"foo": "bar",
							"baz": "bat",
						},
						Annotations: map[string]string{
							"ann1": "ann2",
							"ann3": "ann4",
						},
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetGroupARN: coremodel.LiteralStringToken("arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748"),
						TargetType:     &instanceTargetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromString("my-port"),
						},
						IPAddressType: elbv2api.TargetGroupIPAddressTypeIPv4,
					},
				},
			},
			expected: elbv2api.TargetGroupBinding{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"ingress.k8s.aws/stack-name":      "test-stack",
						"ingress.k8s.aws/stack-namespace": "test-ns",
						"foo":                             "bar",
						"baz":                             "bat",
					},
					Annotations: map[string]string{
						"ann1": "ann2",
						"ann3": "ann4",
					},
					ResourceVersion: "1",
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stack := coremodel.NewDefaultStack(coremodel.StackID(types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-stack",
			}))
			resTGB := elbv2modelk8s.NewTargetGroupBindingResource(stack, "my-tgb", tc.spec)
			manager, k8sClient := createTestDefaultTargetGroupBindingManager()
			status, err := manager.Create(context.Background(), resTGB)
			assert.NoError(t, err)
			res := &elbv2api.TargetGroupBinding{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Namespace: status.TargetGroupBindingRef.Namespace,
				Name:      status.TargetGroupBindingRef.Name,
			}, res)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected.Spec, res.Spec)
			assert.Equal(t, tc.expected.ObjectMeta, res.ObjectMeta)
		})
	}
}

func Test_defaultTargetGroupBindingManager_Update(t *testing.T) {
	instanceTargetType := elbv2api.TargetTypeInstance
	ipv4AddressType := elbv2api.IPAddressTypeIPV4
	testCases := []struct {
		name     string
		spec     elbv2modelk8s.TargetGroupBindingResourceSpec
		existing elbv2api.TargetGroupBinding
		expected elbv2api.TargetGroupBinding
	}{
		{
			name: "just spec, no labels or annotation",
			spec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb",
						Namespace: "tgb-ns",
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetGroupARN: coremodel.LiteralStringToken("arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748"),
						TargetType:     &instanceTargetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromString("my-port"),
						},
						IPAddressType: elbv2api.TargetGroupIPAddressTypeIPv4,
					},
				},
			},
			existing: elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"ingress.k8s.aws/stack-name":      "test-stack-123",
						"ingress.k8s.aws/stack-namespace": "test-ns-1",
					},
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc2",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
			expected: elbv2api.TargetGroupBinding{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"ingress.k8s.aws/stack-name":      "test-stack",
						"ingress.k8s.aws/stack-namespace": "test-ns",
					},
					ResourceVersion: "2",
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
		},
		{
			name: "spec labels annotation",
			spec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb",
						Namespace: "tgb-ns",
						Labels: map[string]string{
							"some label": "some label value",
						},
						Annotations: map[string]string{
							"ann1": "ann2",
							"ann3": "ann4",
						},
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetGroupARN: coremodel.LiteralStringToken("arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748"),
						TargetType:     &instanceTargetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromString("my-port"),
						},
						IPAddressType: elbv2api.TargetGroupIPAddressTypeIPv4,
					},
				},
			},
			existing: elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"ingress.k8s.aws/stack-name":      "test-stack-123",
						"ingress.k8s.aws/stack-namespace": "test-ns-1",
					},
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc2",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
			expected: elbv2api.TargetGroupBinding{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"ingress.k8s.aws/stack-name":      "test-stack",
						"ingress.k8s.aws/stack-namespace": "test-ns",
						"some label":                      "some label value",
					},
					Annotations: map[string]string{
						"ann1": "ann2",
						"ann3": "ann4",
					},
					ResourceVersion: "2",
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
		},
		{
			name: "only diff is checkpoint annotation, no update",
			spec: elbv2modelk8s.TargetGroupBindingResourceSpec{
				Template: elbv2modelk8s.TargetGroupBindingTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb",
						Namespace: "tgb-ns",
						Labels: map[string]string{
							"some label": "some label value",
						},
						Annotations: map[string]string{
							"ann1": "ann2",
							"ann3": "ann4",
						},
					},
					Spec: elbv2modelk8s.TargetGroupBindingSpec{
						TargetGroupARN: coremodel.LiteralStringToken("arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748"),
						TargetType:     &instanceTargetType,
						ServiceRef: elbv2api.ServiceReference{
							Name: "my-svc",
							Port: intstr.FromString("my-port"),
						},
						IPAddressType: elbv2api.TargetGroupIPAddressTypeIPv4,
					},
				},
			},
			existing: elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"some label":                      "some label value",
						"ingress.k8s.aws/stack-name":      "test-stack",
						"ingress.k8s.aws/stack-namespace": "test-ns",
					},
					Annotations: map[string]string{
						"ann1": "ann2",
						"ann3": "ann4",
						annotations.AnnotationCheckPointTimestamp: "foo",
						annotations.AnnotationCheckPoint:          "bar",
					},
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
			expected: elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"some label":                      "some label value",
						"ingress.k8s.aws/stack-name":      "test-stack",
						"ingress.k8s.aws/stack-namespace": "test-ns",
					},
					Annotations: map[string]string{
						"ann1": "ann2",
						"ann3": "ann4",
						annotations.AnnotationCheckPointTimestamp: "foo",
						annotations.AnnotationCheckPoint:          "bar",
					},
					ResourceVersion: "1",
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stack := coremodel.NewDefaultStack(coremodel.StackID(types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-stack",
			}))
			resTGB := elbv2modelk8s.NewTargetGroupBindingResource(stack, "my-tgb", tc.spec)
			manager, k8sClient := createTestDefaultTargetGroupBindingManager()

			err := k8sClient.Create(context.Background(), &tc.existing)
			assert.NoError(t, err)

			status, err := manager.Update(context.Background(), resTGB, &tc.existing)
			assert.NoError(t, err)
			res := &elbv2api.TargetGroupBinding{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Namespace: status.TargetGroupBindingRef.Namespace,
				Name:      status.TargetGroupBindingRef.Name,
			}, res)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected.Spec, res.Spec)
			assert.Equal(t, tc.expected.ObjectMeta, res.ObjectMeta)
		})
	}
}

func Test_defaultTargetGroupBindingManager_Delete(t *testing.T) {
	instanceTargetType := elbv2api.TargetTypeInstance
	ipv4AddressType := elbv2api.IPAddressTypeIPV4
	testCases := []struct {
		name     string
		existing elbv2api.TargetGroupBinding
	}{
		{
			name: "delete",
			existing: elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgb",
					Namespace: "tgb-ns",
					Labels: map[string]string{
						"ingress.k8s.aws/stack-name":      "test-stack-123",
						"ingress.k8s.aws/stack-namespace": "test-ns-1",
					},
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					TargetGroupARN: "arn:aws:elasticloadbalancing:us-east-1:565768096483:targetgroup/k8s-echoserv-brokentg-0b7ba7f4ef/ae85b8ea9fb69748",
					TargetType:     &instanceTargetType,
					ServiceRef: elbv2api.ServiceReference{
						Name: "my-svc2",
						Port: intstr.FromString("my-port"),
					},
					IPAddressType: (*elbv2api.TargetGroupIPAddressType)(&ipv4AddressType),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			manager, k8sClient := createTestDefaultTargetGroupBindingManager()

			err := k8sClient.Create(context.Background(), &tc.existing)
			assert.NoError(t, err)

			err = manager.Delete(context.Background(), &tc.existing)
			assert.NoError(t, err)
			res := &elbv2api.TargetGroupBinding{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Namespace: tc.existing.Namespace,
				Name:      tc.existing.Name,
			}, res)
			assert.Error(t, err)
			assert.NoError(t, client.IgnoreNotFound(err))
		})
	}
}

func createTestDefaultTargetGroupBindingManager() (defaultTargetGroupBindingManager, client.Client) {
	k8sClient := testutils.GenerateTestClient()
	manager := defaultTargetGroupBindingManager{
		k8sClient:        k8sClient,
		trackingProvider: tracking.NewDefaultProvider("ingress.k8s.aws", "foo"),
		logger:           logr.Discard(),

		waitTGBObservedPollInterval: 10 * time.Millisecond,
		waitTGBObservedTimout:       100 * time.Millisecond,
		waitTGBDeletionPollInterval: 10 * time.Millisecond,
		waitTGBDeletionTimeout:      100 * time.Millisecond,
	}
	return manager, k8sClient
}
