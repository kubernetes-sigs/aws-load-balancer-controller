package targetgroupbinding

import (
	"encoding/json"
	"fmt"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/backend"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// Prefix for TargetHealth pod condition type.
	TargetHealthPodConditionTypePrefix = "target-health.elbv2.k8s.aws"
	// Legacy Prefix for TargetHealth pod condition type(used by AWS ALB Ingress Controller)
	TargetHealthPodConditionTypePrefixLegacy = "target-health.alb.ingress.k8s.aws"

	// Index Key for "ServiceReference" index.
	IndexKeyServiceRefName = "spec.serviceRef.name"
)

// BuildTargetHealthPodConditionType constructs the condition type for TargetHealth pod condition.
func BuildTargetHealthPodConditionType(tgb *elbv2api.TargetGroupBinding) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s/%s", TargetHealthPodConditionTypePrefix, tgb.Name))
}

// IndexFuncServiceRefName is IndexFunc for "ServiceReference" index.
func IndexFuncServiceRefName(obj client.Object) []string {
	tgb := obj.(*elbv2api.TargetGroupBinding)
	return []string{tgb.Spec.ServiceRef.Name}
}

// calculateTGBReconcileCheckpoint calculates the checkpoint for a tgb using the endpoints and tgb spec
func calculateTGBReconcileCheckpoint[V backend.Endpoint](endpoints []V, tgb *elbv2api.TargetGroupBinding) (string, error) {

	endpointStrings := make([]string, 0, len(endpoints))

	for _, ep := range endpoints {
		endpointStrings = append(endpointStrings, ep.GetIdentifier(true, tgbProtocolSupportsQuic(tgb)))
	}

	slices.Sort(endpointStrings)
	csv := strings.Join(endpointStrings, ",")

	specJSON, err := json.Marshal(tgb.Spec)
	if err != nil {
		return "", err
	}

	endpointSha := algorithm.ComputeSha256(csv)
	specSha := algorithm.ComputeSha256(string(specJSON))

	return fmt.Sprintf("%s/%s", endpointSha, specSha), nil
}

// GetTGBReconcileCheckpoint gets the sha256 hash saved in the annotations
func GetTGBReconcileCheckpoint(tgb *elbv2api.TargetGroupBinding) string {
	if checkPoint, ok := tgb.Annotations[annotations.AnnotationCheckPoint]; ok {
		return checkPoint
	}
	return ""
}

// SaveTGBReconcileCheckpoint updates the TGB object with a new checkpoint string.
func SaveTGBReconcileCheckpoint(tgb *elbv2api.TargetGroupBinding, checkpoint string) {
	if tgb.Annotations == nil {
		tgb.Annotations = map[string]string{}
	}

	tgb.Annotations[annotations.AnnotationCheckPoint] = checkpoint
	tgb.Annotations[annotations.AnnotationCheckPointTimestamp] = strconv.FormatInt(time.Now().Unix(), 10)
}

func buildServiceReferenceKey(tgb *elbv2api.TargetGroupBinding, svcRef elbv2api.ServiceReference) types.NamespacedName {
	return types.NamespacedName{
		Namespace: tgb.Namespace,
		Name:      svcRef.Name,
	}
}

func tgbProtocolSupportsQuic(tgb *elbv2api.TargetGroupBinding) bool {
	return tgb.Spec.TargetGroupProtocol != nil && (*tgb.Spec.TargetGroupProtocol == elbv2.ProtocolQUIC || *tgb.Spec.TargetGroupProtocol == elbv2.ProtocolTCP_QUIC)
}
