package subnet

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_SourceNATMutator(t *testing.T) {
	testCases := []struct {
		name         string
		subnetConfig []elbv2gw.SubnetConfiguration

		validationError error

		expectedSourceNATPrefixes []*string
		expectErr                 bool
	}{
		{
			name: "no subnet config",
			expectedSourceNATPrefixes: []*string{
				nil, nil, nil,
			},
		},
		{
			name: "subnet config but no source nat config",
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
			expectedSourceNATPrefixes: []*string{
				nil, nil, nil,
			},
		},
		{
			name: "subnet config with source nat config",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:          "foo1",
					SourceNatIPv6Prefix: awssdk.String("alloc1"),
				},
				{
					Identifier:          "foo2",
					SourceNatIPv6Prefix: awssdk.String("alloc2"),
				},
				{
					Identifier:          "foo3",
					SourceNatIPv6Prefix: awssdk.String("alloc3"),
				},
			},
			expectedSourceNATPrefixes: []*string{
				awssdk.String("alloc1"), awssdk.String("alloc2"), awssdk.String("alloc3"),
			},
		},
		{
			name: "mismatch subnet config length should trigger error",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:          "foo1",
					SourceNatIPv6Prefix: awssdk.String("alloc1"),
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

			m := sourceNATMutator{
				validator: func(sourceNatIpv6Prefix string, subnet ec2types.Subnet) error {
					return tc.validationError
				},
			}

			err := m.Mutate(subnets, ec2Subnets, tc.subnetConfig)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			for i, expected := range tc.expectedSourceNATPrefixes {
				assert.Equal(t, expected, subnets[i].SourceNatIpv6Prefix)
			}
		})
	}
}
