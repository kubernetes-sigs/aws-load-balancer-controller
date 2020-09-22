package targetgroupbinding

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	elbv2api "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
	"testing"
)

func Test_defaultNetworkingManager_computeIngressPermissionsForTGBNetworking(t *testing.T) {
	port8080 := intstr.FromInt(8080)
	port8443 := intstr.FromInt(8443)
	type args struct {
		tgbNetworking elbv2api.TargetGroupBindingNetworking
		pods          []*corev1.Pod
	}
	tests := []struct {
		name    string
		args    args
		want    []networking.IPPermissionInfo
		wantErr error
	}{
		{
			name: "with one rule / one peer / one port",
			args: args{
				tgbNetworking: elbv2api.TargetGroupBindingNetworking{
					Ingress: []elbv2api.NetworkingIngressRule{
						{
							From: []elbv2api.NetworkingPeer{
								{
									SecurityGroup: &elbv2api.SecurityGroup{
										GroupID: "sg-abcdefg",
									},
								},
							},
							Ports: []elbv2api.NetworkingPort{
								{
									Port: &port8080,
								},
							},
						},
					},
				},
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
		{
			name: "with one rule / multiple peer / multiple port",
			args: args{
				tgbNetworking: elbv2api.TargetGroupBindingNetworking{
					Ingress: []elbv2api.NetworkingIngressRule{
						{
							From: []elbv2api.NetworkingPeer{
								{
									SecurityGroup: &elbv2api.SecurityGroup{
										GroupID: "sg-abcdefg",
									},
								},
								{
									IPBlock: &elbv2api.IPBlock{
										CIDR: "192.168.1.1/16",
									},
								},
							},
							Ports: []elbv2api.NetworkingPort{
								{
									Port: &port8080,
								},
								{
									Port: &port8443,
								},
							},
						},
					},
				},
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(8443),
						ToPort:     awssdk.Int64(8443),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						IpRanges: []*ec2sdk.IpRange{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								CidrIp:      awssdk.String("192.168.1.1/16"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(8443),
						ToPort:     awssdk.Int64(8443),
						IpRanges: []*ec2sdk.IpRange{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								CidrIp:      awssdk.String("192.168.1.1/16"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
		{
			name: "with multiple rule / one peer / one port",
			args: args{
				tgbNetworking: elbv2api.TargetGroupBindingNetworking{
					Ingress: []elbv2api.NetworkingIngressRule{
						{
							From: []elbv2api.NetworkingPeer{
								{
									SecurityGroup: &elbv2api.SecurityGroup{
										GroupID: "sg-abcdefg",
									},
								},
							},
							Ports: []elbv2api.NetworkingPort{
								{
									Port: &port8080,
								},
							},
						},
						{
							From: []elbv2api.NetworkingPeer{
								{
									IPBlock: &elbv2api.IPBlock{
										CIDR: "192.168.1.1/16",
									},
								},
							},
							Ports: []elbv2api.NetworkingPort{
								{
									Port: &port8443,
								},
							},
						},
					},
				},
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(8443),
						ToPort:     awssdk.Int64(8443),
						IpRanges: []*ec2sdk.IpRange{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								CidrIp:      awssdk.String("192.168.1.1/16"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultNetworkingManager{}
			got, err := m.computeIngressPermissionsForTGBNetworking(context.Background(), tt.args.tgbNetworking, tt.args.pods)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultNetworkingManager_computePermissionsForPeerPort(t *testing.T) {
	port8080 := intstr.FromInt(8080)
	portHTTP := intstr.FromString("http")
	protocolUDP := elbv2api.NetworkingProtocolUDP
	type args struct {
		peer elbv2api.NetworkingPeer
		port elbv2api.NetworkingPort
		pods []*corev1.Pod
	}
	tests := []struct {
		name    string
		args    args
		want    []networking.IPPermissionInfo
		wantErr error
	}{
		{
			name: "permission for securityGroup peer",
			args: args{
				peer: elbv2api.NetworkingPeer{
					SecurityGroup: &elbv2api.SecurityGroup{
						GroupID: "sg-abcdefg",
					},
				},
				port: elbv2api.NetworkingPort{
					Protocol: &protocolUDP,
					Port:     &port8080,
				},
				pods: nil,
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("udp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
		{
			name: "permission for IPBlock peer with IPv4 CIDR",
			args: args{
				peer: elbv2api.NetworkingPeer{
					IPBlock: &elbv2api.IPBlock{
						CIDR: "192.168.1.1/16",
					},
				},
				port: elbv2api.NetworkingPort{
					Protocol: &protocolUDP,
					Port:     &port8080,
				},
				pods: nil,
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("udp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						IpRanges: []*ec2sdk.IpRange{
							{
								CidrIp:      awssdk.String("192.168.1.1/16"),
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
		{
			name: "permission for IPBlock peer with IPv6 CIDR",
			args: args{
				peer: elbv2api.NetworkingPeer{
					IPBlock: &elbv2api.IPBlock{
						CIDR: "2002::1234:abcd:ffff:c0a8:101/64",
					},
				},
				port: elbv2api.NetworkingPort{
					Protocol: &protocolUDP,
					Port:     &port8080,
				},
				pods: nil,
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("udp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						Ipv6Ranges: []*ec2sdk.Ipv6Range{
							{
								CidrIpv6:    awssdk.String("2002::1234:abcd:ffff:c0a8:101/64"),
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
		{
			name: "permission when named ports maps to multiple numeric port",
			args: args{
				peer: elbv2api.NetworkingPeer{
					SecurityGroup: &elbv2api.SecurityGroup{
						GroupID: "sg-abcdefg",
					},
				},
				port: elbv2api.NetworkingPort{
					Protocol: &protocolUDP,
					Port:     &portHTTP,
				},
				pods: []*corev1.Pod{
					{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Ports: []corev1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
					{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Ports: []corev1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
										},
									},
								},
							},
						},
					},
				},
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("udp"),
						FromPort:   awssdk.Int64(80),
						ToPort:     awssdk.Int64(80),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("udp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
		{
			name: "permission when protocol defaults to tcp",
			args: args{
				peer: elbv2api.NetworkingPeer{
					SecurityGroup: &elbv2api.SecurityGroup{
						GroupID: "sg-abcdefg",
					},
				},
				port: elbv2api.NetworkingPort{
					Protocol: nil,
					Port:     &port8080,
				},
				pods: nil,
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(8080),
						ToPort:     awssdk.Int64(8080),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
		{
			name: "permission when port defaults to all ports",
			args: args{
				peer: elbv2api.NetworkingPeer{
					SecurityGroup: &elbv2api.SecurityGroup{
						GroupID: "sg-abcdefg",
					},
				},
				port: elbv2api.NetworkingPort{
					Protocol: &protocolUDP,
				},
				pods: nil,
			},
			want: []networking.IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("udp"),
						FromPort:   awssdk.Int64(0),
						ToPort:     awssdk.Int64(65535),
						UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
							{
								Description: awssdk.String("elbv2.k8s.aws/targetGroupBinding=shared"),
								GroupId:     awssdk.String("sg-abcdefg"),
							},
						},
					},
					Labels: map[string]string{tgbNetworkingIPPermissionLabelKey: tgbNetworkingIPPermissionLabelValue},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultNetworkingManager{}
			got, err := m.computePermissionsForPeerPort(context.Background(), tt.args.peer, tt.args.port, tt.args.pods)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultNetworkingManager_computeNumericalPorts(t *testing.T) {
	type args struct {
		port intstr.IntOrString
		pods []*corev1.Pod
	}
	tests := []struct {
		name    string
		args    args
		want    []int64
		wantErr error
	}{
		{
			name: "numerical port can always be resolved",
			args: args{
				port: intstr.FromInt(8080),
				pods: nil,
			},
			want: []int64{8080},
		},
		{
			name: "named port resolves to same numerical port",
			args: args{
				port: intstr.FromString("http"),
				pods: []*corev1.Pod{
					{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Ports: []corev1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
					{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Ports: []corev1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
				},
			},
			want: []int64{80},
		},
		{
			name: "named port resolves to different numerical port",
			args: args{
				port: intstr.FromString("http"),
				pods: []*corev1.Pod{
					{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Ports: []corev1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
					{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Ports: []corev1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 8080,
										},
									},
								},
							},
						},
					},
				},
			},
			want: []int64{80, 8080},
		},
		{
			name: "numerical port cannot be used without pods",
			args: args{
				port: intstr.FromString("http"),
				pods: nil,
			},
			wantErr: errors.New("named ports can only be used with pod endpoints"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultNetworkingManager{}
			got, err := m.computeNumericalPorts(context.Background(), tt.args.port, tt.args.pods)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
