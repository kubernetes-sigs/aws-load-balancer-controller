package routeutils

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestGatewayBackendConfig_GetTargetType(t *testing.T) {
	config := &GatewayBackendConfig{}

	tests := []struct {
		name         string
		defaultType  elbv2model.TargetType
		expectedType elbv2model.TargetType
	}{
		{
			name:         "returns ALB regardless of default",
			defaultType:  elbv2model.TargetTypeInstance,
			expectedType: elbv2model.TargetTypeALB,
		},
		{
			name:         "returns ALB with IP default",
			defaultType:  elbv2model.TargetTypeIP,
			expectedType: elbv2model.TargetTypeALB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.GetTargetType(tt.defaultType)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

func TestGatewayBackendConfig_GetTargetGroupProps(t *testing.T) {
	props := &elbv2gw.TargetGroupProps{}
	config := &GatewayBackendConfig{targetGroupProps: props}

	assert.Equal(t, props, config.GetTargetGroupProps())
}

func TestGatewayBackendConfig_GetBackendNamespacedName(t *testing.T) {
	gateway := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway",
			Namespace: "test-namespace",
		},
	}
	config := &GatewayBackendConfig{gateway: gateway}

	expected := types.NamespacedName{
		Name:      "test-gateway",
		Namespace: "test-namespace",
	}

	assert.Equal(t, expected, config.GetBackendNamespacedName())
}

func TestGatewayBackendConfig_GetIdentifierPort(t *testing.T) {
	config := &GatewayBackendConfig{port: 8080}

	expected := intstr.FromInt32(8080)
	assert.Equal(t, expected, config.GetIdentifierPort())
}

func TestGatewayBackendConfig_GetExternalTrafficPolicy(t *testing.T) {
	config := &GatewayBackendConfig{}

	result := config.GetExternalTrafficPolicy()
	assert.Equal(t, corev1.ServiceExternalTrafficPolicyTypeCluster, result)
}

func TestGatewayBackendConfig_GetIPAddressType(t *testing.T) {
	config := &GatewayBackendConfig{}

	result := config.GetIPAddressType()
	assert.Equal(t, elbv2model.TargetGroupIPAddressTypeIPv4, result)
}

func TestGatewayBackendConfig_GetTargetGroupPort(t *testing.T) {
	config := &GatewayBackendConfig{port: 9090}

	tests := []struct {
		name       string
		targetType elbv2model.TargetType
		expected   int32
	}{
		{
			name:       "ALB target type",
			targetType: elbv2model.TargetTypeALB,
			expected:   9090,
		},
		{
			name:       "Instance target type",
			targetType: elbv2model.TargetTypeInstance,
			expected:   9090,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.GetTargetGroupPort(tt.targetType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGatewayBackendConfig_GetHealthCheckPort(t *testing.T) {
	tests := []struct {
		name             string
		targetGroupProps *elbv2gw.TargetGroupProps
		port             int32
		targetType       elbv2model.TargetType
		useNodePort      bool
		expectedPort     intstr.IntOrString
		expectedError    bool
	}{
		{
			name:             "no target group props",
			targetGroupProps: nil,
			port:             8080,
			targetType:       elbv2model.TargetTypeALB,
			useNodePort:      false,
			expectedPort:     intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
			expectedError:    false,
		},
		{
			name: "no health check config",
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: nil,
			},
			port:          8080,
			targetType:    elbv2model.TargetTypeALB,
			useNodePort:   false,
			expectedPort:  intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
			expectedError: false,
		},
		{
			name: "no health check port config",
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: nil,
				},
			},
			port:          8080,
			targetType:    elbv2model.TargetTypeALB,
			useNodePort:   false,
			expectedPort:  intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
			expectedError: false,
		},
		{
			name: "health check port set to traffic-port",
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String(shared_constants.HealthCheckPortTrafficPort),
				},
			},
			port:          8080,
			targetType:    elbv2model.TargetTypeALB,
			useNodePort:   false,
			expectedPort:  intstr.FromString(shared_constants.HealthCheckPortTrafficPort),
			expectedError: false,
		},
		{
			name: "health check port set to custom value",
			targetGroupProps: &elbv2gw.TargetGroupProps{
				HealthCheckConfig: &elbv2gw.HealthCheckConfiguration{
					HealthCheckPort: awssdk.String("9090"),
				},
			},
			port:          8080,
			targetType:    elbv2model.TargetTypeALB,
			useNodePort:   false,
			expectedPort:  intstr.FromInt32(8080),
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &GatewayBackendConfig{
				targetGroupProps: tt.targetGroupProps,
				port:             tt.port,
			}

			result, err := config.GetHealthCheckPort(tt.targetType, tt.useNodePort)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPort, result)
			}
		})
	}
}

func TestValidateGatewayARN(t *testing.T) {
	tests := []struct {
		name    string
		arn     string
		wantErr bool
	}{
		{
			name:    "valid ALB ARN",
			arn:     "arn:aws:elasticloadbalancing:us-east-1:565768096483:loadbalancer/app/k8s-echoserv-testgwal-3c92fc24ed/9604d5627427405c",
			wantErr: false,
		},
		{
			name:    "invalid NLB ARN",
			arn:     "arn:aws:elasticloadbalancing:us-east-1:565768096483:loadbalancer/net/my-nlb/1234567890123456",
			wantErr: true,
		},
		{
			name:    "invalid format - no slashes",
			arn:     "arn:aws:elasticloadbalancing:us-east-1:565768096483:loadbalancer",
			wantErr: true,
		},
		{
			name:    "invalid format - only one part",
			arn:     "arn:aws:elasticloadbalancing:us-east-1:565768096483:loadbalancer/",
			wantErr: true,
		},
		{
			name:    "empty string",
			arn:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGatewayARN(tt.arn)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGatewayARN() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
