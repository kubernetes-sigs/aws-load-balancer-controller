package gateway

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
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func newALBResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, lrc *elbv2gw.ListenerRuleConfiguration, httpr []*gwv1.HTTPRoute, grpcrs []*gwv1.GRPCRoute, secret *testOIDCSecret, baseName string, enablePodReadinessGate bool) *albResourceStack {

	commonStack := newCommonResourceStack(dps, svcs, gwc, gw, lbc, tgcs, []*elbv2gw.ListenerRuleConfiguration{lrc}, baseName, enablePodReadinessGate)
	return &albResourceStack{
		httprs:      httpr,
		grpcrs:      grpcrs,
		OIDCSecret:  secret,
		commonStack: commonStack,
	}
}

// resourceStack containing the deployment and service resources
type albResourceStack struct {
	commonStack *commonResourceStack
	httprs      []*gwv1.HTTPRoute
	grpcrs      []*gwv1.GRPCRoute
	OIDCSecret  *testOIDCSecret
}

type testOIDCSecret struct {
	name         string
	namespace    string
	clientId     string
	clientSecret string
}

func (s *albResourceStack) Deploy(ctx context.Context, f *framework.Framework) error {
	return s.commonStack.Deploy(ctx, f, func(ctx context.Context, f *framework.Framework, namespace string) error {
		for i := range s.httprs {
			s.httprs[i].Namespace = namespace
			if err := s.createHTTPRoute(ctx, f, s.httprs[i]); err != nil {
				return err
			}
		}

		for i := range s.grpcrs {
			s.grpcrs[i].Namespace = namespace
			if err := s.createGRPCRoute(ctx, f, s.grpcrs[i]); err != nil {
				return err
			}
		}
		if s.OIDCSecret != nil {
			s.OIDCSecret.namespace = namespace
			return s.createOIDCSecretWithRBAC(ctx, f)
		}
		return nil
	})
}

func (s *albResourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if err := s.commonStack.Cleanup(ctx, f); err != nil {
		return err
	}
	return s.deleteOIDCSecretWithRBAC(ctx, f)
}

func (s *albResourceStack) GetLoadBalancerIngressHostname() string {
	return s.commonStack.GetLoadBalancerIngressHostname()
}

func (s *albResourceStack) getListenersPortMap() map[string]string {
	return s.commonStack.getListenersPortMap()
}

func (s *albResourceStack) GetNamespace() string {
	return s.commonStack.ns.Name
}

func (s *albResourceStack) waitUntilDeploymentReady(ctx context.Context, f *framework.Framework) error {
	return waitUntilDeploymentReady(ctx, f, s.commonStack.dps)
}

func (s *albResourceStack) createHTTPRoute(ctx context.Context, f *framework.Framework, httpr *gwv1.HTTPRoute) error {
	f.Logger.Info("creating http route", "httpr", k8s.NamespacedName(httpr))
	return f.K8sClient.Create(ctx, httpr)
}

func (s *albResourceStack) createGRPCRoute(ctx context.Context, f *framework.Framework, grpcr *gwv1.GRPCRoute) error {
	f.Logger.Info("creating grpc route", "grpc", k8s.NamespacedName(grpcr))
	return f.K8sClient.Create(ctx, grpcr)
}

func (s *albResourceStack) updateGRPCRoute(ctx context.Context, f *framework.Framework, grpcr *gwv1.GRPCRoute) error {
	f.Logger.Info("updating grpc route", "grpc", k8s.NamespacedName(grpcr))
	return f.K8sClient.Update(ctx, grpcr)
}

func (s *albResourceStack) deleteHTTPRoute(ctx context.Context, f *framework.Framework, httpr *gwv1.HTTPRoute) error {
	return f.K8sClient.Delete(ctx, httpr)
}

func (s *albResourceStack) createOIDCSecretWithRBAC(ctx context.Context, f *framework.Framework) error {
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

func (s *albResourceStack) deleteOIDCSecretWithRBAC(ctx context.Context, f *framework.Framework) error {
	if s.OIDCSecret == nil {
		return nil
	}
	namespace := s.commonStack.ns
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
