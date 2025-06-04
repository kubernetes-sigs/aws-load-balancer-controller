package routeutils

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
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

	tgConfigTargetSvcAndNs := elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tg1",
			Namespace: namespaceToUse,
		},
		Spec: elbv2gw.TargetGroupConfigurationSpec{
			TargetReference: elbv2gw.Reference{
				Kind: awssdk.String(serviceKind),
				Name: svcNameToUse,
			},
		},
	}

	tgConfigDifferentSvcAndTargetNs := elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tg2",
			Namespace: namespaceToUse,
		},
		Spec: elbv2gw.TargetGroupConfigurationSpec{
			TargetReference: elbv2gw.Reference{
				Kind: awssdk.String(serviceKind),
				Name: "other-svc-name",
			},
		},
	}

	tgConfigTargetSvcAndDifferentNs := elbv2gw.TargetGroupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tg3",
			Namespace: "differentNs",
		},
		Spec: elbv2gw.TargetGroupConfigurationSpec{
			TargetReference: elbv2gw.Reference{
				Kind: awssdk.String(serviceKind),
				Name: svcNameToUse,
			},
		},
	}

	testCases := []struct {
		name                string
		storedService       *corev1.Service
		storedTGConfigs     []elbv2gw.TargetGroupConfiguration
		referenceGrants     []gwbeta1.ReferenceGrant
		backendRef          gwv1.BackendRef
		routeIdentifier     types.NamespacedName
		weight              int
		servicePort         int32
		expectErr           bool
		expectNoResult      bool
		expectedTargetGroup *elbv2gw.TargetGroupConfiguration
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
			expectedTargetGroup: nil, // namespace is wrong
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
			expectedTargetGroup: nil, // namespace is wrong
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
				Name:      routeNameToUse,
				Namespace: namespaceToUse,
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(svcNameToUse),
					Namespace: (*gwv1.Namespace)(&namespaceToUse),
					Port:      portConverter(80),
				},
			},
			expectedTargetGroup: &tgConfigTargetSvcAndNs,
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
			name: "route and service in different namespace (no reference grant)",
			routeIdentifier: types.NamespacedName{
				Name:      routeNameToUse,
				Namespace: "route-ns",
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(svcNameToUse),
					Namespace: (*gwv1.Namespace)(&namespaceToUse),
					Port:      portConverter(80),
				},
			},
			expectNoResult: true,
			expectErr:      true,
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
		},
		{
			name: "route and service in different namespace (with reference grant)",
			routeIdentifier: types.NamespacedName{
				Name:      routeNameToUse,
				Namespace: "route-ns",
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(svcNameToUse),
					Namespace: (*gwv1.Namespace)(&namespaceToUse),
					Port:      portConverter(80),
				},
			},
			referenceGrants: []gwbeta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespaceToUse,
						Name:      "grant1",
					},
					Spec: gwbeta1.ReferenceGrantSpec{
						From: []gwbeta1.ReferenceGrantFrom{
							{
								Kind:      gwbeta1.Kind(kind),
								Namespace: "route-ns",
							},
						},
						To: []gwbeta1.ReferenceGrantTo{
							{
								Kind: serviceKind,
							},
						},
					},
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
			expectedTargetGroup: &tgConfigTargetSvcAndNs,
			weight:              1,
			servicePort:         80,
		},
		{
			name: "0 weight backend should return nil",
			routeIdentifier: types.NamespacedName{
				Name:      routeNameToUse,
				Namespace: namespaceToUse,
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
				Name:      routeNameToUse,
				Namespace: namespaceToUse,
			},
			backendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(svcNameToUse),
					Namespace: (*gwv1.Namespace)(&namespaceToUse),
					Kind:      (*gwv1.Kind)(awssdk.String("cat")),
					Port:      portConverter(80),
				},
			},
			expectErr: true,
		},
		{
			name: "missing port in backend ref should result in an error",
			routeIdentifier: types.NamespacedName{
				Name:      routeNameToUse,
				Namespace: namespaceToUse,
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
			k8sClient := testutils.GenerateTestClient()

			if tc.storedService != nil {
				err := k8sClient.Create(context.Background(), tc.storedService)
				assert.NoError(t, err)
			}

			for _, c := range []elbv2gw.TargetGroupConfiguration{tgConfigTargetSvcAndNs, tgConfigDifferentSvcAndTargetNs, tgConfigTargetSvcAndDifferentNs} {
				err := k8sClient.Create(context.Background(), &c)
				assert.NoError(t, err, fmt.Sprintf("%+v", c))
			}

			for _, g := range tc.referenceGrants {
				err := k8sClient.Create(context.Background(), &g)
				assert.NoError(t, err, fmt.Sprintf("%+v", g))
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

			if tc.expectedTargetGroup == nil {
				assert.Nil(t, result.ELBv2TargetGroupConfig)
			} else {
				assert.Equal(t, tc.expectedTargetGroup.Name, result.ELBv2TargetGroupConfig.Name)
				assert.Equal(t, tc.expectedTargetGroup.Namespace, result.ELBv2TargetGroupConfig.Namespace)
			}
		})
	}
}

func Test_lookUpTargetGroupConfiguration(t *testing.T) {
	testCases := []struct {
		name                         string
		allTargetGroupConfigurations []elbv2gw.TargetGroupConfiguration
		serviceMetadata              types.NamespacedName
		expectErr                    bool
		expectedTGConfiguration      *elbv2gw.TargetGroupConfiguration
	}{
		{
			name: "happy path, exactly one tg config",
			allTargetGroupConfigurations: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tg1",
						Namespace: "namespace",
					},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(serviceKind),
							Name: "svc1",
						},
					},
				},
			},
			serviceMetadata: types.NamespacedName{
				Namespace: "namespace",
				Name:      "svc1",
			},
			expectedTGConfiguration: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tg1",
					Namespace: "namespace",
				},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Kind: awssdk.String(serviceKind),
						Name: "svc1",
					},
				},
			},
		},
		{
			name: "happy path, exactly one tg config (kind not specified)",
			allTargetGroupConfigurations: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tg1",
						Namespace: "namespace",
					},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Name: "svc1",
						},
					},
				},
			},
			serviceMetadata: types.NamespacedName{
				Namespace: "namespace",
				Name:      "svc1",
			},
			expectedTGConfiguration: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tg1",
					Namespace: "namespace",
				},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Name: "svc1",
					},
				},
			},
		},
		{
			name: "sad path, svc name different",
			allTargetGroupConfigurations: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tg1",
						Namespace: "namespace",
					},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(serviceKind),
							Name: "svc2",
						},
					},
				},
			},
			serviceMetadata: types.NamespacedName{
				Namespace: "namespace",
				Name:      "svc1",
			},
		},
		{
			name: "sad path, kind not supported",
			allTargetGroupConfigurations: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tg1",
						Namespace: "namespace",
					},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String("cat"),
							Name: "svc2",
						},
					},
				},
			},
			serviceMetadata: types.NamespacedName{
				Namespace: "namespace",
				Name:      "svc1",
			},
		},
		{
			name: "sad path, many tg none match",
			allTargetGroupConfigurations: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tg1",
						Namespace: "namespace",
					},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(serviceKind),
							Name: "foo",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tg2",
						Namespace: "namespace",
					},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(serviceKind),
							Name: "baz",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tg3",
						Namespace: "namespace",
					},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(serviceKind),
							Name: "bar",
						},
					},
				},
			},
			serviceMetadata: types.NamespacedName{
				Namespace: "namespace",
				Name:      "svc1",
			},
			expectedTGConfiguration: nil,
		},
		{
			name: "sad path, no tg none match",
			serviceMetadata: types.NamespacedName{
				Namespace: "namespace",
				Name:      "svc1",
			},
			expectedTGConfiguration: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testutils.GenerateTestClient()
			for _, c := range tc.allTargetGroupConfigurations {
				err := k8sClient.Create(context.Background(), &c)
				assert.NoError(t, err)
			}

			result, err := LookUpTargetGroupConfiguration(context.Background(), k8sClient, tc.serviceMetadata)

			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if result != nil {
				// Reset resource version from the create call.
				result.ResourceVersion = ""
			}
			assert.Equal(t, tc.expectedTGConfiguration, result)
		})
	}
}

func Test_referenceGrantCheck(t *testing.T) {
	kind := HTTPRouteKind
	testCases := []struct {
		name            string
		referenceGrants []gwbeta1.ReferenceGrant
		svcIdentifier   types.NamespacedName
		routeIdentifier types.NamespacedName
		expected        bool
		expectErr       bool
	}{
		{
			name: "happy path",
			svcIdentifier: types.NamespacedName{
				Namespace: "svc-namespace",
				Name:      "svc-name",
			},
			routeIdentifier: types.NamespacedName{
				Namespace: "route-namespace",
				Name:      "route-name",
			},
			referenceGrants: []gwbeta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwbeta1.ReferenceGrantSpec{
						From: []gwbeta1.ReferenceGrantFrom{
							{
								Kind:      gwbeta1.Kind(kind),
								Namespace: "route-namespace",
							},
						},
						To: []gwbeta1.ReferenceGrantTo{
							{
								Kind: serviceKind,
								Name: (*gwbeta1.ObjectName)(awssdk.String("svc-name")),
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "happy path (no name equals wildcard)",
			svcIdentifier: types.NamespacedName{
				Namespace: "svc-namespace",
				Name:      "svc-name",
			},
			routeIdentifier: types.NamespacedName{
				Namespace: "route-namespace",
				Name:      "route-name",
			},
			referenceGrants: []gwbeta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwbeta1.ReferenceGrantSpec{
						From: []gwbeta1.ReferenceGrantFrom{
							{
								Kind:      gwbeta1.Kind(kind),
								Namespace: "route-namespace",
							},
						},
						To: []gwbeta1.ReferenceGrantTo{
							{
								Kind: serviceKind,
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "no grants, should not allow",
			svcIdentifier: types.NamespacedName{
				Namespace: "svc-namespace",
				Name:      "svc-name",
			},
			routeIdentifier: types.NamespacedName{
				Namespace: "route-namespace",
				Name:      "route-name",
			},
			expected: false,
		},
		{
			name: "from is allowed, but not to",
			svcIdentifier: types.NamespacedName{
				Namespace: "svc-namespace",
				Name:      "svc-name",
			},
			routeIdentifier: types.NamespacedName{
				Namespace: "route-namespace",
				Name:      "route-name",
			},
			referenceGrants: []gwbeta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwbeta1.ReferenceGrantSpec{
						From: []gwbeta1.ReferenceGrantFrom{
							{
								Kind:      gwbeta1.Kind(kind),
								Namespace: "route-namespace",
							},
						},
						To: []gwbeta1.ReferenceGrantTo{
							{
								Kind: serviceKind,
								Name: (*gwbeta1.ObjectName)(awssdk.String("baz")),
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "to is allowed, but not from",
			svcIdentifier: types.NamespacedName{
				Namespace: "svc-namespace",
				Name:      "svc-name",
			},
			routeIdentifier: types.NamespacedName{
				Namespace: "route-namespace",
				Name:      "route-name",
			},
			referenceGrants: []gwbeta1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "svc-namespace",
						Name:      "grant1",
					},
					Spec: gwbeta1.ReferenceGrantSpec{
						From: []gwbeta1.ReferenceGrantFrom{
							{
								Kind:      gwbeta1.Kind("other kind"),
								Namespace: "route-namespace",
							},
						},
						To: []gwbeta1.ReferenceGrantTo{
							{
								Kind: serviceKind,
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testutils.GenerateTestClient()
			for _, ref := range tc.referenceGrants {
				err := k8sClient.Create(context.Background(), &ref)
				assert.NoError(t, err)
			}

			result, err := referenceGrantCheck(context.Background(), k8sClient, tc.svcIdentifier, tc.routeIdentifier, kind)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
