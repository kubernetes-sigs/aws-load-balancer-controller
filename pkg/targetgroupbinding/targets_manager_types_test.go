package targetgroupbinding

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/stretchr/testify/assert"
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
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: nil,
			},
			want: false,
		},
		{
			name: "target with initial state and elbRegistrationInProgress reason",
			target: TargetInfo{
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: &elbv2sdk.TargetHealth{
					Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
					State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
				},
			},
			want: false,
		},
		{
			name: "target with healthy state",
			target: TargetInfo{
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: &elbv2sdk.TargetHealth{
					State: awssdk.String(elbv2sdk.TargetHealthStateEnumHealthy),
				},
			},
			want: true,
		},
		{
			name: "target with unhealthy state and targetTimeout reason",
			target: TargetInfo{
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: &elbv2sdk.TargetHealth{
					Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetTimeout),
					State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnhealthy),
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
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: nil,
			},
			want: false,
		},
		{
			name: "target with unused state and targetNotInUse reason",
			target: TargetInfo{
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: &elbv2sdk.TargetHealth{
					Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetNotInUse),
					State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnused),
				},
			},
			want: false,
		},
		{
			name: "target with unused state and targetNotRegistered reason",
			target: TargetInfo{
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: &elbv2sdk.TargetHealth{
					Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetNotRegistered),
					State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnused),
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
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: &elbv2sdk.TargetHealth{
					Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbInitialHealthChecking),
					State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
				},
			},
			want: true,
		},
		{
			name: "target with initial state and elb registrationInProgress reason",
			target: TargetInfo{
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: &elbv2sdk.TargetHealth{
					Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumElbRegistrationInProgress),
					State:  awssdk.String(elbv2sdk.TargetHealthStateEnumInitial),
				},
			},
			want: true,
		},
		{
			name: "target with unknown TargetHealth",
			target: TargetInfo{
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: nil,
			},
			want: false,
		},
		{
			name: "target with unused state and targetNotInUse reason",
			target: TargetInfo{
				Target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
				TargetHealth: &elbv2sdk.TargetHealth{
					Reason: awssdk.String(elbv2sdk.TargetHealthReasonEnumTargetNotInUse),
					State:  awssdk.String(elbv2sdk.TargetHealthStateEnumUnused),
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
		target elbv2sdk.TargetDescription
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "instance target",
			args: args{
				target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("i-038a5c60b6c3c7799"),
					Port: awssdk.Int64(8080),
				},
			},
			want: "i-038a5c60b6c3c7799:8080",
		},
		{
			name: "instance target - with AZ info",
			args: args{
				target: elbv2sdk.TargetDescription{
					Id:               awssdk.String("i-038a5c60b6c3c7799"),
					Port:             awssdk.Int64(8080),
					AvailabilityZone: awssdk.String("all"),
				},
			},
			want: "i-038a5c60b6c3c7799:8080",
		},
		{
			name: "ip target",
			args: args{
				target: elbv2sdk.TargetDescription{
					Id:   awssdk.String("192.168.1.1"),
					Port: awssdk.Int64(8080),
				},
			},
			want: "192.168.1.1:8080",
		},
		{
			name: "ip target - with AZ info",
			args: args{
				target: elbv2sdk.TargetDescription{
					Id:               awssdk.String("192.168.1.1"),
					Port:             awssdk.Int64(8080),
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
