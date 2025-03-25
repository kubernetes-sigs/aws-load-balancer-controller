package routeutils

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// namespaceSelector is an internal utility
// that is responsible for transforming a label selector into the all relevant namespaces
// that match the selector criteria.
type namespaceSelector interface {
	getNamespacesFromSelector(context context.Context, selector *metav1.LabelSelector) (sets.Set[string], error)
}

var _ namespaceSelector = &namespaceSelectorImpl{}

type namespaceSelectorImpl struct {
	k8sClient client.Client
}

// getNamespacesFromSelector queries the Kubernetes API for all namespaces that match a selector.
func (n *namespaceSelectorImpl) getNamespacesFromSelector(context context.Context, selector *metav1.LabelSelector) (sets.Set[string], error) {
	namespaceList := v1.NamespaceList{}

	convertedSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}

	err = n.k8sClient.List(context, &namespaceList, client.MatchingLabelsSelector{Selector: convertedSelector})
	if err != nil {
		return nil, err
	}

	namespaces := sets.New[string]()

	for _, ns := range namespaceList.Items {
		namespaces.Insert(ns.Name)
	}

	return namespaces, nil
}
