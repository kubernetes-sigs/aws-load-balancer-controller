package logdelivery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
)

var invalidNamePattern = regexp.MustCompile("[[:^alnum:]]")

// namePrefix is unique to a cluster, controller ownership namespace and stack.
// Delivery sources and destinations that start with it belong to the stack.
// Names are at most 60 characters and may only contain letters, digits, '_' and '-'.
// The hash is 80 bits because users choose IngressGroup names, so a short hash could be brute-forced into another stack's prefix.
func namePrefix(clusterName string, ownershipNamespace string, stackID core.StackID) string {
	uuid := hashOf(clusterName, ownershipNamespace, stackID.String())

	name := invalidNamePattern.ReplaceAllString(stackID.Name, "")
	if stackID.Namespace == "" {
		return fmt.Sprintf("k8s-%.17s-%.20s-", name, uuid)
	}
	namespace := invalidNamePattern.ReplaceAllString(stackID.Namespace, "")
	return fmt.Sprintf("k8s-%.8s-%.8s-%.20s-", namespace, name, uuid)
}

// deliverySourceName names the stack's delivery source for a log type, e.g. k8s-ns-name-0123456789abcdef0123-alb_access.
func deliverySourceName(prefix string, logType string) string {
	return prefix + strings.ToLower(strings.TrimSuffix(logType, "_LOGS"))
}

// deliveryDestinationName names the stack's delivery destination for a destination resource and output format.
// The output format is part of the name because it can't be changed while deliveries use the destination.
func deliveryDestinationName(prefix string, destinationType string, destinationARN string, outputFormat string) string {
	return fmt.Sprintf("%s%s-%.10s", prefix, strings.ToLower(destinationType), hashOf(destinationARN, outputFormat))
}

// hashOf hashes the parts with a separator, so that ("ab", "c") and ("a", "bc") don't collide.
// Ownership is decided by name, so a collision would let one stack delete another stack's resources.
func hashOf(parts ...string) string {
	uuidHash := sha256.New()
	for _, part := range parts {
		_, _ = uuidHash.Write([]byte(part))
		_, _ = uuidHash.Write([]byte{0})
	}
	return hex.EncodeToString(uuidHash.Sum(nil))
}
