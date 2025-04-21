package testutils

import (
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"reflect"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwalpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// NewListOptionEquals constructs new goMock matcher for client's ListOption
func NewListOptionEquals(expectedListOption client.ListOption) *listOptionEquals {
	return &listOptionEquals{
		expectedListOption: expectedListOption,
	}
}

type listOptionEquals struct {
	expectedListOption client.ListOption
}

var _ gomock.Matcher = &listOptionEquals{}

func (m *listOptionEquals) Matches(x interface{}) bool {
	actualListOpt, ok := x.(client.ListOption)
	if !ok {
		return false
	}
	optA := client.ListOptions{}
	optB := client.ListOptions{}
	actualListOpt.ApplyToList(&optA)
	m.expectedListOption.ApplyToList(&optB)
	return reflect.DeepEqual(optA, optB)
}

func (m *listOptionEquals) String() string {
	return "list option equals"
}

func GenerateTestClient() client.Client {
	k8sSchema := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sSchema)
	elbv2api.AddToScheme(k8sSchema)
	gwv1.AddToScheme(k8sSchema)
	gwalpha2.AddToScheme(k8sSchema)
	elbv2gw.AddToScheme(k8sSchema)
	gwbeta1.AddToScheme(k8sSchema)

	return testclient.NewClientBuilder().WithScheme(k8sSchema).Build()
}
