package test_resources

import (
	"context"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	CrossNamespacePort = 5000
)

func NewCommonResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, lrcs []*elbv2gw.ListenerRuleConfiguration, baseName string, namespaceLabels map[string]string) *CommonResourceStack {
	return &CommonResourceStack{
		Dps:             dps,
		Svcs:            svcs,
		Gwc:             gwc,
		Gw:              gw,
		Lbc:             lbc,
		Tgcs:            tgcs,
		Lrcs:            lrcs,
		BaseName:        baseName,
		NamespaceLabels: namespaceLabels,
	}
}

// CommonResourceStack contains resources that are common between nlb / alb gateways
type CommonResourceStack struct {
	// configurations
	Svcs            []*corev1.Service
	Dps             []*appsv1.Deployment
	Gwc             *gwv1.GatewayClass
	Gw              *gwv1.Gateway
	Lbc             *elbv2gw.LoadBalancerConfiguration
	Tgcs            []*elbv2gw.TargetGroupConfiguration
	Lrcs            []*elbv2gw.ListenerRuleConfiguration
	Ns              *corev1.Namespace
	BaseName        string
	NamespaceLabels map[string]string

	// runtime variables
	CreatedGW *gwv1.Gateway
}

func (s *CommonResourceStack) Deploy(ctx context.Context, f *framework.Framework, resourceSpecificCreation func(ctx context.Context, f *framework.Framework, namespace string) error) error {
	ns, err := AllocateNamespace(ctx, f, s.BaseName, s.NamespaceLabels)
	if err != nil {
		return err
	}
	s.Ns = ns

	for _, v := range s.Dps {
		v.Namespace = s.Ns.Name
	}

	for _, v := range s.Svcs {
		v.Namespace = s.Ns.Name
	}

	if s.Tgcs != nil {
		for _, v := range s.Tgcs {
			v.Namespace = s.Ns.Name
		}
	}

	if s.Lrcs != nil {
		for _, v := range s.Lrcs {
			v.Namespace = s.Ns.Name
		}
	}

	s.Gw.Namespace = s.Ns.Name
	s.Lbc.Namespace = s.Ns.Name

	if err := CreateGatewayClass(ctx, f, s.Gwc); err != nil {
		return err
	}
	if err := CreateLoadBalancerConfig(ctx, f, s.Lbc); err != nil {
		return err
	}
	if err := CreateTargetGroupConfigs(ctx, f, s.Tgcs); err != nil {
		return err
	}
	if err := CreateListenerRuleConfigs(ctx, f, s.Lrcs); err != nil {
		return err
	}
	if err := CreateDeployments(ctx, f, s.Dps); err != nil {
		return err
	}
	if err := CreateServices(ctx, f, s.Svcs); err != nil {
		return err
	}

	if err := CreateGateway(ctx, f, s.Gw); err != nil {
		return err
	}

	if err := resourceSpecificCreation(ctx, f, s.Ns.Name); err != nil {
		return err
	}

	if err := WaitUntilDeploymentReady(ctx, f, s.Dps); err != nil {
		return err
	}

	if err := WaitUntilServiceReady(ctx, f, s.Svcs); err != nil {
		return err
	}

	observedGateway, err := WaitUntilGatewayReady(ctx, f, s.Gw)
	if err != nil {
		return err
	}
	s.CreatedGW = observedGateway
	return nil
}

func (s *CommonResourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if err := DeleteNamespace(ctx, f, s.Ns); err != nil {
		return err
	}
	return DeleteGatewayClass(ctx, f, s.Gwc)
}

func (s *CommonResourceStack) GetLoadBalancerIngressHostname() string {
	return s.CreatedGW.Status.Addresses[0].Value
}

func (s *CommonResourceStack) GetListenersPortMap() map[string]string {
	listenersMap := map[string]string{}
	for _, l := range s.CreatedGW.Spec.Listeners {
		listenersMap[strconv.Itoa(int(l.Port))] = string(l.Protocol)
	}
	return listenersMap
}

func CreateDeployments(ctx context.Context, f *framework.Framework, dps []*appsv1.Deployment) error {
	for _, dp := range dps {
		f.Logger.Info("creating deployment", "dp", k8s.NamespacedName(dp))
		if err := f.K8sClient.Create(ctx, dp); err != nil {
			f.Logger.Info("failed to create deployment")
			return err
		}
		f.Logger.Info("created deployment", "dp", k8s.NamespacedName(dp))
	}
	return nil
}

func WaitUntilDeploymentReady(ctx context.Context, f *framework.Framework, dps []*appsv1.Deployment) error {
	for _, dp := range dps {
		f.Logger.Info("waiting until deployment becomes ready", "dp", k8s.NamespacedName(dp))
		_, err := f.DPManager.WaitUntilDeploymentReady(ctx, dp)
		if err != nil {
			f.Logger.Info("failed waiting for deployment")
			return err
		}
		f.Logger.Info("deployment is ready", "dp", k8s.NamespacedName(dp))
	}
	return nil
}

func CreateServices(ctx context.Context, f *framework.Framework, svcs []*corev1.Service) error {
	for _, svc := range svcs {
		f.Logger.Info("creating service", "svc", k8s.NamespacedName(svc))
		if err := f.K8sClient.Create(ctx, svc); err != nil {
			f.Logger.Info("failed to create service")
			return err
		}
		f.Logger.Info("created service", "svc", k8s.NamespacedName(svc))
	}
	return nil
}

func CreateReferenceGrants(ctx context.Context, f *framework.Framework, refGrants []*gwbeta1.ReferenceGrant) error {
	f.Logger.Info("About to create ref grant")
	for _, refg := range refGrants {
		f.Logger.Info("creating ref grant", "refg", k8s.NamespacedName(refg))
		if err := f.K8sClient.Create(ctx, refg); err != nil {
			f.Logger.Error(err, "failed to create ref grant")
			return err
		}
		f.Logger.Info("created ref grant", "refg", k8s.NamespacedName(refg))
	}
	return nil
}

func deleteReferenceGrants(ctx context.Context, f *framework.Framework, refGrants []*gwbeta1.ReferenceGrant) error {
	f.Logger.Info("About to delete ref grant")
	for _, refg := range refGrants {
		f.Logger.Info("deleting ref grant", "refg", k8s.NamespacedName(refg))
		if err := f.K8sClient.Delete(ctx, refg); err != nil {
			f.Logger.Error(err, "failed to delete ref grant")
			return err
		}
		f.Logger.Info("deleted ref grant", "refg", k8s.NamespacedName(refg))
	}
	return nil
}

func CreateGatewayClass(ctx context.Context, f *framework.Framework, gwc *gwv1.GatewayClass) error {
	f.Logger.Info("creating gateway class", "gwc", k8s.NamespacedName(gwc))
	err := f.K8sClient.Create(ctx, gwc)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	if apierrors.IsAlreadyExists(err) {
		f.Logger.Info("gateway class already exists", "gwc", k8s.NamespacedName(gwc))
	}
	return nil
}

func CreateLoadBalancerConfig(ctx context.Context, f *framework.Framework, lbc *elbv2gw.LoadBalancerConfiguration) error {
	f.Logger.Info("creating loadbalancer config", "lbc", k8s.NamespacedName(lbc))
	return f.K8sClient.Create(ctx, lbc)
}

func CreateTargetGroupConfigs(ctx context.Context, f *framework.Framework, tgcs []*elbv2gw.TargetGroupConfiguration) error {
	for _, tgc := range tgcs {
		f.Logger.Info("creating target group config", "tgc", k8s.NamespacedName(tgc))
		err := f.K8sClient.Create(ctx, tgc)
		if err != nil {
			f.Logger.Error(err, "failed to create target group config")
			return err
		}
		f.Logger.Info("created target group config", "tgc", k8s.NamespacedName(tgc))
	}
	return nil
}

func CreateListenerRuleConfigs(ctx context.Context, f *framework.Framework, lrcs []*elbv2gw.ListenerRuleConfiguration) error {
	for _, lrc := range lrcs {
		f.Logger.Info("creating listener rule config", "lrc", k8s.NamespacedName(lrc))
		err := f.K8sClient.Create(ctx, lrc)
		if err != nil {
			f.Logger.Error(err, "failed to create listener rule config")
			return err
		}
		f.Logger.Info("created listener rule config", "tgc", k8s.NamespacedName(lrc))
	}
	return nil
}

func CreateGateway(ctx context.Context, f *framework.Framework, gw *gwv1.Gateway) error {
	f.Logger.Info("creating gateway", "gw", k8s.NamespacedName(gw))
	return f.K8sClient.Create(ctx, gw)
}

func WaitUntilServiceReady(ctx context.Context, f *framework.Framework, svcs []*corev1.Service) error {
	for _, svc := range svcs {
		observedSvc := &corev1.Service{}
		err := f.K8sClient.Get(ctx, k8s.NamespacedName(svc), observedSvc)
		if err != nil {
			f.Logger.Error(err, "unable to observe service go ready")
			return err
		}
	}
	return nil
}

func WaitUntilGatewayReady(ctx context.Context, f *framework.Framework, gw *gwv1.Gateway) (*gwv1.Gateway, error) {
	observedGw := &gwv1.Gateway{}

	err := wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := f.K8sClient.Get(ctx, k8s.NamespacedName(gw), observedGw); err != nil {
			return false, err
		}

		if observedGw.Status.Conditions != nil {
			for _, cond := range observedGw.Status.Conditions {
				if cond.Type == string(gwv1.GatewayConditionProgrammed) && cond.Status == metav1.ConditionTrue {
					return true, nil
				}
			}
		}

		return false, nil
	}, ctx.Done())
	if err != nil {
		return nil, err
	}
	return observedGw, nil
}

func DeleteGatewayClass(ctx context.Context, f *framework.Framework, gwc *gwv1.GatewayClass) error {
	return f.K8sClient.Delete(ctx, gwc)
}

func DeleteNamespace(ctx context.Context, tf *framework.Framework, ns *corev1.Namespace) error {
	tf.Logger.Info("deleting namespace", "ns", k8s.NamespacedName(ns))
	if err := tf.K8sClient.Delete(ctx, ns); err != nil {
		tf.Logger.Info("failed to delete namespace", "ns", k8s.NamespacedName(ns))
		return err
	}
	if err := tf.NSManager.WaitUntilNamespaceDeleted(ctx, ns); err != nil {
		tf.Logger.Info("failed to wait for namespace deletion", "ns", k8s.NamespacedName(ns))
		return err
	}
	tf.Logger.Info("deleted namespace", "ns", k8s.NamespacedName(ns))
	return nil
}
