package routeutils

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"testing"
)

func Test_buildTargetGroupPort(t *testing.T) {
	testCases := []struct {
		name       string
		targetType elbv2model.TargetType
		svcPort    *corev1.ServicePort
		expected   int32
	}{
		{
			name: "instance",
			svcPort: &corev1.ServicePort{
				NodePort: 8080,
			},
			targetType: elbv2model.TargetTypeInstance,
			expected:   8080,
		},
		{
			name:       "instance - no node port",
			svcPort:    &corev1.ServicePort{},
			targetType: elbv2model.TargetTypeInstance,
			expected:   0,
		},
		{
			name: "ip",
			svcPort: &corev1.ServicePort{
				NodePort:   8080,
				TargetPort: intstr.FromInt32(80),
			},
			targetType: elbv2model.TargetTypeIP,
			expected:   80,
		},
		{
			name: "ip - str port",
			svcPort: &corev1.ServicePort{
				NodePort:   8080,
				TargetPort: intstr.FromString("foo"),
			},
			targetType: elbv2model.TargetTypeIP,
			expected:   1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svcBackend := NewServiceBackendConfig(nil, nil, tc.svcPort)
			res := svcBackend.GetTargetGroupPort(tc.targetType)
			assert.Equal(t, res, tc.expected)
		})
	}
}

func Test_buildTargetGroupHealthCheckPort(t *testing.T) {
	testCases := []struct {
		name                                    string
		isServiceExternalTrafficPolicyTypeLocal bool
		targetGroupProps                        *elbv2gw.TargetGroupProps
		targetType                              elbv2model.TargetType
		svc                                     *corev1.Service
		expected                                intstr.IntOrString
		expectErr                               bool
	}{
		{
			name:                                    "nil props",
			isServiceExternalTrafficPolicyTypeLocal: false,
			expected:                                intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
		},
		{
			name:                                    "nil hc props",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetGroupProps:                        &elbv2gw.TargetGroupProps{},
			expected:                                intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
		},
		{
			name:                                    "nil hc port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{},
			},
			expected: intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
		},
		{
			name:                                    "explicit is use traffic port hc port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String(shared_constants.HealthCheckPortTrafficPort),
				},
			},
			expected: intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
		},
		{
			name:                                    "explicit port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("80"),
				},
			},
			expected: intstr.FromInt32(80),
		},
		{
			name:                                    "resolve str port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "foo",
							TargetPort: intstr.FromInt32(80),
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expected: intstr.FromInt32(80),
		},
		{
			name:                                    "resolve str port - instance",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetType:                              elbv2model.TargetTypeInstance,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "foo",
							TargetPort: intstr.FromInt32(80),
							NodePort:   1000,
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expected: intstr.FromInt32(1000),
		},
		{
			name:                                    "resolve str port - resolves to other str port (error)",
			isServiceExternalTrafficPolicyTypeLocal: false,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "foo",
							TargetPort: intstr.FromString("bar"),
							NodePort:   1000,
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expectErr: true,
		},
		{
			name:                                    "resolve str port - resolves to other str port but instance mode",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetType:                              elbv2model.TargetTypeInstance,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "foo",
							TargetPort: intstr.FromString("bar"),
							NodePort:   1000,
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expected: intstr.FromInt32(1000),
		},
		{
			name:                                    "resolve str port - cant find configured port",
			isServiceExternalTrafficPolicyTypeLocal: false,
			targetType:                              elbv2model.TargetTypeInstance,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "baz",
							TargetPort: intstr.FromString("bar"),
							NodePort:   1000,
						},
					},
				},
			},
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("foo"),
				},
			},
			expectErr: true,
		},
		{
			name:                                    "with ExternalTrafficPolicyTypeLocal and HealthCheckNodePort specified",
			isServiceExternalTrafficPolicyTypeLocal: true,
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					HealthCheckNodePort:   32000,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
				},
			},
			expected: intstr.FromInt32(32000),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svcBackend := NewServiceBackendConfig(tc.svc, tc.targetGroupProps, nil)
			res, err := svcBackend.GetHealthCheckPort(tc.targetType, tc.isServiceExternalTrafficPolicyTypeLocal)
			if tc.expectErr {
				assert.Error(t, err, res)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func Test_buildTargetGroupTargetType(t *testing.T) {
	svcBackend := NewServiceBackendConfig(nil, nil, nil)

	res := svcBackend.GetTargetType(elbv2model.TargetTypeIP)
	assert.Equal(t, elbv2model.TargetTypeIP, res)

	svcBackendEmptyProps := NewServiceBackendConfig(nil, &elbv2gw.TargetGroupProps{}, nil)

	res = svcBackend.GetTargetType(elbv2model.TargetTypeIP)

	res = svcBackendEmptyProps.GetTargetType(elbv2model.TargetTypeIP)
	assert.Equal(t, elbv2model.TargetTypeIP, res)

	inst := elbv2gw.TargetTypeInstance
	svcBackendWithProps := NewServiceBackendConfig(nil, &elbv2gw.TargetGroupProps{
		TargetType: &inst,
	}, nil)
	res = svcBackendWithProps.GetTargetType(elbv2model.TargetTypeIP)
	assert.Equal(t, elbv2model.TargetTypeInstance, res)
}

func Test_GetProtocolVersion(t *testing.T) {
	testCases := []struct {
		name     string
		svcPort  *corev1.ServicePort
		expected *elbv2model.ProtocolVersion
	}{
		{
			name:    "null app protocol version",
			svcPort: &corev1.ServicePort{},
		},
		{
			name: "unknown app protocol version",
			svcPort: &corev1.ServicePort{
				AppProtocol: awssdk.String("foo"),
			},
		},
		{
			name: "supported protocol version",
			svcPort: &corev1.ServicePort{
				AppProtocol: awssdk.String("kubernetes.io/h2c"),
			},
			expected: &http2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svcBackend := NewServiceBackendConfig(nil, nil, tc.svcPort)
			res := svcBackend.GetProtocolVersion()
			assert.Equal(t, res, tc.expected)
		})
	}
}
