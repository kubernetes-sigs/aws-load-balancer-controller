package ingress

import (
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
)

// ClassifiedIngress is Ingress with it's associated IngressClass Configuration
type ClassifiedIngress struct {
	Ing            *networking.Ingress
	IngClassConfig ClassConfiguration
}

// ClassConfiguration contains configurations for IngressClass
type ClassConfiguration struct {
	// The IngressClass for Ingress if any.
	IngClass *networking.IngressClass

	// The IngressClassParams for Ingress if any.
	IngClassParams *elbv2api.IngressClassParams
}

func (c ClassifiedIngress) GetObjectMeta() metav1.Object {
	return c.Ing
}
