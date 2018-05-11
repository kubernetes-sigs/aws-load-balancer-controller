package annotations

type FakeValidator struct {
	VpcId string
	ResolveVPCValidateSubnetsDelegate func() error
	ValidateSecurityGroupsDelegate func() error
	ValidateCertARNDelegate func() error
	ValidateInboundCidrsDelegate func() error
	ValidateSchemeDelegate func() bool
	ValidateWafAclIdDelegate func() error
}

func (fv FakeValidator) ResolveVPCValidateSubnets(a *Annotations) error {
	if fv.ResolveVPCValidateSubnetsDelegate != nil {
		return fv.ResolveVPCValidateSubnetsDelegate()
	}
	a.VPCID = &fv.VpcId
	return nil
}

func (fv FakeValidator) ValidateSecurityGroups(a *Annotations) error {
	if fv.ValidateSecurityGroupsDelegate != nil {
		return fv.ValidateSecurityGroupsDelegate()
	}
	return nil
}

func (fv FakeValidator) ValidateCertARN(a *Annotations) error {
	if fv.ValidateCertARNDelegate != nil {
		return fv.ValidateCertARNDelegate()
	}
	return nil
}

func (fv FakeValidator) ValidateInboundCidrs(a *Annotations) error {
	if fv.ValidateInboundCidrsDelegate != nil {
		return fv.ValidateCertARNDelegate()
	}
	return nil
}

func (fv FakeValidator) ValidateScheme(a *Annotations, ingressNamespace, ingressName string) bool {
	if fv.ValidateSchemeDelegate != nil {
		return fv.ValidateSchemeDelegate()
	}
	return true
}

func (fv FakeValidator) ValidateWafAclId(a *Annotations) error {
	if fv.ValidateWafAclIdDelegate != nil {
		return fv.ValidateWafAclIdDelegate()
	}
	return nil
}
