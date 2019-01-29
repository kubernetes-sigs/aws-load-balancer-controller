package cert

import (
	"k8s.io/apimachinery/pkg/types"
)

// CertGroup is certArn indexed by the certName(tls secret key).
type CertGroup map[types.NamespacedName]string

// TagGenerator provides tag generation functionality for cert package.
type TagGenerator interface {
	// TagCertGroup generates tags for the group of managed ACM certificates created for a single ingress.
	TagCertGroup(ingKey types.NamespacedName) map[string]string
}
