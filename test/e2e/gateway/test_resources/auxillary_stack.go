package test_resources

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func NewAuxiliaryResourceStack(ctx context.Context, f *framework.Framework, tgSpec elbv2gw.TargetGroupConfigurationSpec, enablePodReadinessGate bool) *AuxiliaryResourceStack {

	dps := []*appsv1.Deployment{BuildDeploymentSpec(f.Options.TestImageRegistry)}
	svcs := []*corev1.Service{BuildServiceSpec(map[string]string{})}
	tgcs := []*elbv2gw.TargetGroupConfiguration{BuildTargetGroupConfig("aux", tgSpec, svcs[0])}

	ns, err := AllocateNamespace(ctx, f, "auxiliary", GetNamespaceLabels(enablePodReadinessGate))
	if err != nil {
		panic(err)
	}

	return &AuxiliaryResourceStack{
		Dps:  dps,
		Svcs: svcs,
		Tgcs: tgcs,
		Ns:   ns,
	}
}

// AuxiliaryResourceStack contains resources that are not specific to a load balancer
type AuxiliaryResourceStack struct {
	// configurations
	Svcs      []*corev1.Service
	Dps       []*appsv1.Deployment
	Tgcs      []*elbv2gw.TargetGroupConfiguration
	RefGrants []*gwbeta1.ReferenceGrant

	// runtime variables
	Ns *corev1.Namespace
}

func (s *AuxiliaryResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {

	for _, v := range s.Dps {
		v.Namespace = s.Ns.Name
	}

	for _, v := range s.Svcs {
		v.Namespace = s.Ns.Name
	}

	for _, v := range s.Tgcs {
		v.Namespace = s.Ns.Name
	}

	for _, v := range s.RefGrants {
		v.Namespace = s.Ns.Name
	}

	if err := CreateTargetGroupConfigs(ctx, f, s.Tgcs); err != nil {
		return err
	}
	if err := CreateDeployments(ctx, f, s.Dps); err != nil {
		return err
	}
	if err := CreateServices(ctx, f, s.Svcs); err != nil {
		return err
	}
	return nil
}

func (s *AuxiliaryResourceStack) CreateReferenceGrants(ctx context.Context, f *framework.Framework, mainNamespace *corev1.Namespace) error {
	refGrants := []*gwbeta1.ReferenceGrant{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "refgrant",
				Namespace: s.Ns.Name,
			},
			Spec: gwbeta1.ReferenceGrantSpec{
				From: []gwbeta1.ReferenceGrantFrom{
					{
						Group:     gwbeta1.Group(gwbeta1.GroupName),
						Kind:      gwbeta1.Kind("HTTPRoute"),
						Namespace: gwbeta1.Namespace(mainNamespace.Name),
					},
					{
						Group:     gwbeta1.Group(gwbeta1.GroupName),
						Kind:      gwbeta1.Kind("TCPRoute"),
						Namespace: gwbeta1.Namespace(mainNamespace.Name),
					},
				},
				To: []gwbeta1.ReferenceGrantTo{
					{
						Kind: "Service",
					},
				},
			},
		},
	}
	s.RefGrants = refGrants

	if err := CreateReferenceGrants(ctx, f, s.RefGrants); err != nil {
		return err
	}

	return nil
}

func (s *AuxiliaryResourceStack) DeleteReferenceGrants(ctx context.Context, f *framework.Framework) error {
	return deleteReferenceGrants(ctx, f, s.RefGrants)
}

func (s *AuxiliaryResourceStack) Cleanup(ctx context.Context, f *framework.Framework) {
	if s.Ns != nil {
		_ = deleteNamespace(ctx, f, s.Ns)
	}
}
