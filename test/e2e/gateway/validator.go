package gateway

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Route Status Validation
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
				if pr.ParentRef.SectionName == nil || listener.listenerName == string(*pr.ParentRef.SectionName) {
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

// Gateway Status Validation
type gatewayValidationInfo struct {
	conditions []gatewayConditionValidation
	listeners  []gatewayListenerValidation
}

type gatewayConditionValidation struct {
	conditionType   gwv1.GatewayConditionType
	conditionStatus string
	conditionReason string
}

type gatewayListenerValidation struct {
	listenerName   gwv1.SectionName
	attachedRoutes int32
	conditions     []listenerConditionValidation
}

type listenerConditionValidation struct {
	conditionType   gwv1.ListenerConditionType
	conditionStatus string
	conditionReason string
}

func validateGatewayStatus(tf *framework.Framework, gw *gwv1.Gateway, validation gatewayValidationInfo) {
	retrievedGW := &gwv1.Gateway{}
	err := tf.K8sClient.Get(context.Background(), k8s.NamespacedName(gw), retrievedGW)
	Expect(err).NotTo(HaveOccurred())

	for _, condValidation := range validation.conditions {
		var found bool
		for _, cond := range retrievedGW.Status.Conditions {
			if cond.Type == string(condValidation.conditionType) {
				found = true
				Expect(string(cond.Status)).To(Equal(condValidation.conditionStatus))
				Expect(cond.Reason).To(Equal(condValidation.conditionReason))
				break
			}
		}
		Expect(found).To(BeTrue())
	}

	for _, listenerValidation := range validation.listeners {
		var found bool
		for _, listener := range retrievedGW.Status.Listeners {
			if listener.Name == listenerValidation.listenerName {
				found = true
				Expect(listener.AttachedRoutes).To(Equal(listenerValidation.attachedRoutes))
				for _, condValidation := range listenerValidation.conditions {
					var condFound bool
					for _, cond := range listener.Conditions {
						if cond.Type == string(condValidation.conditionType) {
							condFound = true
							Expect(string(cond.Status)).To(Equal(condValidation.conditionStatus))
							Expect(cond.Reason).To(Equal(condValidation.conditionReason))
							break
						}
					}
					Expect(condFound).To(BeTrue())
				}
				break
			}
		}
		Expect(found).To(BeTrue())
	}
}
