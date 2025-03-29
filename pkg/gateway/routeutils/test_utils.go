package routeutils

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func generateTestClient() client.Client {
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)
	elbv2api.AddToScheme(k8sSchema)
	gwv1.AddToScheme(k8sSchema)
	gwalpha2.AddToScheme(k8sSchema)
	return testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
}
