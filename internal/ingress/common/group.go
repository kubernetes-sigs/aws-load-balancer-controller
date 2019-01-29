package common

import "k8s.io/apimachinery/pkg/types"

// For ingress within a group, we use it's groupID as GroupID.
// For ingress without a group, we use it's namespace/name as GroupID.
type GroupID types.NamespacedName
