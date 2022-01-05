package ingress

import (
	"fmt"

	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// GroupID is the unique identifier for an IngressGroup within cluster.
type GroupID types.NamespacedName

// IsExplicit tests whether this is an explicit group.
// Explicit groups are defined by either:
//	* annotation on Ingress: `group.name`
//  * field on associated IngressClassParams: `group.name`
func (groupID GroupID) IsExplicit() bool {
	return groupID.Namespace == ""
}

// String returns the string representation of a GroupID.
func (groupID GroupID) String() string {
	if groupID.IsExplicit() {
		return groupID.Name
	}
	return fmt.Sprintf("%s/%s", groupID.Namespace, groupID.Name)
}

// NewGroupIDForExplicitGroup generates GroupID for an explicit group.
func NewGroupIDForExplicitGroup(groupName string) GroupID {
	return GroupID{
		Namespace: "",
		Name:      groupName,
	}
}

// NewGroupIDForImplicitGroup generates GroupID for an implicit group.
func NewGroupIDForImplicitGroup(ingKey types.NamespacedName) GroupID {
	return GroupID(ingKey)
}

// EncodeGroupIDToReconcileRequest encodes a GroupID into a controller-runtime reconcile request
func EncodeGroupIDToReconcileRequest(gID GroupID) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName(gID)}
}

// DecodeGroupIDFromReconcileRequest decodes a GroupID from a controller-runtime reconcile request
func DecodeGroupIDFromReconcileRequest(request ctrl.Request) GroupID {
	return GroupID(request.NamespacedName)
}

// An Ingress Group is an group of Ingresses that should be hosted by a single LoadBalancer.
// It's our customization for Kubernetes's Ingress Spec, an Ingress group represents an "LoadBalancer",
// where each member Ingress defines rules for that LoadBalancer.
// There are two types of group: explicit and implicit.
// Explicit groups are defined by either annotation(group.name) on Ingress or field(group.name) on associated IngressClassParams
// Implicit groups are for ingresses without explicit group, each ingress become a standalone group of itself.
type Group struct {
	ID GroupID

	// Members are Ingresses that is belong to this group.
	Members []ClassifiedIngress

	// InactiveMembers are Ingresses that no longer belong to this group, but still hold the finalizers.
	InactiveMembers []*networking.Ingress
}
