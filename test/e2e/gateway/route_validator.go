package gateway

import (
	"fmt"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type routeValidationInfo struct {
	parentGatewayName string
	listenerInfo      []listenerValidationInfo
}

type listenerValidationInfo struct {
	listenerName       string
	parentKind         string
	resolvedRefsStatus string
	resolvedRefReason  string
	acceptedStatus     string
	acceptedReason     string
}

func validateRouteStatus[R any](tf *framework.Framework, routes []R, routeStatusConverter func(*framework.Framework, interface{}) (gwv1.RouteStatus, types.NamespacedName, error), validationMap map[string]routeValidationInfo) {

	for _, createdRoute := range routes {
		rs, nsn, err := routeStatusConverter(tf, createdRoute)
		Expect(err).NotTo(HaveOccurred())
		info := validationMap[nsn.String()]
		for _, pr := range rs.Parents {
			Expect(string(pr.ParentRef.Name)).To(Equal(info.parentGatewayName))
			var found bool
			for _, listener := range info.listenerInfo {
				if listener.listenerName == string(*pr.ParentRef.SectionName) {
					found = true
					Expect(string(*pr.ParentRef.Kind)).To(Equal(listener.parentKind))
					for _, cond := range pr.Conditions {
						if cond.Type == string(gwv1.RouteConditionResolvedRefs) {
							Expect(string(cond.Status)).To(Equal(listener.resolvedRefsStatus))
							Expect(cond.Reason).To(Equal(listener.resolvedRefReason))
						} else if cond.Type == string(gwv1.RouteConditionAccepted) {
							Expect(string(cond.Status)).To(Equal(listener.acceptedStatus))
							Expect(cond.Reason).To(Equal(listener.acceptedReason))
						} else {
							ginkgo.Fail(fmt.Sprintf("Unexpected condition type: %s", cond.Type))
						}
					}
					break
				}
			}
			Expect(found).To(BeTrue())
		}
	}

}
