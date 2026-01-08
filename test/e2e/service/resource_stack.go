package service

import (
	"context"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewResourceStack(dp *appsv1.Deployment, svc *corev1.Service, lbTypeSvcs []*corev1.Service, nonLbTypeSvcs []*corev1.Service, baseName string, namespaceLabels map[string]string) *ResourceStack {
	return &ResourceStack{
		dp:              dp,
		svc:             svc,
		lbTypeSvcs:      lbTypeSvcs,
		nonLbTypeSvcs:   nonLbTypeSvcs,
		baseName:        baseName,
		namespaceLabels: namespaceLabels,
	}
}

// ResourceStack containing the deployment and service resources
type ResourceStack struct {
	// configurations
	svc             *corev1.Service   // base Load balancer type service
	lbTypeSvcs      []*corev1.Service // more Load balancer type services
	nonLbTypeSvcs   []*corev1.Service // Use this for non-load balancer type services
	dp              *appsv1.Deployment
	ns              *corev1.Namespace
	baseName        string
	namespaceLabels map[string]string

	// runtime variables
	createdDP  *appsv1.Deployment
	createdSVC *corev1.Service
}

func (s *ResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	if err := s.allocateNamespace(ctx, f); err != nil {
		return err
	}
	s.dp.Namespace = s.ns.Name
	s.svc.Namespace = s.ns.Name
	for _, svc := range s.lbTypeSvcs {
		svc.Namespace = s.ns.Name
	}
	for _, svc := range s.nonLbTypeSvcs {
		svc.Namespace = s.ns.Name
	}
	if err := s.createDeployment(ctx, f); err != nil {
		return err
	}
	if err := s.createServices(ctx, f); err != nil {
		return err
	}
	if err := s.waitUntilDeploymentReady(ctx, f); err != nil {
		return err
	}
	if err := s.waitUntilBaseServiceReady(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *ResourceStack) UpdateServiceAnnotations(ctx context.Context, f *framework.Framework, svcAnnotations map[string]string) error {
	if err := s.updateServiceAnnotations(ctx, f, svcAnnotations); err != nil {
		return err
	}
	if err := s.waitUntilBaseServiceReady(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *ResourceStack) DeleteServiceAnnotations(ctx context.Context, f *framework.Framework, annotationKeys []string) error {
	if err := s.removeServiceAnnotations(ctx, f, annotationKeys); err != nil {
		return err
	}
	if err := s.waitUntilBaseServiceReady(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *ResourceStack) UpdateServiceTrafficPolicy(ctx context.Context, f *framework.Framework, trafficPolicy corev1.ServiceExternalTrafficPolicyType) error {
	if err := s.updateServiceTrafficPolicy(ctx, f, trafficPolicy); err != nil {
		return err
	}
	if err := s.waitUntilBaseServiceReady(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *ResourceStack) ScaleDeployment(ctx context.Context, f *framework.Framework, numReplicas int32) error {
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

func (s *ResourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if err := s.deleteDeployment(ctx, f); err != nil {
		return err
	}
	if err := s.deleteService(ctx, f); err != nil {
		return err
	}
	if err := s.deleteNamespace(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *ResourceStack) GetLoadBalancerIngressHostname() string {
	return s.GetLoadBalancerIngressHostnameForService(s.createdSVC)
}

func (s *ResourceStack) GetStackName() string {
	return fmt.Sprintf("%v/%v", s.ns.Name, s.svc.Name)
}

func (s *ResourceStack) GetNamespace() string {
	return s.ns.Name
}

func (s *ResourceStack) getListenersPortMap() map[string]string {
	listenersMap := map[string]string{}
	for _, port := range s.createdSVC.Spec.Ports {
		listenersMap[strconv.Itoa(int(port.Port))] = string(port.Protocol)
	}
	return listenersMap
}

func (s *ResourceStack) getTargetGroupNodePortMap() map[string][]string {
	tgPortProtocolMap := map[string][]string{}
	for _, port := range s.createdSVC.Spec.Ports {
		tgPortProtocolMap[strconv.Itoa(int(port.NodePort))] = []string{string(port.Protocol)}
	}
	return tgPortProtocolMap
}

func (s *ResourceStack) getHealthCheckNodePort() string {
	return strconv.Itoa(int(s.svc.Spec.HealthCheckNodePort))
}

func (s *ResourceStack) updateServiceTrafficPolicy(ctx context.Context, f *framework.Framework, trafficPolicy corev1.ServiceExternalTrafficPolicyType) error {
	f.Logger.Info("updating service annotations", "svc", k8s.NamespacedName(s.svc))
	oldSvc := s.svc.DeepCopy()
	s.svc.Spec.ExternalTrafficPolicy = trafficPolicy
	return s.updateService(ctx, f, oldSvc)
}

func (s *ResourceStack) updateServiceAnnotations(ctx context.Context, f *framework.Framework, svcAnnotations map[string]string) error {
	f.Logger.Info("updating service annotations", "svc", k8s.NamespacedName(s.svc))
	oldSvc := s.svc.DeepCopy()
	for key, value := range svcAnnotations {
		s.svc.Annotations[key] = value
	}
	return s.updateService(ctx, f, oldSvc)
}

func (s *ResourceStack) removeServiceAnnotations(ctx context.Context, f *framework.Framework, annotationKeys []string) error {
	f.Logger.Info("removing service annotations", "svc", k8s.NamespacedName(s.svc))
	oldSvc := s.svc.DeepCopy()
	for _, key := range annotationKeys {
		delete(s.svc.Annotations, key)
	}
	return s.updateService(ctx, f, oldSvc)
}

func (s *ResourceStack) updateService(ctx context.Context, f *framework.Framework, oldSvc *corev1.Service) error {
	f.Logger.Info("updating service", "svc", k8s.NamespacedName(s.svc))
	if err := f.K8sClient.Patch(ctx, s.svc, client.MergeFrom(oldSvc)); err != nil {
		f.Logger.Info("failed to update service", "svc", k8s.NamespacedName(s.svc))
		return err
	}
	return nil
}

func (s *ResourceStack) createDeployment(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating deployment", "dp", k8s.NamespacedName(s.dp))
	if err := f.K8sClient.Create(ctx, s.dp); err != nil {
		f.Logger.Info("failed to create deployment")
		return err
	}
	f.Logger.Info("created deployment", "dp", k8s.NamespacedName(s.dp))
	return nil
}

func (s *ResourceStack) waitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
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

func (s *ResourceStack) createServices(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("creating base service", "svc", k8s.NamespacedName(s.svc))
	if err := f.K8sClient.Create(ctx, s.svc); err != nil {
		return err
	}
	f.Logger.Info("created base service", "svc", k8s.NamespacedName(s.svc))

	for _, svc := range s.nonLbTypeSvcs {
		f.Logger.Info("creating target service", "svc", k8s.NamespacedName(svc))
		svc = svc.DeepCopy()
		if err := f.K8sClient.Create(ctx, svc); err != nil {
			return err
		}
		f.Logger.Info("created target service", "svc", k8s.NamespacedName(svc))
	}

	for _, svc := range s.lbTypeSvcs {
		f.Logger.Info("creating another lb type service", "svc", k8s.NamespacedName(svc))
		svc = svc.DeepCopy()
		if err := f.K8sClient.Create(ctx, svc); err != nil {
			return err
		}
		f.Logger.Info("created another lb type service", "svc", k8s.NamespacedName(svc))
	}

	return nil
}

func (s *ResourceStack) waitUntilBaseServiceReady(ctx context.Context, f *framework.Framework) error {
	observedSVC, err := s.waitUntilServiceReady(ctx, f, s.svc)
	if err != nil {
		return err
	}
	s.createdSVC = observedSVC
	return nil
}

func (s *ResourceStack) waitUntilServiceReady(ctx context.Context, f *framework.Framework, svc *corev1.Service) (*corev1.Service, error) {
	f.Logger.Info("waiting until service becomes ready", "svc", k8s.NamespacedName(svc))
	observedSVC, err := f.SVCManager.WaitUntilServiceActive(ctx, svc)
	if err != nil {
		f.Logger.Info("failed waiting for service")
		return nil, err
	}
	return observedSVC, nil
}

func (s *ResourceStack) deleteDeployment(ctx context.Context, f *framework.Framework) error {
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

func (s *ResourceStack) deleteService(ctx context.Context, f *framework.Framework) error {
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

func (s *ResourceStack) allocateNamespace(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("allocating namespace")
	ns, err := f.NSManager.AllocateNamespace(ctx, s.baseName)
	if err != nil {
		return err
	}
	s.ns = ns
	f.Logger.Info("allocated namespace", "nsName", s.ns.Name)
	if s.namespaceLabels != nil && len(s.ns.Labels) > 0 {
		f.Logger.Info("labeling namespace", "nsName", s.ns.Name, "labels", s.namespaceLabels)
		oldNS := s.ns.DeepCopy()
		s.ns.Labels = algorithm.MergeStringMap(s.namespaceLabels, s.ns.Labels)
		err := f.K8sClient.Patch(ctx, ns, client.MergeFrom(oldNS))
		if err != nil {
			return err
		}
		f.Logger.Info("labeled namespace with podReadinessGate injection", "nsName", s.ns.Name)
	}
	return nil
}

func (s *ResourceStack) deleteNamespace(ctx context.Context, tf *framework.Framework) error {
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

func (s *ResourceStack) GetLoadBalancerIngressHostnameForService(svc *corev1.Service) string {
	return svc.Status.LoadBalancer.Ingress[0].Hostname
}
