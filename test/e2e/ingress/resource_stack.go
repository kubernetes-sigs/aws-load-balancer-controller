package ingress

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"sync"
)

func NewResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, ings []*networking.Ingress) *resourceStack {
	return &resourceStack{
		dps:  dps,
		svcs: svcs,
		ings: ings,
	}
}

// resourceStack can deploy and cleanup itself from a Kubernetes cluster.
type resourceStack struct {
	// configurations
	dps  []*appsv1.Deployment
	svcs []*corev1.Service
	ings []*networking.Ingress

	// runtime variables
	createdDPs      []*appsv1.Deployment
	createdDPsMutex sync.Mutex

	createdSVCs      []*corev1.Service
	createdSVCsMutex sync.Mutex

	createdINGs      []*networking.Ingress
	createdINGsMutex sync.Mutex
}

func (s *resourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	if err := s.createDeployments(ctx, f); err != nil {
		return err
	}
	if err := s.createServices(ctx, f); err != nil {
		return err
	}
	if err := s.createIngresses(ctx, f); err != nil {
		return err
	}
	if err := s.waitDeploymentsBecomesReady(ctx, f); err != nil {
		return err
	}
	if err := s.waitIngressesBecomesReady(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *resourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if err := s.cleanupIngresses(ctx, f); err != nil {
		return err
	}
	if err := s.cleanupServices(ctx, f); err != nil {
		return err
	}
	if err := s.cleanupDeployments(ctx, f); err != nil {
		return err
	}
	return nil
}

func (s *resourceStack) createDeployments(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("create all deployments")
	var createErrs []error
	var createErrsMutex sync.Mutex
	var wg sync.WaitGroup

	for _, dp := range s.dps {
		wg.Add(1)
		go func(dp *appsv1.Deployment) {
			defer wg.Done()
			f.Logger.Info("creating deployment", "dp", k8s.NamespacedName(dp))
			dp = dp.DeepCopy()
			if err := f.K8sClient.Create(ctx, dp); err != nil {
				createErrsMutex.Lock()
				createErrs = append(createErrs, err)
				createErrsMutex.Unlock()
				return
			}
			s.createdDPsMutex.Lock()
			s.createdDPs = append(s.createdDPs, dp)
			s.createdDPsMutex.Unlock()
			f.Logger.Info("created deployment", "dp", k8s.NamespacedName(dp))
		}(dp)
	}

	wg.Wait()
	if len(createErrs) != 0 {
		return utils.NewMultiError(createErrs...)
	}
	return nil
}

func (s *resourceStack) waitDeploymentsBecomesReady(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("wait all deployments becomes ready")
	var waitErrs []error
	var waitErrsMutex sync.Mutex
	var wg sync.WaitGroup

	s.createdDPsMutex.Lock()
	defer s.createdDPsMutex.Unlock()
	for i, dp := range s.createdDPs {
		wg.Add(1)
		go func(i int, dp *appsv1.Deployment) {
			defer wg.Done()
			f.Logger.Info("waiting deployment becomes ready", "dp", k8s.NamespacedName(dp))
			observedDP, err := f.DPManager.WaitUntilDeploymentReady(ctx, dp)
			if err != nil {
				waitErrsMutex.Lock()
				waitErrs = append(waitErrs, err)
				waitErrsMutex.Unlock()
				return
			}
			f.Logger.Info("deployment becomes ready", "dp", k8s.NamespacedName(dp))
			s.createdDPs[i] = observedDP
		}(i, dp)
	}

	wg.Wait()
	if len(waitErrs) != 0 {
		return utils.NewMultiError(waitErrs...)
	}
	return nil
}

func (s *resourceStack) cleanupDeployments(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("cleanup all deployments")
	var cleanupErrs []error
	var cleanupErrsMutex sync.Mutex
	var wg sync.WaitGroup

	s.createdDPsMutex.Lock()
	defer s.createdDPsMutex.Unlock()
	for _, dp := range s.createdDPs {
		wg.Add(1)
		go func(dp *appsv1.Deployment) {
			defer wg.Done()
			f.Logger.Info("deleting deployment", "dp", k8s.NamespacedName(dp))
			if err := f.K8sClient.Delete(ctx, dp); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			if err := f.DPManager.WaitUntilDeploymentDeleted(ctx, dp); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			f.Logger.Info("deleted deployment", "dp", k8s.NamespacedName(dp))
		}(dp)
	}

	wg.Wait()
	if len(cleanupErrs) != 0 {
		return utils.NewMultiError(cleanupErrs...)
	}
	return nil
}

func (s *resourceStack) createServices(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("create all services")
	var createErrs []error
	var createErrsMutex sync.Mutex
	var wg sync.WaitGroup

	for _, svc := range s.svcs {
		wg.Add(1)
		go func(svc *corev1.Service) {
			defer wg.Done()
			f.Logger.Info("creating service", "svc", k8s.NamespacedName(svc))
			svc = svc.DeepCopy()
			if err := f.K8sClient.Create(ctx, svc); err != nil {
				createErrsMutex.Lock()
				createErrs = append(createErrs, err)
				createErrsMutex.Unlock()
				return
			}
			s.createdSVCsMutex.Lock()
			s.createdSVCs = append(s.createdSVCs, svc)
			s.createdSVCsMutex.Unlock()
			f.Logger.Info("created service", "svc", k8s.NamespacedName(svc))
		}(svc)
	}

	wg.Wait()
	if len(createErrs) != 0 {
		return utils.NewMultiError(createErrs...)
	}
	return nil
}

func (s *resourceStack) cleanupServices(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("cleanup all services")
	var cleanupErrs []error
	var cleanupErrsMutex sync.Mutex
	var wg sync.WaitGroup

	s.createdSVCsMutex.Lock()
	defer s.createdSVCsMutex.Unlock()
	for _, svc := range s.createdSVCs {
		wg.Add(1)
		go func(svc *corev1.Service) {
			defer wg.Done()
			f.Logger.Info("deleting service", "svc", k8s.NamespacedName(svc))
			if err := f.K8sClient.Delete(ctx, svc); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			if err := f.SVCManager.WaitUntilServiceDeleted(ctx, svc); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			f.Logger.Info("deleted service", "svc", k8s.NamespacedName(svc))
		}(svc)
	}

	wg.Wait()
	if len(cleanupErrs) != 0 {
		return utils.NewMultiError(cleanupErrs...)
	}
	return nil
}

func (s *resourceStack) createIngresses(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("create all ingresses")
	var createErrs []error
	var createErrsMutex sync.Mutex
	var wg sync.WaitGroup

	for _, ing := range s.ings {
		wg.Add(1)
		go func(ing *networking.Ingress) {
			defer wg.Done()
			f.Logger.Info("creating ingress", "ing", k8s.NamespacedName(ing))
			ing = ing.DeepCopy()
			if err := f.K8sClient.Create(ctx, ing); err != nil {
				createErrsMutex.Lock()
				createErrs = append(createErrs, err)
				createErrsMutex.Unlock()
				return
			}
			s.createdINGsMutex.Lock()
			s.createdINGs = append(s.createdINGs, ing)
			s.createdINGsMutex.Unlock()
			f.Logger.Info("created ingress", "ing", k8s.NamespacedName(ing))
		}(ing)
	}

	wg.Wait()
	if len(createErrs) != 0 {
		return utils.NewMultiError(createErrs...)
	}
	return nil
}

func (s *resourceStack) waitIngressesBecomesReady(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("wait all ingresses becomes ready")
	var waitErrs []error
	var waitErrsMutex sync.Mutex
	var wg sync.WaitGroup

	s.createdINGsMutex.Lock()
	defer s.createdINGsMutex.Unlock()
	for i, ing := range s.createdINGs {
		wg.Add(1)
		go func(i int, ing *networking.Ingress) {
			defer wg.Done()
			f.Logger.Info("waiting ingress becomes ready", "ing", k8s.NamespacedName(ing))
			observedING, err := f.INGManager.WaitUntilIngressReady(ctx, ing)
			if err != nil {
				waitErrsMutex.Lock()
				waitErrs = append(waitErrs, err)
				waitErrsMutex.Unlock()
				return
			}
			f.Logger.Info("ingress becomes ready", "ing", k8s.NamespacedName(ing))
			s.createdINGs[i] = observedING
		}(i, ing)
	}

	wg.Wait()
	if len(waitErrs) != 0 {
		return utils.NewMultiError(waitErrs...)
	}
	return nil
}

func (s *resourceStack) cleanupIngresses(ctx context.Context, f *framework.Framework) error {
	f.Logger.Info("cleanup all ingresses")
	var cleanupErrs []error
	var cleanupErrsMutex sync.Mutex
	var wg sync.WaitGroup

	s.createdINGsMutex.Lock()
	defer s.createdINGsMutex.Unlock()
	for _, ing := range s.createdINGs {
		wg.Add(1)
		go func(ing *networking.Ingress) {
			defer wg.Done()
			f.Logger.Info("deleting ingress", "ing", k8s.NamespacedName(ing))
			if err := f.K8sClient.Delete(ctx, ing); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			if err := f.INGManager.WaitUntilIngressDeleted(ctx, ing); err != nil {
				cleanupErrsMutex.Lock()
				cleanupErrs = append(cleanupErrs, err)
				cleanupErrsMutex.Unlock()
				return
			}
			f.Logger.Info("deleted ingress", "ing", k8s.NamespacedName(ing))
		}(ing)
	}

	wg.Wait()
	if len(cleanupErrs) != 0 {
		return utils.NewMultiError(cleanupErrs...)
	}
	return nil
}
