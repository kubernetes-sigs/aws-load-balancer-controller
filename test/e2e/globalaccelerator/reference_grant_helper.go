package globalaccelerator

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/shared_constants"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// CreateReferenceGrant creates a ReferenceGrant resource to allow cross-namespace references
// from a GlobalAccelerator in fromNamespace to resources of the specified group and kind in toNamespace
func CreateReferenceGrant(
	ctx context.Context,
	tf *framework.Framework,
	name string,
	fromNamespace string,
	toNamespace string,
	toGroup string,
	toKind string,
) error {
	refGrant := &gwv1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: toNamespace, // ReferenceGrant must be in the target namespace
		},
		Spec: gwv1.ReferenceGrantSpec{
			From: []gwv1.ReferenceGrantFrom{
				{
					Group:     gwbeta1.Group(shared_constants.GlobalAcceleratorResourcesGroup),
					Kind:      gwbeta1.Kind(shared_constants.GlobalAcceleratorKind),
					Namespace: gwbeta1.Namespace(fromNamespace),
				},
			},
			To: []gwv1.ReferenceGrantTo{
				{
					Group: gwbeta1.Group(toGroup),
					Kind:  gwbeta1.Kind(toKind),
				},
			},
		},
	}

	return tf.K8sClient.Create(ctx, refGrant)
}
