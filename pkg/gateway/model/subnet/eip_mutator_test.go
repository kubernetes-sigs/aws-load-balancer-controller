package subnet

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_EIPMutator(t *testing.T) {
	testCases := []struct {
		name         string
		subnetConfig []elbv2gw.SubnetConfiguration

		expectedEIPAllocations []*string
		expectErr              bool
	}{
		{
			name: "no subnet config",
			expectedEIPAllocations: []*string{
				nil, nil, nil,
			},
		},
		{
			name: "subnet config but no eip allocation",
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
			expectedEIPAllocations: []*string{
				nil, nil, nil,
			},
		},
		{
			name: "subnet config with eip allocation",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:    "foo1",
					EIPAllocation: awssdk.String("alloc1"),
				},
				{
					Identifier:    "foo2",
					EIPAllocation: awssdk.String("alloc2"),
				},
				{
					Identifier:    "foo3",
					EIPAllocation: awssdk.String("alloc3"),
				},
			},
			expectedEIPAllocations: []*string{
				awssdk.String("alloc1"), awssdk.String("alloc2"), awssdk.String("alloc3"),
			},
		},
		{
			name: "mismatch subnet config length should trigger error",
			subnetConfig: []elbv2gw.SubnetConfiguration{
				{
					Identifier:    "foo1",
					EIPAllocation: awssdk.String("alloc1"),
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

			m := eipMutator{}

			err := m.Mutate(subnets, nil, tc.subnetConfig)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			for i, expected := range tc.expectedEIPAllocations {
				assert.Equal(t, expected, subnets[i].AllocationID)
			}
		})
	}
}
