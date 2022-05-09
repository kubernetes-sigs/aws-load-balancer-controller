package ingress

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PathConfig struct {
	Path      string
	BackendID string
}

type BackendConfig struct {
	Replicas   int32
	TargetType elbv2model.TargetType

	HTTPBody string
}

type MultiPathIngressConfig struct {
	GroupName  string
	GroupOrder int64
	PathCFGs   []PathConfig
}

type NamespacedResourcesConfig struct {
	IngCFGs     map[string]MultiPathIngressConfig
	BackendCFGs map[string]BackendConfig
}

func NewMultiPathBackendStack(namespacedResourcesCFGs map[string]NamespacedResourcesConfig, enablePodReadinessGate bool) *multiPathBackendStack {
	return &multiPathBackendStack{
		namespacedResourcesCFGs: namespacedResourcesCFGs,
		enablePodReadinessGate:  enablePodReadinessGate,

		nsByNSID:         make(map[string]*corev1.Namespace),
		resStackByNSID:   make(map[string]*resourceStack),
		ingByIngIDByNSID: make(map[string]map[string]*networking.Ingress),
	}
}

type multiPathBackendStack struct {
	namespacedResourcesCFGs map[string]NamespacedResourcesConfig
	enablePodReadinessGate  bool

	// runtime variables
	nsByNSID         map[string]*corev1.Namespace
	resStackByNSID   map[string]*resourceStack
	ingByIngIDByNSID map[string]map[string]*networking.Ingress
}

func (s *multiPathBackendStack) Deploy(ctx context.Context, f *framework.Framework) error {
	if err := s.allocateNamespaces(ctx, f); err != nil {
		return err
	}
	s.resStackByNSID, s.ingByIngIDByNSID = s.buildResourceStacks(s.namespacedResourcesCFGs, s.nsByNSID, f)
	if err := s.deployResourceStacks(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *multiPathBackendStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if err := s.cleanupResourceStacks(ctx, f); err != nil {
		return err
	}
	if err := s.cleanupNamespaces(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *multiPathBackendStack) FindIngress(nsID string, ingID string) *networking.Ingress {
	if ingByIngID, ok := s.ingByIngIDByNSID[nsID]; ok {
		if ing, ok := ingByIngID[ingID]; ok {
			return ing
		}
	}
	return nil
}

func (s *multiPathBackendStack) allocateNamespaces(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("allocate all namespaces")
	for nsID := range s.namespacedResourcesCFGs {
		f.Logger.Info("allocating namespace", "nsID", nsID)
		ns, err := f.NSManager.AllocateNamespace(ctx, "aws-lb-e2e")
		if err != nil {
			return err
		}
		f.Logger.Info("allocated namespace", "nsID", nsID, "nsName", ns.Name)
		s.nsByNSID[nsID] = ns
	}

	if s.enablePodReadinessGate {
		f.Logger.Info("label all namespaces with podReadinessGate injection")
		for _, ns := range s.nsByNSID {
			f.Logger.Info("labeling namespace with podReadinessGate injection", "nsName", ns.Name)
			oldNS := ns.DeepCopy()
			ns.Labels = algorithm.MergeStringMap(map[string]string{
				"elbv2.k8s.aws/pod-readiness-gate-inject": "enabled",
			}, ns.Labels)
			err := f.K8sClient.Patch(ctx, ns, client.MergeFrom(oldNS))
			if err != nil {
				return err
			}
			f.Logger.Info("labeled namespace with podReadinessGate injection", "nsName", ns.Name)
		}
	}
	return nil
}

func (s *multiPathBackendStack) cleanupNamespaces(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("cleanup all namespaces")
	var cleanupErrs []error
	var cleanupErrsMutex sync.Mutex
	var wg sync.WaitGroup
	for nsID, ns := range s.nsByNSID {
		wg.Add(1)
		go func(nsID string, ns *corev1.Namespace) {
			defer wg.Done()
			f.Logger.Info("deleting namespace", "nsID", nsID, "nsName", ns.Name)
			if err := f.K8sClient.Delete(ctx, ns); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			if err := f.NSManager.WaitUntilNamespaceDeleted(ctx, ns); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			f.Logger.Info("deleted namespace", "nsID", nsID, "nsName", ns.Name)
		}(nsID, ns)
	}

	wg.Wait()
	if len(cleanupErrs) != 0 {
		return utils.NewMultiError(cleanupErrs...)
	}
	return nil
}

func (s *multiPathBackendStack) deployResourceStacks(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("deploy all resource stacks")
	for _, stack := range s.resStackByNSID {
		if err := stack.Deploy(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

func (s *multiPathBackendStack) cleanupResourceStacks(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("cleanup all resource stacks")
	var cleanupErrs []error
	var cleanupErrsMutex sync.Mutex
	var wg sync.WaitGroup

	for nsID, resStack := range s.resStackByNSID {
		wg.Add(1)
		go func(nsID string, resStack *resourceStack) {
			defer wg.Done()
			f.Logger.Info("begin cleanup resource stack", "nsID", nsID)
			if err := resStack.Cleanup(ctx, f); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			f.Logger.Info("end cleanup resource stack", "nsID", nsID)
		}(nsID, resStack)
	}

	wg.Wait()
	if len(cleanupErrs) != 0 {
		return utils.NewMultiError(cleanupErrs...)
	}
	return nil
}

func (s *multiPathBackendStack) buildResourceStacks(namespacedResourcesCFGs map[string]NamespacedResourcesConfig, nsByNSID map[string]*corev1.Namespace, f *framework.Framework) (map[string]*resourceStack, map[string]map[string]*networking.Ingress) {
	resStackByNSID := make(map[string]*resourceStack, len(namespacedResourcesCFGs))
	ingByIngIDByNSID := make(map[string]map[string]*networking.Ingress, len(namespacedResourcesCFGs))
	for nsID, resCFG := range namespacedResourcesCFGs {
		ns := nsByNSID[nsID]
		resStack, ingByIngID := s.buildResourceStack(ns, resCFG, f)
		resStackByNSID[nsID] = resStack
		ingByIngIDByNSID[nsID] = ingByIngID
	}
	return resStackByNSID, ingByIngIDByNSID
}

func (s *multiPathBackendStack) buildResourceStack(ns *corev1.Namespace, resourcesCFG NamespacedResourcesConfig, f *framework.Framework) (*resourceStack, map[string]*networking.Ingress) {
	dpByBackendID, svcByBackendID := s.buildBackendResources(ns, resourcesCFG.BackendCFGs)
	ingByIngID := s.buildIngressResources(ns, resourcesCFG.IngCFGs, svcByBackendID, f)

	dps := make([]*appsv1.Deployment, 0, len(dpByBackendID))
	for _, dp := range dpByBackendID {
		dps = append(dps, dp)
	}
	svcs := make([]*corev1.Service, 0, len(svcByBackendID))
	for _, svc := range svcByBackendID {
		svcs = append(svcs, svc)
	}
	ings := make([]*networking.Ingress, 0, len(ingByIngID))
	for _, ing := range ingByIngID {
		ings = append(ings, ing)
	}
	return NewResourceStack(dps, svcs, ings), ingByIngID
}

func (s *multiPathBackendStack) buildIngressResources(ns *corev1.Namespace, ingCFGs map[string]MultiPathIngressConfig, svcByBackendID map[string]*corev1.Service, f *framework.Framework) map[string]*networking.Ingress {
	ingByIngID := make(map[string]*networking.Ingress, len(ingCFGs))
	for ingID, ingCFG := range ingCFGs {
		ing := s.buildIngressResource(ns, ingID, ingCFG, svcByBackendID, f)
		ingByIngID[ingID] = ing
	}
	return ingByIngID
}

func (s *multiPathBackendStack) buildIngressResource(ns *corev1.Namespace, ingID string, ingCFG MultiPathIngressConfig, svcByBackendID map[string]*corev1.Service, f *framework.Framework) *networking.Ingress {
	annotations := map[string]string{
		"kubernetes.io/ingress.class":      "alb",
		"alb.ingress.kubernetes.io/scheme": "internet-facing",
	}
	if f.Options.IPFamily == "IPv6" {
		annotations["alb.ingress.kubernetes.io/ip-address-type"] = "dualstack"
	}
	ing := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns.Name,
			Name:        ingID,
			Annotations: annotations,
		},
		Spec: networking.IngressSpec{
			Rules: []networking.IngressRule{
				{
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{},
					},
				},
			},
		},
	}
	for _, pathCFG := range ingCFG.PathCFGs {
		backendSVC := svcByBackendID[pathCFG.BackendID]
		exact := networking.PathTypeExact
		ing.Spec.Rules[0].HTTP.Paths = append(ing.Spec.Rules[0].HTTP.Paths, networking.HTTPIngressPath{
			Path:     pathCFG.Path,
			PathType: &exact,
			Backend: networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: backendSVC.Name,
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			},
		})
	}
	if ingCFG.GroupName != "" {
		ing.Annotations["alb.ingress.kubernetes.io/group.name"] = ingCFG.GroupName
		if ingCFG.GroupOrder != 0 {
			ing.Annotations["alb.ingress.kubernetes.io/group.order"] = fmt.Sprintf("%v", ingCFG.GroupOrder)
		}
	}
	return ing
}

func (s *multiPathBackendStack) buildBackendResources(ns *corev1.Namespace, backendCFGs map[string]BackendConfig) (map[string]*appsv1.Deployment, map[string]*corev1.Service) {
	dpByBackendID := make(map[string]*appsv1.Deployment, len(backendCFGs))
	svcByBackendID := make(map[string]*corev1.Service, len(backendCFGs))
	for backendID, backendCFG := range backendCFGs {
		dp, svc := s.buildBackendResource(ns, backendID, backendCFG)
		dpByBackendID[backendID] = dp
		svcByBackendID[backendID] = svc
	}
	return dpByBackendID, svcByBackendID
}

func (s *multiPathBackendStack) buildBackendResource(ns *corev1.Namespace, backendID string, backendCFG BackendConfig) (*appsv1.Deployment, *corev1.Service) {
	dp := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      backendID,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": backendID,
				},
			},
			Replicas: aws.Int32(backendCFG.Replicas),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": backendID,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "970805265562.dkr.ecr.us-west-2.amazonaws.com/colorteller:latest",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "SERVER_PORT",
									Value: fmt.Sprintf("%d", 8080),
								},
								{
									Name:  "COLOR",
									Value: backendCFG.HTTPBody,
								},
							},
						},
					},
				},
			},
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      backendID,
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/target-type": string(backendCFG.TargetType),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app.kubernetes.io/name": backendID,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	return dp, svc
}
