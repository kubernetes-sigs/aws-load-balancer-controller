package routeutils

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func TestCommonBackendLoader(t *testing.T) {

	kind := HTTPRouteKind

	namespaceToUse := "current-namespace"
	svcNameToUse := "current-svc"
	routeNameToUse := "my-route"

	portConverter := func(port int) *gwv1.PortNumber {
		pn := gwv1.PortNumber(port)
		return &pn
	}

	testCases := []struct {
		name            string
		storedService   *corev1.Service
		backendRef      gwv1.BackendRef
		routeIdentifier types.NamespacedName
		weight          int
		servicePort     int32
		expectErr       bool
		expectNoResult  bool
	}{
		{
			name: "backend ref without namespace",
			routeIdentifier: types.NamespacedName{
				Namespace: "backend-ref-ns",
				Name:      routeNameToUse,
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name: gwv1.ObjectName(svcNameToUse),
					Port: portConverter(80),
				},
			},
			storedService: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "backend-ref-ns",
					Name:      svcNameToUse,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "port-80",
							Port: 80,
						},
					},
				},
			},
			weight:      1,
			servicePort: 80,
		},
		{
			name: "backend ref, fill in weight",
			routeIdentifier: types.NamespacedName{
				Namespace: "backend-ref-ns",
				Name:      routeNameToUse,
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name: gwv1.ObjectName(svcNameToUse),
					Port: portConverter(80),
				},
				Weight: awssdk.Int32(100),
			},
			storedService: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "backend-ref-ns",
					Name:      svcNameToUse,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "port-80",
							Port: 80,
						},
					},
				},
			},
			weight:      100,
			servicePort: 80,
		},
		{
			name: "backend ref with namespace",
			routeIdentifier: types.NamespacedName{
				Name: routeNameToUse,
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(svcNameToUse),
					Namespace: (*gwv1.Namespace)(&namespaceToUse),
					Port:      portConverter(80),
				},
			},
			storedService: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespaceToUse,
					Name:      svcNameToUse,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "port-80",
							Port: 80,
						},
					},
				},
			},
			weight:      1,
			servicePort: 80,
		},
		{
			name: "0 weight backend should return nil",
			routeIdentifier: types.NamespacedName{
				Name: routeNameToUse,
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(svcNameToUse),
					Namespace: (*gwv1.Namespace)(&namespaceToUse),
					Port:      portConverter(80),
				},
				Weight: awssdk.Int32(0),
			},
			expectNoResult: true,
		},
		{
			name: "non-service based backend should return nil",
			routeIdentifier: types.NamespacedName{
				Name: routeNameToUse,
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(svcNameToUse),
					Namespace: (*gwv1.Namespace)(&namespaceToUse),
					Kind:      (*gwv1.Kind)(awssdk.String("cat")),
					Port:      portConverter(80),
				},
			},
			expectNoResult: true,
		},
		{
			name: "missing port in backend ref should result in an error",
			routeIdentifier: types.NamespacedName{
				Name: routeNameToUse,
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(svcNameToUse),
					Namespace: (*gwv1.Namespace)(&namespaceToUse),
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := generateTestClient()

			if tc.storedService != nil {
				k8sClient.Create(context.Background(), tc.storedService)
			}

			result, err := commonBackendLoader(context.Background(), k8sClient, tc.backendRef, tc.backendRef, tc.routeIdentifier, kind)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tc.expectNoResult {
				assert.Nil(t, result)
				return
			}

			assert.Equal(t, tc.storedService, result.Service)
			assert.Equal(t, tc.weight, result.Weight)
			assert.Equal(t, tc.servicePort, result.ServicePort.Port)
			assert.Equal(t, tc.backendRef, result.TypeSpecificBackend)
		})
	}

}
