package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestMutateUpdate_WhenServiceIsNotLoadBalancer(t *testing.T) {
	m := &serviceMutator{}
	svc := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}
	_, err := m.MutateUpdate(context.Background(), svc, svc)
	assert.NoError(t, err)

	assert.Nil(t, svc.Spec.LoadBalancerClass)
}

func TestMutateUpdate_WhenOldServiceHasLoadBalancerClassAndNewServiceDoesNot(t *testing.T) {
	m := &serviceMutator{}
	oldSvc := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: stringPtr("old-class")}}
	newSvc := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	_, err := m.MutateUpdate(context.Background(), newSvc, oldSvc)
	assert.NoError(t, err)
	assert.Equal(t, "old-class", *newSvc.Spec.LoadBalancerClass)
}

func TestMutateUpdate_WhenOldServiceHasLoadBalancerClassAndNewServiceHasDifferent(t *testing.T) {
	m := &serviceMutator{}
	oldSvc := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: stringPtr("old-class")}}
	newSvc := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: stringPtr("new-class")}}
	_, err := m.MutateUpdate(context.Background(), newSvc, oldSvc)
	assert.NoError(t, err)
	assert.Equal(t, "new-class", *newSvc.Spec.LoadBalancerClass)
}

func TestMutateUpdate_WhenOldServiceDoesNotHaveLoadBalancerClass(t *testing.T) {
	m := &serviceMutator{}
	oldSvc := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	newSvc := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	_, err := m.MutateUpdate(context.Background(), newSvc, oldSvc)
	assert.NoError(t, err)

	assert.Nil(t, newSvc.Spec.LoadBalancerClass)
}

func stringPtr(s string) *string {
	return &s
}
