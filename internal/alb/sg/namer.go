package sg

import (
	"fmt"
)

const (
	lbSGNamePtn       = "%s"
	instanceSGNamePtn = "instance-%s"
)

// Namer can name securityGroup related resources.
type Namer interface {
	// NameLbSG generates names for securityGroup we created for loadBalancer
	NameLbSG(loadBalancerID string) string
	// NameInstanceSG generates names for securityGroup we created for ec2-instance
	NameInstanceSG(loadBalancerID string) string
}

type namer struct{}

func (namer *namer) NameLbSG(loadBalancerID string) string {
	return fmt.Sprintf(lbSGNamePtn, loadBalancerID)
}

func (namer *namer) NameInstanceSG(loadBalancerID string) string {
	return fmt.Sprintf(instanceSGNamePtn, loadBalancerID)
}
