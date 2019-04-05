package ingress

import (
	"fmt"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type GroupID struct {
	namespace string
	name      string
}

// For Ingress Groups, the groupID is the groupName.
func NewGroupIDForClusterGroup(groupName string) GroupID {
	return GroupID{namespace: "", name: groupName}
}

// For standalone Ingresses, the groupID is it's own full-qualified name.
func NewGroupIDForStandaloneGroup(ingKey types.NamespacedName) GroupID {
	return GroupID{namespace: ingKey.Namespace, name: ingKey.Name}
}

func (groupID GroupID) IsStandaloneGroup() bool {
	return groupID.namespace != ""
}

func (groupID GroupID) String() string {
	if groupID.IsStandaloneGroup() {
		return fmt.Sprintf("%s/%s", groupID.namespace, groupID.name)
	}
	return groupID.name
}

func (groupID GroupID) EncodeToReconcileRequest() reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: groupID.namespace, Name: groupID.name}}
}

func DecodeGroupIDFromReconcileRequest(request reconcile.Request) GroupID {
	return GroupID{namespace: request.Namespace, name: request.Name}
}

type Group struct {
	ID            GroupID
	ActiveMembers []*extensions.Ingress

	// when ingress don't belong to ingressClass or been deleted, it become LeavingMembers
	LeavingMembers []*extensions.Ingress
}
