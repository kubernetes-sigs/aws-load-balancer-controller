package test_resources

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

// RouteValidationInfo Route Status Validation
type RouteValidationInfo struct {
	ParentGatewayName string
	ListenerInfo      []ListenerValidationInfo
}

type ListenerValidationInfo struct {
	ListenerName       string
	ParentKind         string
	ResolvedRefsStatus string
	ResolvedRefReason  string
	AcceptedStatus     string
	AcceptedReason     string
}

func ValidateRouteStatus[R any](tf *framework.Framework, routes []R, routeStatusConverter func(*framework.Framework, interface{}) (gwv1.RouteStatus, types.NamespacedName, error), validationMap map[string]RouteValidationInfo) {

	for _, createdRoute := range routes {
		rs, nsn, err := routeStatusConverter(tf, createdRoute)
		Expect(err).NotTo(HaveOccurred())
		info := validationMap[nsn.String()]
		for _, pr := range rs.Parents {
			Expect(string(pr.ParentRef.Name)).To(Equal(info.ParentGatewayName))
			var found bool
			for _, listener := range info.ListenerInfo {
				if pr.ParentRef.SectionName == nil || listener.ListenerName == string(*pr.ParentRef.SectionName) {
					found = true
					Expect(string(*pr.ParentRef.Kind)).To(Equal(listener.ParentKind))
					for _, cond := range pr.Conditions {
						if cond.Type == string(gwv1.RouteConditionResolvedRefs) {
							Expect(string(cond.Status)).To(Equal(listener.ResolvedRefsStatus))
							Expect(cond.Reason).To(Equal(listener.ResolvedRefReason))
						} else if cond.Type == string(gwv1.RouteConditionAccepted) {
							Expect(string(cond.Status)).To(Equal(listener.AcceptedStatus))
							Expect(cond.Reason).To(Equal(listener.AcceptedReason))
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

// GatewayValidationInfo Gateway Status Validation
type GatewayValidationInfo struct {
	Conditions []GatewayConditionValidation
	Listeners  []GatewayListenerValidation
}

type GatewayConditionValidation struct {
	ConditionType   gwv1.GatewayConditionType
	ConditionStatus string
	ConditionReason string
}

type GatewayListenerValidation struct {
	ListenerName   gwv1.SectionName
	AttachedRoutes int32
	Conditions     []ListenerConditionValidation
}

type ListenerConditionValidation struct {
	ConditionType   gwv1.ListenerConditionType
	ConditionStatus string
	ConditionReason string
}

func ValidateGatewayStatus(tf *framework.Framework, gw *gwv1.Gateway, validation GatewayValidationInfo) {
	retrievedGW := &gwv1.Gateway{}
	err := tf.K8sClient.Get(context.Background(), k8s.NamespacedName(gw), retrievedGW)
	Expect(err).NotTo(HaveOccurred())

	for _, condValidation := range validation.Conditions {
		var found bool
		for _, cond := range retrievedGW.Status.Conditions {
			if cond.Type == string(condValidation.ConditionType) {
				found = true
				Expect(string(cond.Status)).To(Equal(condValidation.ConditionStatus))
				Expect(cond.Reason).To(Equal(condValidation.ConditionReason))
				break
			}
		}
		Expect(found).To(BeTrue())
	}

	for _, listenerValidation := range validation.Listeners {
		var found bool
		for _, listener := range retrievedGW.Status.Listeners {
			if listener.Name == listenerValidation.ListenerName {
				found = true
				Expect(listener.AttachedRoutes).To(Equal(listenerValidation.AttachedRoutes))
				for _, condValidation := range listenerValidation.Conditions {
					var condFound bool
					for _, cond := range listener.Conditions {
						if cond.Type == string(condValidation.ConditionType) {
							condFound = true
							Expect(string(cond.Status)).To(Equal(condValidation.ConditionStatus))
							Expect(cond.Reason).To(Equal(condValidation.ConditionReason))
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
