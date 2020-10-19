package networking

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIPPermissionInfo_HashCode(t *testing.T) {
	type fields struct {
		Permission ec2sdk.IpPermission
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
			want: "IpProtocol: tcp, FromPort: 80, ToPort: 8080, IpRange: 192.168.0.0/16",
		},
		{
			name: "Ipv6Range permission",
			fields: fields{
				Permission: ec2sdk.IpPermission{
					IpProtocol: awssdk.String("tcp"),
					FromPort:   awssdk.Int64(80),
					ToPort:     awssdk.Int64(8080),
					Ipv6Ranges: []*ec2sdk.Ipv6Range{
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
				Permission: ec2sdk.IpPermission{
					IpProtocol: awssdk.String("tcp"),
					FromPort:   awssdk.Int64(80),
					ToPort:     awssdk.Int64(8080),
					PrefixListIds: []*ec2sdk.PrefixListId{
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
				Permission: ec2sdk.IpPermission{
					IpProtocol: awssdk.String("tcp"),
					FromPort:   awssdk.Int64(80),
					ToPort:     awssdk.Int64(8080),
					UserIdGroupPairs: []*ec2sdk.UserIdGroupPair{
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
