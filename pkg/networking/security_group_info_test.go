package networking

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIPPermissionInfo_HashCode(t *testing.T) {
	type fields struct {
		Permission ec2types.IpPermission
		Labels     map[string]string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "IpRange permission",
			fields: fields{
				Permission: ec2types.IpPermission{
					IpProtocol: awssdk.String("tcp"),
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(8080),
					IpRanges: []ec2types.IpRange{
						{
							CidrIp: awssdk.String("192.168.0.0/16"),
						},
					},
				},
			},
			want: "IpProtocol: tcp, FromPort: 80, ToPort: 8080, IpRange: 192.168.0.0/16",
		},
		{
			name: "Ipv6Range permission",
			fields: fields{
				Permission: ec2types.IpPermission{
					IpProtocol: awssdk.String("tcp"),
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(8080),
					Ipv6Ranges: []ec2types.Ipv6Range{
						{
							CidrIpv6: awssdk.String("::/0"),
						},
					},
				},
			},
			want: "IpProtocol: tcp, FromPort: 80, ToPort: 8080, Ipv6Range: ::/0",
		},
		{
			name: "PrefixListId permission",
			fields: fields{
				Permission: ec2types.IpPermission{
					IpProtocol: awssdk.String("tcp"),
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(8080),
					PrefixListIds: []ec2types.PrefixListId{
						{
							PrefixListId: awssdk.String("pl-123456abcde123456"),
						},
					},
				},
			},
			want: "IpProtocol: tcp, FromPort: 80, ToPort: 8080, PrefixListId: pl-123456abcde123456",
		},
		{
			name: "UserIdGroupPair permission",
			fields: fields{
				Permission: ec2types.IpPermission{
					IpProtocol: awssdk.String("tcp"),
					FromPort:   awssdk.Int32(80),
					ToPort:     awssdk.Int32(8080),
					UserIdGroupPairs: []ec2types.UserIdGroupPair{
						{
							GroupId: awssdk.String("sg-xxxx"),
						},
					},
				},
			},
			want: "IpProtocol: tcp, FromPort: 80, ToPort: 8080, UserIdGroupPair: sg-xxxx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perm := &IPPermissionInfo{
				Permission: tt.fields.Permission,
				Labels:     tt.fields.Labels,
			}
			got := perm.HashCode()
			assert.Equal(t, tt.want, got)
		})
	}
}
