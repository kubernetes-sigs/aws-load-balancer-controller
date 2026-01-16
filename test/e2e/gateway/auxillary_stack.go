package gateway

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func newAuxiliaryResourceStack(ctx context.Context, f *framework.Framework, tgSpec elbv2gw.TargetGroupConfigurationSpec, enablePodReadinessGate bool) *auxiliaryResourceStack {

	dps := []*appsv1.Deployment{buildDeploymentSpec(f.Options.TestImageRegistry)}
	svcs := []*corev1.Service{buildServiceSpec(map[string]string{})}
	tgcs := []*elbv2gw.TargetGroupConfiguration{buildTargetGroupConfig("aux", tgSpec, svcs[0])}

	ns, err := allocateNamespace(ctx, f, "auxiliary", getNamespaceLabels(enablePodReadinessGate))
	if err != nil {
		panic(err)
	}

	return &auxiliaryResourceStack{
		dps:  dps,
		svcs: svcs,
		tgcs: tgcs,
		ns:   ns,
	}
}

// auxiliaryResourceStack contains resources that are not specific to a load balancer
type auxiliaryResourceStack struct {
	// configurations
	svcs      []*corev1.Service
	dps       []*appsv1.Deployment
	tgcs      []*elbv2gw.TargetGroupConfiguration
	refGrants []*gwbeta1.ReferenceGrant

	// runtime variables
	ns *corev1.Namespace
}

func (s *auxiliaryResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {

	for _, v := range s.dps {
		v.Namespace = s.ns.Name
	}

	for _, v := range s.svcs {
		v.Namespace = s.ns.Name
	}

	for _, v := range s.tgcs {
		v.Namespace = s.ns.Name
	}

	for _, v := range s.refGrants {
		v.Namespace = s.ns.Name
	}

	if err := createTargetGroupConfigs(ctx, f, s.tgcs); err != nil {
		return err
	}
	if err := createDeployments(ctx, f, s.dps); err != nil {
		return err
	}
	if err := createServices(ctx, f, s.svcs); err != nil {
		return err
	}
	return nil
}

func (s *auxiliaryResourceStack) CreateReferenceGrants(ctx context.Context, f *framework.Framework, mainNamespace *corev1.Namespace) error {
	refGrants := []*gwbeta1.ReferenceGrant{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "refgrant",
				Namespace: s.ns.Name,
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
	s.refGrants = refGrants

	if err := CreateReferenceGrants(ctx, f, s.refGrants); err != nil {
		return err
	}

	return nil
}

func (s *auxiliaryResourceStack) DeleteReferenceGrants(ctx context.Context, f *framework.Framework) error {
	return deleteReferenceGrants(ctx, f, s.refGrants)
}

func (s *auxiliaryResourceStack) Cleanup(ctx context.Context, f *framework.Framework) {
	if s.ns != nil {
		_ = deleteNamespace(ctx, f, s.ns)
	}
}
