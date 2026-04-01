package alb_tests

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/e2e/gateway/test_resources"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func newALBResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, lrc *elbv2gw.ListenerRuleConfiguration, httpr []*gwv1.HTTPRoute, grpcrs []*gwv1.GRPCRoute, secret *testOIDCSecret, baseName string, namespaceLabels map[string]string) *ALBResourceStack {

	commonStack := test_resources.NewCommonResourceStack(dps, svcs, gwc, gw, lbc, tgcs, []*elbv2gw.ListenerRuleConfiguration{lrc}, baseName, namespaceLabels)
	return &ALBResourceStack{
		Httprs:      httpr,
		Grpcrs:      grpcrs,
		OIDCSecret:  secret,
		CommonStack: commonStack,
	}
}

// ALBResourceStack containing the deployment and service resources
type ALBResourceStack struct {
	CommonStack *test_resources.CommonResourceStack
	Httprs      []*gwv1.HTTPRoute
	Grpcrs      []*gwv1.GRPCRoute
	OIDCSecret  *testOIDCSecret
}

type testOIDCSecret struct {
	name         string
	namespace    string
	clientId     string
	clientSecret string
}

func (s *ALBResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.CommonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {
		for i := range s.Httprs {
			s.Httprs[i].Namespace = namespace
			if err := s.CreateHTTPRoute(ctx, f, s.Httprs[i]); err != nil {
				return err
			}
		}

		for i := range s.Grpcrs {
			s.Grpcrs[i].Namespace = namespace
			if err := s.CreateGRPCRoute(ctx, f, s.Grpcrs[i]); err != nil {
				return err
			}
		}
		if s.OIDCSecret != nil {
			s.OIDCSecret.namespace = namespace
			return s.CreateOIDCSecretWithRBAC(ctx, f)
		}
		return nil
	})
}

func (s *ALBResourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if s == nil || s.CommonStack == nil {
		return nil
	}
	if err := s.CommonStack.Cleanup(ctx, f); err != nil {
		return err
	}
	return s.DeleteOIDCSecretWithRBAC(ctx, f)
}

func (s *ALBResourceStack) GetLoadBalancerIngressHostname() string {
	return s.CommonStack.GetLoadBalancerIngressHostname()
}

func (s *ALBResourceStack) GetListenersPortMap() map[string]string {
	return s.CommonStack.GetListenersPortMap()
}

func (s *ALBResourceStack) GetNamespace() string {
	return s.CommonStack.Ns.Name
}

func (s *ALBResourceStack) WaitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
	return test_resources.WaitUntilDeploymentReady(ctx, f, s.CommonStack.Dps)
}

func (s *ALBResourceStack) CreateHTTPRoute(ctx context.Context, f *framework.Framework, httpr *gwv1.HTTPRoute) error {
	f.Logger.Info("creating http route", "httpr", k8s.NamespacedName(httpr))
	return f.K8sClient.Create(ctx, httpr)
}

func (s *ALBResourceStack) CreateGRPCRoute(ctx context.Context, f *framework.Framework, grpcr *gwv1.GRPCRoute) error {
	f.Logger.Info("creating grpc route", "grpc", k8s.NamespacedName(grpcr))
	return f.K8sClient.Create(ctx, grpcr)
}

func (s *ALBResourceStack) updateGRPCRoute(ctx context.Context, f *framework.Framework, grpcr *gwv1.GRPCRoute) error {
	f.Logger.Info("updating grpc route", "grpc", k8s.NamespacedName(grpcr))
	return f.K8sClient.Update(ctx, grpcr)
}

func (s *ALBResourceStack) deleteHTTPRoute(ctx context.Context, f *framework.Framework, httpr *gwv1.HTTPRoute) error {
	return f.K8sClient.Delete(ctx, httpr)
}

func (s *ALBResourceStack) CreateOIDCSecretWithRBAC(ctx context.Context, f *framework.Framework) error {
	if s.OIDCSecret == nil {
		return nil
	}
	f.Logger.Info("creating oidc secret", "secret", types.NamespacedName{Name: s.OIDCSecret.name, Namespace: s.OIDCSecret.namespace})
	roleName := fmt.Sprintf("oidc-secret-reader-%s", s.OIDCSecret.name)
	roleBindingName := fmt.Sprintf("oidc-secret-reader-binding-%s", s.OIDCSecret.name)

	// 1. Create Role for secret access
	role := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: s.OIDCSecret.namespace,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{s.OIDCSecret.name},
				Verbs:         []string{"get", "list", "watch"},
			},
		},
	}
	if err := f.K8sClient.Create(ctx, role); err != nil {
		return err
	}

	// 2. Create RoleBinding to aws-load-balancer-controller service account
	roleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: s.OIDCSecret.namespace,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "aws-load-balancer-controller",
				Namespace: "kube-system",
			},
		},
	}
	if err := f.K8sClient.Create(ctx, roleBinding); err != nil {
		return err
	}

	// 3. Create the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.OIDCSecret.name,
			Namespace: s.OIDCSecret.namespace,
		},
		Data: map[string][]byte{
			"clientID":     []byte(s.OIDCSecret.clientId),
			"clientSecret": []byte(s.OIDCSecret.clientSecret),
		},
	}
	return f.K8sClient.Create(ctx, secret)
}

func (s *ALBResourceStack) DeleteOIDCSecretWithRBAC(ctx context.Context, f *framework.Framework) error {
	if s.OIDCSecret == nil {
		return nil
	}
	namespace := s.CommonStack.Ns
	roleName := fmt.Sprintf("oidc-secret-reader-%s", s.OIDCSecret.name)
	roleBindingName := fmt.Sprintf("oidc-secret-reader-binding-%s", s.OIDCSecret.name)

	// 1. Delete RoleBinding
	roleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: namespace.Namespace,
		},
	}
	if err := f.K8sClient.Delete(ctx, roleBinding); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// 2. Delete Role
	role := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace.Namespace,
		},
	}
	if err := f.K8sClient.Delete(ctx, role); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// 3. Delete Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.OIDCSecret.name,
			Namespace: namespace.Namespace,
		},
	}
	return f.K8sClient.Delete(ctx, secret)
}
