package logdelivery

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
)

var validNamePattern = `^[\w-]{1,60}$`

func Test_namePrefix(t *testing.T) {
	implicit := namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Namespace: "awesome-namespace", Name: "awesome-ingress"})
	assert.Regexp(t, `^k8s-awesomen-awesomei-[0-9a-f]{20}-$`, implicit)

	explicit := namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Name: "my.ingress-group.with.a.long.name"})
	assert.Regexp(t, `^k8s-myingressgroupwit-[0-9a-f]{20}-$`, explicit)

	assert.Equal(t, implicit, namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Namespace: "awesome-namespace", Name: "awesome-ingress"}), "prefix is stable")
	assert.NotEqual(t, implicit, namePrefix("other-cluster", "ingress.k8s.aws/resource", core.StackID{Namespace: "awesome-namespace", Name: "awesome-ingress"}), "prefix depends on the cluster")
	assert.NotEqual(t, implicit, namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Namespace: "awesome-namespace", Name: "awesome-ingress-2"}), "prefix depends on the stack")

	// Cluster names and stack IDs that run together into the same text, and truncate to the same name, still get different prefixes.
	assert.NotEqual(t,
		namePrefix("c", "ingress.k8s.aws/resource", core.StackID{Name: "aaaaaaaaaaaaaaaaaa"}),
		namePrefix("ca", "ingress.k8s.aws/resource", core.StackID{Name: "aaaaaaaaaaaaaaaaa"}))
	assert.NotEqual(t,
		namePrefix("dev", "ingress.k8s.aws/resource", core.StackID{Namespace: "aaaaaaaaa", Name: "web"}),
		namePrefix("deva", "ingress.k8s.aws/resource", core.StackID{Namespace: "aaaaaaaa", Name: "web"}))

	// Stacks whose names truncate to the same text still get different prefixes.
	a := namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Namespace: "team-a", Name: "frontend-public"})
	b := namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Namespace: "team-a", Name: "frontend-private"})
	assert.False(t, strings.HasPrefix(a, b) || strings.HasPrefix(b, a))
}

func Test_namePrefix_controllerOwnership(t *testing.T) {
	stackID := core.StackID{Namespace: "awesome-ns", Name: "awesome-name"}
	prefixes := map[string]bool{}
	for _, tagPrefix := range []string{"ingress.k8s.aws", "service.k8s.aws", "gateway.k8s.aws.alb", "gateway.k8s.aws.nlb"} {
		provider := tracking.NewDefaultProvider(tagPrefix, "my-cluster")
		prefix := namePrefix("my-cluster", provider.ResourceIDTagKey(), stackID)
		assert.False(t, prefixes[prefix], "each controller must have its own prefix")
		prefixes[prefix] = true
		assert.Equal(t, prefix, namePrefix("my-cluster", provider.ResourceIDTagKey(), stackID))
		assert.Regexp(t, validNamePattern, deliverySourceName(prefix, logdeliverymodel.LogTypeALBHealthCheckLogs))
	}
}

func Test_deliverySourceName(t *testing.T) {
	longest := namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Namespace: "a-very-long-namespace-name", Name: "a-very-long-ingress-name"})
	tests := map[string]string{
		logdeliverymodel.LogTypeALBAccessLogs:      "alb_access",
		logdeliverymodel.LogTypeALBConnectionLogs:  "alb_connection",
		logdeliverymodel.LogTypeALBHealthCheckLogs: "alb_health_check",
		logdeliverymodel.LogTypeNLBAccessLogs:      "nlb_access",
	}
	for logType, suffix := range tests {
		t.Run(logType, func(t *testing.T) {
			name := deliverySourceName(longest, logType)
			assert.Equal(t, longest+suffix, name)
			assert.Regexp(t, validNamePattern, name)
		})
	}

	explicit := namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Name: "a-very-long-ingress-group-name"})
	assert.Regexp(t, validNamePattern, deliverySourceName(explicit, logdeliverymodel.LogTypeALBHealthCheckLogs))
}

func Test_deliveryDestinationName(t *testing.T) {
	prefix := namePrefix("my-cluster", "ingress.k8s.aws/resource", core.StackID{Namespace: "a-very-long-namespace-name", Name: "a-very-long-ingress-name"})
	logGroup := "arn:aws:logs:us-west-2:111122223333:log-group:/alb/my-app"

	json := deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeCloudWatchLogs, logGroup, "json")
	assert.Regexp(t, `^`+prefix+`cwl-[0-9a-f]{10}$`, json)
	assert.Regexp(t, validNamePattern, json)

	assert.Equal(t, json, deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeCloudWatchLogs, logGroup, "json"), "name is stable")
	assert.NotEqual(t, json, deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeCloudWatchLogs, logGroup, "plain"), "name depends on the output format")
	assert.NotEqual(t, json, deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeCloudWatchLogs, logGroup+"-2", "json"), "name depends on the destination")
	assert.NotEqual(t,
		deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeS3, "arn:aws:s3:::bjson", ""),
		deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeS3, "arn:aws:s3:::b", "json"),
		"a bucket name ending in a format name doesn't collide with a format")
	assert.Regexp(t, validNamePattern, deliveryDestinationName(prefix, logdeliverymodel.DestinationTypeFirehose, "arn:aws:firehose:us-west-2:111122223333:deliverystream/x", ""))
}
