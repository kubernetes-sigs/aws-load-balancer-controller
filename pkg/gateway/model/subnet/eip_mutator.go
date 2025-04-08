package subnet

import (
	"fmt"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

type eipMutator struct {
}

func NewEIPMutator() Mutator {
	return &eipMutator{}
}

func (mutator *eipMutator) Mutate(elbSubnets []*elbv2model.SubnetMapping, _ []ec2types.Subnet, subnetConfig []elbv2gw.SubnetConfiguration) error {

	// We've already validated it's all or none for EIP Allocations.
	if elbSubnets == nil || len(subnetConfig) == 0 || subnetConfig[0].EIPAllocation == nil {
		return nil
	}

	if len(elbSubnets) != len(subnetConfig) {
		return errors.Errorf("Unable to assign EIP allocations we have %+v subnets and %v EIP allocations", len(elbSubnets), len(subnetConfig))
	}

	for i, elbSubnet := range elbSubnets {
		fmt.Println("HERE!")
		elbSubnet.AllocationID = subnetConfig[i].EIPAllocation
	}
	return nil
}
