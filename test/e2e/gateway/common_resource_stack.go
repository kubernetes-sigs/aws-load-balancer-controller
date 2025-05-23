package gateway

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strconv"
	"time"
)

func newCommonResourceStack(dp *appsv1.Deployment, svc *corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, baseName string, enablePodReadinessGate bool) *commonResourceStack {
	return &commonResourceStack{
		dp:                     dp,
		svc:                    svc,
		gwc:                    gwc,
		gw:                     gw,
		lbc:                    lbc,
		baseName:               baseName,
		enablePodReadinessGate: enablePodReadinessGate,
	}
}

// commonResourceStack contains resources that are common between nlb / alb gateways
type commonResourceStack struct {
	// configurations
	svc                    *corev1.Service
	dp                     *appsv1.Deployment
	gwc                    *gwv1.GatewayClass
	gw                     *gwv1.Gateway
	lbc                    *elbv2gw.LoadBalancerConfiguration
	ns                     *corev1.Namespace
	baseName               string
	enablePodReadinessGate bool

	// runtime variables
	createdDP  *appsv1.Deployment
	createdSVC *corev1.Service
	createdGW  *gwv1.Gateway
}

func (s *commonResourceStack) Deploy(ctx context.Context, f *framework.Framework, resourceSpecificCreation func(ctx context.Context, f *framework.Framework, namespace string) error) error {
	if err := s.allocateNamespace(ctx, f); err != nil {
		return err
	}
	s.dp.Namespace = s.ns.Name
	s.svc.Namespace = s.ns.Name
	s.gw.Namespace = s.ns.Name
	s.lbc.Namespace = s.ns.Name

	if err := s.createGatewayClass(ctx, f); err != nil {
		return err
	}
	if err := s.createLoadBalancerConfig(ctx, f); err != nil {
		return err
	}
	if err := s.createDeployment(ctx, f); err != nil {
		return err
	}
	if err := s.createService(ctx, f); err != nil {
		return err
	}
	if err := s.createGateway(ctx, f); err != nil {
		return err
	}

	if err := resourceSpecificCreation(ctx, f, s.ns.Name); err != nil {
		return err
	}
	// TODO -- Fix
	time.Sleep(7 * time.Minute)
	if err := s.waitUntilDeploymentReady(ctx, f); err != nil {
		return err
	}

	if err := s.waitUntilServiceReady(ctx, f); err != nil {
		return err
	}

	if err := s.waitUntilGatewayReady(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *commonResourceStack) ScaleDeployment(ctx context.Context, f *framework.Framework, numReplicas int32) error {
	f.Logger.Info("scaling deployment", "dp", k8s.NamespacedName(s.dp), "currentReplicas", s.dp.Spec.Replicas, "desiredReplicas", numReplicas)
	oldDP := s.dp.DeepCopy()
	s.dp.Spec.Replicas = &numReplicas
	if err := f.K8sClient.Patch(ctx, s.dp, client.MergeFrom(oldDP)); err != nil {
		f.Logger.Info("failed to update deployment", "dp", k8s.NamespacedName(s.dp))
		return err
	}
	if err := s.waitUntilDeploymentReady(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *commonResourceStack) Cleanup(ctx context.Context, f *framework.Framework) {
	_ = s.deleteGateway(ctx, f)
	// todo - fix
	time.Sleep(5 * time.Minute)
	_ = s.deleteNamespace(ctx, f)
	_ = s.deleteGatewayClass(ctx, f)
}

func (s *commonResourceStack) GetLoadBalancerIngressHostname() string {
	return s.createdGW.Status.Addresses[0].Value
}

func (s *commonResourceStack) GetStackName() string {
	return fmt.Sprintf("%v/%v", s.ns.Name, s.svc.Name)
}

func (s *commonResourceStack) getListenersPortMap() map[string]string {
	listenersMap := map[string]string{}
	for _, l := range s.createdGW.Spec.Listeners {
		listenersMap[strconv.Itoa(int(l.Port))] = string(l.Protocol)
	}
	return listenersMap
}

func (s *commonResourceStack) getTargetGroupNodePortMap() map[string]string {
	tgPortProtocolMap := map[string]string{}
	for _, port := range s.createdSVC.Spec.Ports {
		tgPortProtocolMap[strconv.Itoa(int(port.NodePort))] = string(port.Protocol)
	}
	return tgPortProtocolMap
}

func (s *commonResourceStack) getHealthCheckNodePort() string {
	return strconv.Itoa(int(s.svc.Spec.HealthCheckNodePort))
}

func (s *commonResourceStack) createDeployment(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating deployment", "dp", k8s.NamespacedName(s.dp))
	if err := f.K8sClient.Create(ctx, s.dp); err != nil {
		f.Logger.Info("failed to create deployment")
		return err
	}
	f.Logger.Info("created deployment", "dp", k8s.NamespacedName(s.dp))
	return nil
}

func (s *commonResourceStack) waitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("waiting until deployment becomes ready", "dp", k8s.NamespacedName(s.dp))
	observedDP, err := f.DPManager.WaitUntilDeploymentReady(ctx, s.dp)
	if err != nil {
		f.Logger.Info("failed waiting for deployment")
		return err
	}
	f.Logger.Info("deployment is ready", "dp", k8s.NamespacedName(s.dp))
	s.createdDP = observedDP
	return nil
}

func (s *commonResourceStack) createService(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating service", "svc", k8s.NamespacedName(s.svc))
	return f.K8sClient.Create(ctx, s.svc)
}

func (s *commonResourceStack) createGatewayClass(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating gateway class", "gwc", k8s.NamespacedName(s.gwc))
	return f.K8sClient.Create(ctx, s.gwc)
}

func (s *commonResourceStack) createLoadBalancerConfig(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating loadbalancer config", "lbc", k8s.NamespacedName(s.lbc))
	return f.K8sClient.Create(ctx, s.lbc)
}

func (s *commonResourceStack) createGateway(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating gateway", "gw", k8s.NamespacedName(s.gw))
	return f.K8sClient.Create(ctx, s.gw)
}

func (s *commonResourceStack) waitUntilServiceReady(ctx context.Context, f *framework.Framework) error {
	observedSvc := &corev1.Service{}
	err := f.K8sClient.Get(ctx, k8s.NamespacedName(s.svc), observedSvc)
	if err != nil {
		return err
	}
	s.createdSVC = observedSvc
	return nil
}

func (s *commonResourceStack) waitUntilGatewayReady(ctx context.Context, f *framework.Framework) error {
	observedGw := &gwv1.Gateway{}
	err := wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := f.K8sClient.Get(ctx, k8s.NamespacedName(s.gw), observedGw); err != nil {
			return false, err
		}
		if observedGw.Status.Addresses != nil && len(observedGw.Status.Addresses) > 0 {
			return true, nil
		}
		return false, nil
	}, ctx.Done())
	if err != nil {
		return err
	}
	s.createdGW = observedGw
	return nil
}

func (s *commonResourceStack) deleteDeployment(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("deleting deployment", "dp", k8s.NamespacedName(s.dp))
	if err := f.K8sClient.Delete(ctx, s.dp); err != nil {
		f.Logger.Info("failed to delete deployment", "dp", k8s.NamespacedName(s.dp))
		return err
	}
	if err := f.DPManager.WaitUntilDeploymentDeleted(ctx, s.dp); err != nil {
		f.Logger.Info("failed to wait for deployment deletion", "dp", k8s.NamespacedName(s.dp))
		return err
	}
	f.Logger.Info("deleted deployment", "dp", k8s.NamespacedName(s.dp))
	return nil
}

func (s *commonResourceStack) deleteService(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("deleting service", "svc", k8s.NamespacedName(s.svc))
	if err := f.K8sClient.Delete(ctx, s.svc); err != nil {
		f.Logger.Info("failed to delete service", "svc", k8s.NamespacedName(s.svc))
		return err
	}
	if err := f.SVCManager.WaitUntilServiceDeleted(ctx, s.svc); err != nil {
		f.Logger.Info("failed to wait for service deletion", "svc", k8s.NamespacedName(s.svc))
		return err
	}
	f.Logger.Info("deleted service", "svc", k8s.NamespacedName(s.svc))
	return nil
}

func (s *commonResourceStack) deleteGateway(ctx context.Context, f *framework.Framework) error {
	return f.K8sClient.Delete(ctx, s.gw)
}

func (s *commonResourceStack) deleteGatewayClass(ctx context.Context, f *framework.Framework) error {
	return f.K8sClient.Delete(ctx, s.gwc)
}

func (s *commonResourceStack) deleteLoadbalancerConfig(ctx context.Context, f *framework.Framework) error {
	return f.K8sClient.Delete(ctx, s.lbc)
}

func (s *commonResourceStack) allocateNamespace(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("allocating namespace")
	ns, err := f.NSManager.AllocateNamespace(ctx, s.baseName)
	if err != nil {
		return err
	}
	s.ns = ns
	f.Logger.Info("allocated namespace", "nsName", s.ns.Name)
	if s.enablePodReadinessGate {
		f.Logger.Info("label namespace for podReadinessGate injection", "nsName", s.ns.Name)
		oldNS := s.ns.DeepCopy()
		s.ns.Labels = algorithm.MergeStringMap(map[string]string{
			"elbv2.k8s.aws/pod-readiness-gate-inject": "enabled",
		}, s.ns.Labels)
		err := f.K8sClient.Patch(ctx, ns, client.MergeFrom(oldNS))
		if err != nil {
			return err
		}
		f.Logger.Info("labeled namespace with podReadinessGate injection", "nsName", s.ns.Name)
	}
	return nil
}

func (s *commonResourceStack) deleteNamespace(ctx context.Context, tf *framework.Framework) error {
	tf.Logger.Info("deleting namespace", "ns", k8s.NamespacedName(s.ns))
	if err := tf.K8sClient.Delete(ctx, s.ns); err != nil {
		tf.Logger.Info("failed to delete namespace", "ns", k8s.NamespacedName(s.ns))
		return err
	}
	if err := tf.NSManager.WaitUntilNamespaceDeleted(ctx, s.ns); err != nil {
		tf.Logger.Info("failed to wait for namespace deletion", "ns", k8s.NamespacedName(s.ns))
		return err
	}
	tf.Logger.Info("deleted namespace", "ns", k8s.NamespacedName(s.ns))
	return nil
}
