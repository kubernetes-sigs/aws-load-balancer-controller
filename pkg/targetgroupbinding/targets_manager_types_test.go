package targetgroupbinding

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/backend"
	"testing"
)

func TestTargetInfo_IsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		target TargetInfo
		want   bool
	}{
		{
			name: "target with unknown TargetHealth",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: nil,
			},
			want: false,
		},
		{
			name: "target with initial state and elbRegistrationInProgress reason",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: &elbv2types.TargetHealth{
					Reason: elbv2types.TargetHealthReasonEnumRegistrationInProgress,
					State:  elbv2types.TargetHealthStateEnumInitial,
				},
			},
			want: false,
		},
		{
			name: "target with healthy state",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: &elbv2types.TargetHealth{
					State: elbv2types.TargetHealthStateEnumHealthy,
				},
			},
			want: true,
		},
		{
			name: "target with unhealthy state and targetTimeout reason",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: &elbv2types.TargetHealth{
					Reason: elbv2types.TargetHealthReasonEnumTimeout,
					State:  elbv2types.TargetHealthStateEnumUnhealthy,
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.target.IsHealthy()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTargetInfo_IsNotRegistered(t *testing.T) {
	tests := []struct {
		name   string
		target TargetInfo
		want   bool
	}{
		{
			name: "target with unknown TargetHealth",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: nil,
			},
			want: false,
		},
		{
			name: "target with unused state and targetNotInUse reason",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: &elbv2types.TargetHealth{
					Reason: elbv2types.TargetHealthReasonEnumNotInUse,
					State:  elbv2types.TargetHealthStateEnumUnused,
				},
			},
			want: false,
		},
		{
			name: "target with unused state and targetNotRegistered reason",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: &elbv2types.TargetHealth{
					Reason: elbv2types.TargetHealthReasonEnumNotRegistered,
					State:  elbv2types.TargetHealthStateEnumUnused,
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.target.IsNotRegistered()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTargetInfo_IsInitial(t *testing.T) {
	tests := []struct {
		name   string
		target TargetInfo
		want   bool
	}{
		{
			name: "target with initial state and initial healthCheck reason",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: &elbv2types.TargetHealth{
					Reason: elbv2types.TargetHealthReasonEnumInitialHealthChecking,
					State:  elbv2types.TargetHealthStateEnumInitial,
				},
			},
			want: true,
		},
		{
			name: "target with initial state and elb registrationInProgress reason",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: &elbv2types.TargetHealth{
					Reason: elbv2types.TargetHealthReasonEnumRegistrationInProgress,
					State:  elbv2types.TargetHealthStateEnumInitial,
				},
			},
			want: true,
		},
		{
			name: "target with unknown TargetHealth",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: nil,
			},
			want: false,
		},
		{
			name: "target with unused state and targetNotInUse reason",
			target: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
				TargetHealth: &elbv2types.TargetHealth{
					Reason: elbv2types.TargetHealthReasonEnumNotInUse,
					State:  elbv2types.TargetHealthStateEnumUnused,
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			got := tt.target.IsInitial()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUniqueIDForTargetDescription(t *testing.T) {
	type args struct {
		target elbv2types.TargetDescription
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "instance target",
			args: args{
				target: elbv2types.TargetDescription{
					Id:   awssdk.String("i-038a5c60b6c3c7799"),
					Port: awssdk.Int32(8080),
				},
			},
			want: "i-038a5c60b6c3c7799:8080",
		},
		{
			name: "instance target - with AZ info",
			args: args{
				target: elbv2types.TargetDescription{
					Id:               awssdk.String("i-038a5c60b6c3c7799"),
					Port:             awssdk.Int32(8080),
					AvailabilityZone: awssdk.String("all"),
				},
			},
			want: "i-038a5c60b6c3c7799:8080",
		},
		{
			name: "ip target",
			args: args{
				target: elbv2types.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int32(8080),
				},
			},
			want: "192.168.1.1:8080",
		},
		{
			name: "ip target - with AZ info",
			args: args{
				target: elbv2types.TargetDescription{
					Id:               awssdk.String("192.168.1.1"),
					Port:             awssdk.Int32(8080),
					AvailabilityZone: awssdk.String("all"),
				},
			},
			want: "192.168.1.1:8080",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UniqueIDForTargetDescription(tt.args.target)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   backend.Endpoint
		targetInfo TargetInfo
		want       string
	}{
		{
			name: "instance",
			endpoint: backend.NodePortEndpoint{
				InstanceID: "i-12345",
				Port:       80,
			},
			targetInfo: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("i-12345"),
					Port: awssdk.Int32(80),
				},
			},
			want: "i-12345:80",
		},
		{
			name: "ip",
			endpoint: backend.PodEndpoint{
				IP:   "127.0.0.1",
				Port: 80,
			},
			targetInfo: TargetInfo{
				Target: elbv2types.TargetDescription{
					Id:   awssdk.String("127.0.0.1"),
					Port: awssdk.Int32(80),
				},
			},
			want: "127.0.0.1:80",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.targetInfo.GetIdentifier())
			assert.Equal(t, tt.want, tt.endpoint.GetIdentifier(false))
		})
	}
}
