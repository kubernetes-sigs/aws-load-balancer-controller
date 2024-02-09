package service

import (
	"context"
	"crypto/tls"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
	"time"
)

const (
	ResourceTypeELBLoadBalancer = "elasticloadbalancing:loadbalancer"
)

type NLBIPTestStack struct {
	resourceStack *resourceStack
}

func (s *NLBIPTestStack) Deploy(ctx context.Context, f *framework.Framework, svc *corev1.Service, dp *appsv1.Deployment) error {
	s.resourceStack = NewResourceStack(dp, svc, "service-ip-e2e", false)
	return s.resourceStack.Deploy(ctx, f)
}

func (s *NLBIPTestStack) UpdateServiceAnnotations(ctx context.Context, f *framework.Framework, svcAnnotations map[string]string) error {
	return s.resourceStack.UpdateServiceAnnotations(ctx, f, svcAnnotations)
}

func (s *NLBIPTestStack) ScaleDeployment(ctx context.Context, f *framework.Framework, numReplicas int32) error {
	return s.resourceStack.ScaleDeployment(ctx, f, numReplicas)
}

func (s *NLBIPTestStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	return s.resourceStack.Cleanup(ctx, f)
}

func (s *NLBIPTestStack) GetLoadBalancerIngressHostName() string {
	return s.resourceStack.GetLoadBalancerIngressHostname()
}

func (s *NLBIPTestStack) SendTrafficToLB(ctx context.Context, f *framework.Framework) error {
	httpClient := http.Client{Timeout: utils.PollIntervalMedium}
	protocol := "http"
	if s.listenerTLS() {
		protocol = "https"
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}
	// Choose the first port for now, TODO: verify all listeners
	port := s.resourceStack.svc.Spec.Ports[0].Port
	noerr := false
	for i := 0; i < 10; i++ {
		resp, err := httpClient.Get(fmt.Sprintf("%s://%s:%v/from-tls-client", protocol, s.GetLoadBalancerIngressHostName(), port))
		if err != nil {
			time.Sleep(2 * utils.PollIntervalLong)
			continue
		}
		noerr = true
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Unexpected HTTP status code %v", resp.StatusCode)
		}
		if resp.StatusCode == http.StatusOK {
			break
		}
	}
	if noerr {
		return nil
	}
	return fmt.Errorf("Unsuccessful after 10 retries")
}

func (s *NLBIPTestStack) listenerTLS() bool {
	_, ok := s.resourceStack.svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-ssl-cert"]
	return ok
}

func (s *NLBIPTestStack) targetGroupTLS() bool {
	return false
}
