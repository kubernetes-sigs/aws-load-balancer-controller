package ingress

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// the controller name used in IngressClass for ALB.
	ingressClassControllerALB = "ingress.k8s.aws/alb"
	// the Kind for IngressClassParams CRD.
	ingressClassParamsKind = "IngressClassParams"
)

// ErrInvalidIngressClass is an sentinel error that represents the IngressClass configuration for Ingress is invalid.
var ErrInvalidIngressClass = errors.New("invalid ingress class")

// ClassLoader loads IngressClass configurations for Ingress.
type ClassLoader interface {
	// Load loads the ClassConfiguration for Ingress with IngressClassName.
	Load(ctx context.Context, ing *networking.Ingress) (ClassConfiguration, error)
}

// NewDefaultClassLoader constructs new defaultClassLoader instance.
func NewDefaultClassLoader(client client.Client) *defaultClassLoader {
	return &defaultClassLoader{
		client: client,
	}
}

// default implementation for ClassLoader
type defaultClassLoader struct {
	client client.Client
}

func (l *defaultClassLoader) Load(ctx context.Context, ing *networking.Ingress) (ClassConfiguration, error) {
	if ing.Spec.IngressClassName == nil {
		return ClassConfiguration{}, fmt.Errorf("%w: %v", ErrInvalidIngressClass, "spec.ingressClassName is nil")
	}

	ingClassKey := types.NamespacedName{Name: *ing.Spec.IngressClassName}
	ingClass := &networking.IngressClass{}
	if err := l.client.Get(ctx, ingClassKey, ingClass); err != nil {
		if apierrors.IsNotFound(err) {
			return ClassConfiguration{}, fmt.Errorf("%w: %v", ErrInvalidIngressClass, err.Error())
		}
		return ClassConfiguration{}, err
	}
	if ingClass.Spec.Controller != ingressClassControllerALB || ingClass.Spec.Parameters == nil {
		return ClassConfiguration{
			IngClass: ingClass,
		}, nil
	}

	if ingClass.Spec.Parameters.APIGroup == nil ||
		(*ingClass.Spec.Parameters.APIGroup) != elbv2api.GroupVersion.Group ||
		ingClass.Spec.Parameters.Kind != ingressClassParamsKind {
		return ClassConfiguration{}, fmt.Errorf("%w: IngressClass %v references unknown parameters", ErrInvalidIngressClass, ingClass.Name)
	}
	ingClassParamsKey := types.NamespacedName{Name: ingClass.Spec.Parameters.Name}
	ingClassParams := &elbv2api.IngressClassParams{}
	if err := l.client.Get(ctx, ingClassParamsKey, ingClassParams); err != nil {
		if apierrors.IsNotFound(err) {
			return ClassConfiguration{}, fmt.Errorf("%w: %v", ErrInvalidIngressClass, err.Error())
		}
		return ClassConfiguration{}, err
	}
	if err := l.validateIngressClassParamsNamespaceRestriction(ctx, ing, ingClassParams); err != nil {
		return ClassConfiguration{}, fmt.Errorf("%w: %v", ErrInvalidIngressClass, err.Error())
	}

	return ClassConfiguration{
		IngClass:       ingClass,
		IngClassParams: ingClassParams,
	}, nil
}

func (l *defaultClassLoader) validateIngressClassParamsNamespaceRestriction(ctx context.Context, ing *networking.Ingress, ingClassParams *elbv2api.IngressClassParams) error {
	// when namespaceSelector is empty, it matches every namespace
	if ingClassParams.Spec.NamespaceSelector == nil {
		return nil
	}

	ingNamespace := ing.Namespace
	// see https://github.com/kubernetes/kubernetes/issues/88282 and https://github.com/kubernetes/kubernetes/issues/76680
	if admissionReq := webhook.ContextGetAdmissionRequest(ctx); admissionReq != nil {
		ingNamespace = admissionReq.Namespace
	}
	ingNSKey := types.NamespacedName{Name: ingNamespace}
	ingNS := &corev1.Namespace{}
	if err := l.client.Get(ctx, ingNSKey, ingNS); err != nil {
		return err
	}
	selector, err := metav1.LabelSelectorAsSelector(ingClassParams.Spec.NamespaceSelector)
	if err != nil {
		return err
	}
	if !selector.Matches(labels.Set(ingNS.Labels)) {
		return errors.Errorf("namespaceSelector of IngressClassParams %v mismatch", ingClassParams.Name)
	}
	return nil
}
