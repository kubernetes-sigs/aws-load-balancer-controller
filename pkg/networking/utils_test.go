package networking

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"inet.af/netaddr"
	"testing"
)

func TestParseCIDRs(t *testing.T) {
	type args struct {
		cidrs []string
	}
	tests := []struct {
		name    string
		args    args
		want    []netaddr.IPPrefix
		wantErr error
	}{
		{
			name: "has one valid CIDR",
			args: args{
				cidrs: []string{"192.168.5.100/16"},
			},
			want: []netaddr.IPPrefix{
				netaddr.MustParseIPPrefix("192.168.5.100/16"),
			},
		},
		{
			name: "has multiple valid CIDRs",
			args: args{
				cidrs: []string{"192.168.5.100/16", "10.100.0.0/16"},
			},
			want: []netaddr.IPPrefix{
				netaddr.MustParseIPPrefix("192.168.5.100/16"),
				netaddr.MustParseIPPrefix("10.100.0.0/16"),
			},
		},
		{
			name: "has one invalid CIDR",
			args: args{
				cidrs: []string{"192.168.5.100/16", "10.100.0.0"},
			},
			wantErr: errors.New("netaddr.ParseIPPrefix(\"10.100.0.0\"): no '/'"),
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
		ip    netaddr.IP
		cidrs []netaddr.IPPrefix
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "ipv4 address within CIDRs",
			args: args{
				ip: netaddr.MustParseIP("192.168.1.42"),
				cidrs: []netaddr.IPPrefix{
					netaddr.MustParseIPPrefix("10.100.0.0/16"),
					netaddr.MustParseIPPrefix("192.168.0.0/16"),
					netaddr.MustParseIPPrefix("2600:1f14:f8c:2700::/56"),
				},
			},
			want: true,
		},
		{
			name: "ipv4 address not within CIDRs",
			args: args{
				ip: netaddr.MustParseIP("172.16.1.42"),
				cidrs: []netaddr.IPPrefix{
					netaddr.MustParseIPPrefix("10.100.0.0/16"),
					netaddr.MustParseIPPrefix("192.168.0.0/16"),
					netaddr.MustParseIPPrefix("2600:1f14:f8c:2700::/56"),
				},
			},
			want: false,
		},
		{
			name: "ipv6 address within CIDRs",
			args: args{
				ip: netaddr.MustParseIP("2600:1f14:f8c:2701:a740::"),
				cidrs: []netaddr.IPPrefix{
					netaddr.MustParseIPPrefix("10.100.0.0/16"),
					netaddr.MustParseIPPrefix("2700:1f14:f8c:2700::/56"),
					netaddr.MustParseIPPrefix("2600:1f14:f8c:2700::/56"),
				},
			},
			want: true,
		},
		{
			name: "ipv6 address not within CIDRs",
			args: args{
				ip: netaddr.MustParseIP("2800:1f14:f8c:2701:a740::"),
				cidrs: []netaddr.IPPrefix{
					netaddr.MustParseIPPrefix("10.100.0.0/16"),
					netaddr.MustParseIPPrefix("2700:1f14:f8c:2700::/56"),
					netaddr.MustParseIPPrefix("2600:1f14:f8c:2700::/56"),
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
