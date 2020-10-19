package networking

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_defaultSecurityGroupReconciler_shouldRetryWithoutCache(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should retry without cache when got duplicated permission error",
			args: args{
				err: awserr.New("InvalidPermission.Duplicate", "", nil),
			},
			want: true,
		},
		{
			name: "should retry without cache when got not found permission error",
			args: args{
				err: awserr.New("InvalidPermission.NotFound", "", nil),
			},
			want: true,
		},
		{
			name: "shouldn't retry when got some other error",
			args: args{
				err: awserr.New("SomeOtherError", "", nil),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &defaultSecurityGroupReconciler{}
			got := r.shouldRetryWithoutCache(tt.args.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_diffIPPermissionInfos(t *testing.T) {
	type args struct {
		source []IPPermissionInfo
		target []IPPermissionInfo
	}
	tests := []struct {
		name string
		args args
		want []IPPermissionInfo
	}{
		{
			name: "source contains more than target",
			args: args{
				source: []IPPermissionInfo{
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.168.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.171.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.170.0.0/16"),
								},
							},
						},
					},
				},
				target: []IPPermissionInfo{
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.168.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.170.0.0/16"),
								},
							},
						},
					},
				},
			},
			want: []IPPermissionInfo{
				{
					Permission: ec2sdk.IpPermission{
						IpProtocol: awssdk.String("tcp"),
						FromPort:   awssdk.Int64(80),
						ToPort:     awssdk.Int64(8080),
						IpRanges: []*ec2sdk.IpRange{
							{
								CidrIp: awssdk.String("192.171.0.0/16"),
							},
						},
					},
				},
			},
		},
		{
			name: "source equals to target",
			args: args{
				source: []IPPermissionInfo{
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.168.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.170.0.0/16"),
								},
							},
						},
					},
				},
				target: []IPPermissionInfo{
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.168.0.0/16"),
								},
							},
						},
					},
					{
						Permission: ec2sdk.IpPermission{
							IpProtocol: awssdk.String("tcp"),
							FromPort:   awssdk.Int64(80),
							ToPort:     awssdk.Int64(8080),
							IpRanges: []*ec2sdk.IpRange{
								{
									CidrIp: awssdk.String("192.170.0.0/16"),
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "both source & target is nil",
			args: args{
				source: nil,
				target: nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffIPPermissionInfos(tt.args.source, tt.args.target)
			assert.Equal(t, tt.want, got)
		})
	}
}
