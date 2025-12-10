package gateway

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"strconv"
)

const (
	crossNamespacePort = 5000
)

func newCommonResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, lrcs []*elbv2gw.ListenerRuleConfiguration, baseName string, enablePodReadinessGate bool) *commonResourceStack {
	return &commonResourceStack{
		dps:                    dps,
		svcs:                   svcs,
		gwc:                    gwc,
		gw:                     gw,
		lbc:                    lbc,
		tgcs:                   tgcs,
		lrcs:                   lrcs,
		baseName:               baseName,
		enablePodReadinessGate: enablePodReadinessGate,
	}
}

// commonResourceStack contains resources that are common between nlb / alb gateways
type commonResourceStack struct {
	// configurations
	svcs                   []*corev1.Service
	dps                    []*appsv1.Deployment
	gwc                    *gwv1.GatewayClass
	gw                     *gwv1.Gateway
	lbc                    *elbv2gw.LoadBalancerConfiguration
	tgcs                   []*elbv2gw.TargetGroupConfiguration
	lrcs                   []*elbv2gw.ListenerRuleConfiguration
	ns                     *corev1.Namespace
	baseName               string
	enablePodReadinessGate bool

	// runtime variables
	createdGW *gwv1.Gateway
}

func (s *commonResourceStack) Deploy(ctx context.Context, f *framework.Framework, resourceSpecificCreation func(ctx context.Context, f *framework.Framework, namespace string) error) error {
	ns, err := allocateNamespace(ctx, f, s.baseName, s.enablePodReadinessGate)
	if err != nil {
		return err
	}
	s.ns = ns

	for _, v := range s.dps {
		v.Namespace = s.ns.Name
	}

	for _, v := range s.svcs {
		v.Namespace = s.ns.Name
	}

	if s.tgcs != nil {
		for _, v := range s.tgcs {
			v.Namespace = s.ns.Name
		}
	}

	if s.lrcs != nil {
		for _, v := range s.lrcs {
			v.Namespace = s.ns.Name
		}
	}

	s.gw.Namespace = s.ns.Name
	s.lbc.Namespace = s.ns.Name

	if err := createGatewayClass(ctx, f, s.gwc); err != nil {
		return err
	}
	if err := createLoadBalancerConfig(ctx, f, s.lbc); err != nil {
		return err
	}
	if err := createTargetGroupConfigs(ctx, f, s.tgcs); err != nil {
		return err
	}
	if err := createListenerRuleConfigs(ctx, f, s.lrcs); err != nil {
		return err
	}
	if err := createDeployments(ctx, f, s.dps); err != nil {
		return err
	}
	if err := createServices(ctx, f, s.svcs); err != nil {
		return err
	}

	if err := createGateway(ctx, f, s.gw); err != nil {
		return err
	}

	if err := resourceSpecificCreation(ctx, f, s.ns.Name); err != nil {
		return err
	}

	if err := waitUntilDeploymentReady(ctx, f, s.dps); err != nil {
		return err
	}

	if err := waitUntilServiceReady(ctx, f, s.svcs); err != nil {
		return err
	}

	observedGateway, err := waitUntilGatewayReady(ctx, f, s.gw)
	if err != nil {
		return err
	}
	s.createdGW = observedGateway
	return nil
}

func (s *commonResourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if err := deleteNamespace(ctx, f, s.ns); err != nil {
		return err
	}
	return deleteGatewayClass(ctx, f, s.gwc)
}

func (s *commonResourceStack) GetLoadBalancerIngressHostname() string {
	return s.createdGW.Status.Addresses[0].Value
}

func (s *commonResourceStack) getListenersPortMap() map[string]string {
	listenersMap := map[string]string{}
	for _, l := range s.createdGW.Spec.Listeners {
		listenersMap[strconv.Itoa(int(l.Port))] = string(l.Protocol)
	}
	return listenersMap
}

func createDeployments(ctx context.Context, f *framework.Framework, dps []*appsv1.Deployment) error {
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

func waitUntilDeploymentReady(ctx context.Context, f *framework.Framework, dps []*appsv1.Deployment) error {
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

func createServices(ctx context.Context, f *framework.Framework, svcs []*corev1.Service) error {
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

func createReferenceGrants(ctx context.Context, f *framework.Framework, refGrants []*gwbeta1.ReferenceGrant) error {
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

func createGatewayClass(ctx context.Context, f *framework.Framework, gwc *gwv1.GatewayClass) error {
	f.Logger.Info("creating gateway class", "gwc", k8s.NamespacedName(gwc))
	return f.K8sClient.Create(ctx, gwc)
}

func createLoadBalancerConfig(ctx context.Context, f *framework.Framework, lbc *elbv2gw.LoadBalancerConfiguration) error {
	f.Logger.Info("creating loadbalancer config", "lbc", k8s.NamespacedName(lbc))
	return f.K8sClient.Create(ctx, lbc)
}

func createTargetGroupConfigs(ctx context.Context, f *framework.Framework, tgcs []*elbv2gw.TargetGroupConfiguration) error {
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

func createListenerRuleConfigs(ctx context.Context, f *framework.Framework, lrcs []*elbv2gw.ListenerRuleConfiguration) error {
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

func createGateway(ctx context.Context, f *framework.Framework, gw *gwv1.Gateway) error {
	f.Logger.Info("creating gateway", "gw", k8s.NamespacedName(gw))
	return f.K8sClient.Create(ctx, gw)
}

func waitUntilServiceReady(ctx context.Context, f *framework.Framework, svcs []*corev1.Service) error {
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

func waitUntilGatewayReady(ctx context.Context, f *framework.Framework, gw *gwv1.Gateway) (*gwv1.Gateway, error) {
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

func deleteGatewayClass(ctx context.Context, f *framework.Framework, gwc *gwv1.GatewayClass) error {
	return f.K8sClient.Delete(ctx, gwc)
}

func deleteNamespace(ctx context.Context, tf *framework.Framework, ns *corev1.Namespace) error {
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
