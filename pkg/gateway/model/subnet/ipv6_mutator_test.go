package subnet

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"net/netip"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_IPv6Mutator(t *testing.T) {
	testCases := []struct {
		name         string
		subnetConfig []elbv2gw.SubnetConfiguration

		resolvedCidrs    []netip.Prefix
		resolvedCidrsErr error

		filteredIPs []netip.Addr

		returnNoValidIPs bool

		expectedIPv6Allocation []*string
		expectErr              bool
	}{
		{
			name: "no subnet config",
			expectedIPv6Allocation: []*string{
				nil, nil, nil,
			},
		},
		{
			name: "subnet config but no ipv6 addrs",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier: "foo1",
				},
				{
					Identifier: "foo2",
				},
				{
					Identifier: "foo3",
				},
			},
			expectedIPv6Allocation: []*string{
				nil, nil, nil,
			},
		},
		{
			name: "subnet config with ipv6 addrs",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:     "foo1",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::1"),
				},
				{
					Identifier:     "foo2",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::2"),
				},
				{
					Identifier:     "foo3",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::3"),
				},
			},
			resolvedCidrs: []netip.Prefix{},
			expectedIPv6Allocation: []*string{
				awssdk.String("2600:1f13:837:8504::1"), awssdk.String("2600:1f13:837:8504::2"), awssdk.String("2600:1f13:837:8504::3"),
			},
		},
		{
			name: "ip doesnt belong to cidr",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:     "foo1",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::1"),
				},
				{
					Identifier:     "foo2",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::2"),
				},
				{
					Identifier:     "foo3",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::3"),
				},
			},
			returnNoValidIPs: true,
			expectErr:        true,
		},
		{
			name: "cidr resolver error",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:     "foo1",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::1"),
				},
				{
					Identifier:     "foo2",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::2"),
				},
				{
					Identifier:     "foo3",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::3"),
				},
			},
			resolvedCidrsErr: errors.New("bad thing"),
			expectErr:        true,
		},
		{
			name: "subnet config with ipv6 addrs - allocation is not an ip address",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:     "foo1",
					IPv6Allocation: awssdk.String("foo"),
				},
				{
					Identifier:     "foo2",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::2"),
				},
				{
					Identifier:     "foo3",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::3"),
				},
			},
			expectErr: true,
		},
		{
			name: "subnet config with ipv6 addrs - allocation is not an ipv6 address",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:     "foo1",
					IPv6Allocation: awssdk.String("127.0.0.1"),
				},
				{
					Identifier:     "foo2",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::2"),
				},
				{
					Identifier:     "foo3",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::3"),
				},
			},
			expectErr: true,
		},
		{
			name: "mismatch subnet config length should trigger error",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:     "foo1",
					IPv6Allocation: awssdk.String("2600:1f13:837:8504::1"),
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			subnets := []*elbv2model.SubnetMapping{
				{
					SubnetID: "foo1",
				},
				{
					SubnetID: "foo2",
				},
				{
					SubnetID: "foo3",
				},
			}

			ec2Subnets := make([]ec2types.Subnet, 0)

			for _, s := range subnets {
				ec2Subnets = append(ec2Subnets, ec2types.Subnet{
					SubnetId: &s.SubnetID,
				})
			}

			filterIndx := 0

			m := ipv6Mutator{
				prefixResolver: func(subnet ec2types.Subnet) ([]netip.Prefix, error) {
					return tc.resolvedCidrs, tc.resolvedCidrsErr
				},
				ipCidrFilter: func(ips []netip.Addr, cidrs []netip.Prefix) []netip.Addr {

					if tc.returnNoValidIPs {
						return []netip.Addr{}
					}

					assert.Equal(t, tc.resolvedCidrs, cidrs)
					ip := tc.expectedIPv6Allocation[filterIndx]
					filterIndx += 1
					return []netip.Addr{netip.MustParseAddr(*ip)}
				},
			}

			err := m.Mutate(subnets, ec2Subnets, tc.subnetConfig)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			for i, expected := range tc.expectedIPv6Allocation {
				assert.Equal(t, expected, subnets[i].IPv6Address)
			}
		})
	}
}
