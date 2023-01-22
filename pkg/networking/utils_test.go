package networking

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"net/netip"
	"testing"
)

func TestParseCIDRs(t *testing.T) {
	type args struct {
		cidrs []string
	}
	tests := []struct {
		name    string
		args    args
		want    []netip.Prefix
		wantErr error
	}{
		{
			name: "has one valid CIDR",
			args: args{
				cidrs: []string{"192.168.5.100/16"},
			},
			want: []netip.Prefix{
				netip.MustParsePrefix("192.168.5.100/16"),
			},
		},
		{
			name: "has multiple valid CIDRs",
			args: args{
				cidrs: []string{"192.168.5.100/16", "10.100.0.0/16"},
			},
			want: []netip.Prefix{
				netip.MustParsePrefix("192.168.5.100/16"),
				netip.MustParsePrefix("10.100.0.0/16"),
			},
		},
		{
			name: "has one invalid CIDR",
			args: args{
				cidrs: []string{"192.168.5.100/16", "10.100.0.0"},
			},
			wantErr: errors.New("netip.ParsePrefix(\"10.100.0.0\"): no '/'"),
		},
		{
			name: "empty CIDRs",
			args: args{
				cidrs: []string{},
			},
			want: nil,
		},
		{
			name: "nil CIDRs",
			args: args{
				cidrs: nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCIDRs(tt.args.cidrs)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestIsIPWithinCIDRs(t *testing.T) {
	type args struct {
		ip    netip.Addr
		cidrs []netip.Prefix
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "ipv4 address within CIDRs",
			args: args{
				ip: netip.MustParseAddr("192.168.1.42"),
				cidrs: []netip.Prefix{
					netip.MustParsePrefix("10.100.0.0/16"),
					netip.MustParsePrefix("192.168.0.0/16"),
					netip.MustParsePrefix("2600:1f14:f8c:2700::/56"),
				},
			},
			want: true,
		},
		{
			name: "ipv4 address not within CIDRs",
			args: args{
				ip: netip.MustParseAddr("172.16.1.42"),
				cidrs: []netip.Prefix{
					netip.MustParsePrefix("10.100.0.0/16"),
					netip.MustParsePrefix("192.168.0.0/16"),
					netip.MustParsePrefix("2600:1f14:f8c:2700::/56"),
				},
			},
			want: false,
		},
		{
			name: "ipv6 address within CIDRs",
			args: args{
				ip: netip.MustParseAddr("2600:1f14:f8c:2701:a740::"),
				cidrs: []netip.Prefix{
					netip.MustParsePrefix("10.100.0.0/16"),
					netip.MustParsePrefix("2700:1f14:f8c:2700::/56"),
					netip.MustParsePrefix("2600:1f14:f8c:2700::/56"),
				},
			},
			want: true,
		},
		{
			name: "ipv6 address not within CIDRs",
			args: args{
				ip: netip.MustParseAddr("2800:1f14:f8c:2701:a740::"),
				cidrs: []netip.Prefix{
					netip.MustParsePrefix("10.100.0.0/16"),
					netip.MustParsePrefix("2700:1f14:f8c:2700::/56"),
					netip.MustParsePrefix("2600:1f14:f8c:2700::/56"),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsIPWithinCIDRs(tt.args.ip, tt.args.cidrs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterIPsWithinCIDRs(t *testing.T) {
	type args struct {
		ips   []netip.Addr
		cidrs []netip.Prefix
	}
	tests := []struct {
		name string
		args args
		want []netip.Addr
	}{
		{
			name: "ipv4 addresses within one CIDR",
			args: args{
				ips: []netip.Addr{
					netip.MustParseAddr("192.168.1.42"),
					netip.MustParseAddr("192.168.2.42"),
					netip.MustParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7334"),
				},
				cidrs: []netip.Prefix{
					netip.MustParsePrefix("192.168.1.0/24"),
				},
			},
			want: []netip.Addr{
				netip.MustParseAddr("192.168.1.42"),
			},
		},
		{
			name: "ipv4 addresses within multiple CIDRs",
			args: args{
				ips: []netip.Addr{
					netip.MustParseAddr("192.168.1.42"),
					netip.MustParseAddr("192.168.2.42"),
					netip.MustParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7334"),
				},
				cidrs: []netip.Prefix{
					netip.MustParsePrefix("192.168.1.0/24"),
					netip.MustParsePrefix("192.168.0.0/16"),
				},
			},
			want: []netip.Addr{
				netip.MustParseAddr("192.168.1.42"),
				netip.MustParseAddr("192.168.2.42"),
			},
		},
		{
			name: "ipv6 addresses within one CIDR",
			args: args{
				ips: []netip.Addr{
					netip.MustParseAddr("2600:1F13:0837:8500::1"),
					netip.MustParseAddr("2600:1F13:0837:8504::1"),
					netip.MustParseAddr("192.168.1.42"),
				},
				cidrs: []netip.Prefix{
					netip.MustParsePrefix("2600:1f13:837:8500::/64"),
				},
			},
			want: []netip.Addr{
				netip.MustParseAddr("2600:1F13:0837:8500::1"),
			},
		},
		{
			name: "ipv6 addresses within multiple CIDRs",
			args: args{
				ips: []netip.Addr{
					netip.MustParseAddr("2600:1F13:0837:8500::1"),
					netip.MustParseAddr("2600:1F13:0837:8504::1"),
					netip.MustParseAddr("192.168.1.42"),
				},
				cidrs: []netip.Prefix{
					netip.MustParsePrefix("2600:1f13:837:8500::/64"),
					netip.MustParsePrefix("2600:1f13:837:8500::/56"),
				},
			},
			want: []netip.Addr{
				netip.MustParseAddr("2600:1F13:0837:8500::1"),
				netip.MustParseAddr("2600:1F13:0837:8504::1"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterIPsWithinCIDRs(tt.args.ips, tt.args.cidrs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetSubnetAssociatedIPv4CIDRs(t *testing.T) {
	type args struct {
		subnet *ec2sdk.Subnet
	}
	tests := []struct {
		name    string
		args    args
		want    []netip.Prefix
		wantErr error
	}{
		{
			name: "one IPv4 CIDR",
			args: args{
				subnet: &ec2sdk.Subnet{
					CidrBlock: awssdk.String("192.168.1.0/24"),
				},
			},
			want: []netip.Prefix{
				netip.MustParsePrefix("192.168.1.0/24"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetSubnetAssociatedIPv4CIDRs(tt.args.subnet)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestGetSubnetAssociatedIPv6CIDRs(t *testing.T) {
	type args struct {
		subnet *ec2sdk.Subnet
	}
	tests := []struct {
		name    string
		args    args
		want    []netip.Prefix
		wantErr error
	}{
		{
			name: "one IPv6 CIDR",
			args: args{
				subnet: &ec2sdk.Subnet{
					CidrBlock: awssdk.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2sdk.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: awssdk.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2sdk.SubnetCidrBlockState{
								State: awssdk.String(ec2sdk.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
			},
			want: []netip.Prefix{
				netip.MustParsePrefix("2600:1f13:837:8500::/64"),
			},
		},
		{
			name: "multiple IPv6 CIDR",
			args: args{
				subnet: &ec2sdk.Subnet{
					CidrBlock: awssdk.String("192.168.1.0/24"),
					Ipv6CidrBlockAssociationSet: []*ec2sdk.SubnetIpv6CidrBlockAssociation{
						{
							Ipv6CidrBlock: awssdk.String("2600:1f13:837:8500::/64"),
							Ipv6CidrBlockState: &ec2sdk.SubnetCidrBlockState{
								State: awssdk.String(ec2sdk.SubnetCidrBlockStateCodeAssociated),
							},
						},
						{
							Ipv6CidrBlock: awssdk.String("2600:1f13:837:8504::/64"),
							Ipv6CidrBlockState: &ec2sdk.SubnetCidrBlockState{
								State: awssdk.String(ec2sdk.SubnetCidrBlockStateCodeAssociated),
							},
						},
					},
				},
			},
			want: []netip.Prefix{
				netip.MustParsePrefix("2600:1f13:837:8500::/64"),
				netip.MustParsePrefix("2600:1f13:837:8504::/64"),
			},
		},
		{
			name: "zero IPv6 CIDR",
			args: args{
				subnet: &ec2sdk.Subnet{
					CidrBlock: awssdk.String("192.168.1.0/24"),
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetSubnetAssociatedIPv6CIDRs(tt.args.subnet)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
