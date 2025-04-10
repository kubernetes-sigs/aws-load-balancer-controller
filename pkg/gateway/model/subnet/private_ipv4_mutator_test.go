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

func Test_PrivateIPv4Mutator(t *testing.T) {
	testCases := []struct {
		name         string
		subnetConfig []elbv2gw.SubnetConfiguration

		resolvedCidrs    []netip.Prefix
		resolvedCidrsErr error

		filteredIPs []netip.Addr

		returnNoValidIPs bool

		expectedPrivateIPv4 []*string
		expectErr           bool
	}{
		{
			name: "no subnet config",
			expectedPrivateIPv4: []*string{
				nil, nil, nil,
			},
		},
		{
			name: "subnet config but no ipv4 addrs",
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
			expectedPrivateIPv4: []*string{
				nil, nil, nil,
			},
		},
		{
			name: "subnet config with ipv4 addrs",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:            "foo1",
					PrivateIPv4Allocation: awssdk.String("127.0.0.1"),
				},
				{
					Identifier:            "foo2",
					PrivateIPv4Allocation: awssdk.String("127.0.0.2"),
				},
				{
					Identifier:            "foo3",
					PrivateIPv4Allocation: awssdk.String("127.0.0.3"),
				},
			},
			resolvedCidrs: []netip.Prefix{},
			expectedPrivateIPv4: []*string{
				awssdk.String("127.0.0.1"), awssdk.String("127.0.0.2"), awssdk.String("127.0.0.3"),
			},
		},
		{
			name: "ip doesnt belong to cidr",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:            "foo1",
					PrivateIPv4Allocation: awssdk.String("127.0.0.1"),
				},
				{
					Identifier:            "foo2",
					PrivateIPv4Allocation: awssdk.String("127.0.0.2"),
				},
				{
					Identifier:            "foo3",
					PrivateIPv4Allocation: awssdk.String("127.0.0.3"),
				},
			},
			returnNoValidIPs: true,
			expectErr:        true,
		},
		{
			name: "cidr resolver error",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:            "foo1",
					PrivateIPv4Allocation: awssdk.String("127.0.0.1"),
				},
				{
					Identifier:            "foo2",
					PrivateIPv4Allocation: awssdk.String("127.0.0.2"),
				},
				{
					Identifier:            "foo3",
					PrivateIPv4Allocation: awssdk.String("127.0.0.3"),
				},
			},
			resolvedCidrsErr: errors.New("bad thing"),
			expectErr:        true,
		},
		{
			name: "subnet config with ipv4 addrs - allocation is not an ip address",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:            "foo1",
					PrivateIPv4Allocation: awssdk.String("foo"),
				},
				{
					Identifier:            "foo2",
					PrivateIPv4Allocation: awssdk.String("127.0.0.2"),
				},
				{
					Identifier:            "foo3",
					PrivateIPv4Allocation: awssdk.String("127.0.0.3"),
				},
			},
			expectErr: true,
		},
		{
			name: "subnet config with ipv6 addrs - allocation is not an ipv4 address",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:            "foo1",
					PrivateIPv4Allocation: awssdk.String("2600:1f13:837:8504::2"),
				},
				{
					Identifier:            "foo2",
					PrivateIPv4Allocation: awssdk.String("127.0.0.2"),
				},
				{
					Identifier:            "foo3",
					PrivateIPv4Allocation: awssdk.String("127.0.0.3"),
				},
			},
			expectErr: true,
		},
		{
			name: "mismatch subnet config length should trigger error",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:            "foo1",
					PrivateIPv4Allocation: awssdk.String("127.0.0.2"),
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

			m := privateIPv4Mutator{
				prefixResolver: func(subnet ec2types.Subnet) ([]netip.Prefix, error) {
					return tc.resolvedCidrs, tc.resolvedCidrsErr
				},
				ipCidrFilter: func(ips []netip.Addr, cidrs []netip.Prefix) []netip.Addr {

					if tc.returnNoValidIPs {
						return []netip.Addr{}
					}

					assert.Equal(t, tc.resolvedCidrs, cidrs)
					ip := tc.expectedPrivateIPv4[filterIndx]
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

			for i, expected := range tc.expectedPrivateIPv4 {
				assert.Equal(t, expected, subnets[i].PrivateIPv4Address)
			}
		})
	}
}
