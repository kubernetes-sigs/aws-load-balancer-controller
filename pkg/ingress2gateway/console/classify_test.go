package console

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

func TestClassifyEntry(t *testing.T) {
	tests := []struct {
		name         string
		entry        DiffEntry
		wantExpected bool
		wantReason   string
	}{
		{
			name: "added migrated-from tag on LoadBalancer",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeLoadBalancer,
				Field:        "spec.tags.gateway.k8s.aws/migrated-from",
				Status:       StatusAdded,
			},
			wantExpected: true,
			wantReason:   "Added by migration tool",
		},
		{
			name: "added migrated-from tag on Listener",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeListener,
				Field:        "spec.tags.gateway.k8s.aws/migrated-from",
				Status:       StatusAdded,
			},
			wantExpected: true,
		},
		{
			name: "removed migrated-from tag — not expected (tag should only be added)",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeLoadBalancer,
				Field:        "spec.tags.gateway.k8s.aws/migrated-from",
				Status:       StatusRemoved,
			},
			wantExpected: false,
		},
		{
			name: "LoadBalancer spec.name change with controller-generated format on both sides",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeLoadBalancer,
				Field:        "spec.name",
				Ingress:      "k8s-consolet-echo-8c9b609bd1",
				Gateway:      "k8s-consolet-echogate-caaec044ea",
				Status:       StatusChanged,
			},
			wantExpected: true,
			wantReason:   "Controller-generated name; format preserved",
		},
		{
			name: "TargetGroup spec.name with generated format",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroup,
				Field:        "spec.name",
				Ingress:      "k8s-consolet-echoserv-3bc6d40162",
				Gateway:      "k8s-consolet-echorout-9a9e0d3ac4",
				Status:       StatusChanged,
			},
			wantExpected: true,
		},
		{
			name: "SecurityGroup spec.groupName with generated format",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeSecurityGroup,
				Field:        "spec.groupName",
				Ingress:      "k8s-consolet-echo-335c53f321",
				Gateway:      "k8s-consolet-echogate-30c8df6a5a",
				Status:       StatusChanged,
			},
			wantExpected: true,
		},
		{
			// Explicit ingress group form (2-section) vs standalone/gateway form (3-section).
			// The ingress controller emits `k8s-<groupName>-<hash>` for explicit groups;
			// the gateway controller always emits `k8s-<ns>-<name>-<hash>`. Both sides
			// are controller-generated so the diff is expected.
			name: "SecurityGroup spec.groupName across explicit-group and 3-section forms",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeSecurityGroup,
				Field:        "spec.groupName",
				Ingress:      "k8s-demogroup-b8cc80b37d",
				Gateway:      "k8s-consoleg-demogrog-a6e036f217",
				Status:       StatusChanged,
			},
			wantExpected: true,
		},
		{
			name: "LoadBalancer spec.name across explicit-group and 3-section forms",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeLoadBalancer,
				Field:        "spec.name",
				Ingress:      "k8s-demogroup-2fa5ab75f2",
				Gateway:      "k8s-consoleg-demogrog-5f4636b5ee",
				Status:       StatusChanged,
			},
			wantExpected: true,
		},
		{
			name: "SecurityGroup spec.name is not the field used — not classified",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeSecurityGroup,
				Field:        "spec.name",
				Ingress:      "k8s-consolet-echo-335c53f321",
				Gateway:      "k8s-consolet-echogate-30c8df6a5a",
				Status:       StatusChanged,
			},
			wantExpected: false,
		},
		{
			name: "LoadBalancer name change with custom (non-generated) value on one side",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeLoadBalancer,
				Field:        "spec.name",
				Ingress:      "my-custom-lb",
				Gateway:      "k8s-consolet-echogate-caaec044ea",
				Status:       StatusChanged,
			},
			wantExpected: false,
		},
		{
			name: "Listener spec.name change — not on the allowlist",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeListener,
				Field:        "spec.name",
				Ingress:      "k8s-consolet-echo-8c9b609bd1",
				Gateway:      "k8s-consolet-echo-8c9b609bd2",
				Status:       StatusChanged,
			},
			wantExpected: false,
		},
		{
			name: "TargetGroup healthyThresholdCount default drift",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroup,
				Field:        "spec.healthCheckConfig.healthyThresholdCount",
				Ingress:      float64(2),
				Gateway:      float64(3),
				Status:       StatusChanged,
			},
			wantExpected: true,
			wantReason:   "Controller default differs (Ingress=2, Gateway=3)",
		},
		{
			name: "TargetGroup unhealthyThresholdCount default drift",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroup,
				Field:        "spec.healthCheckConfig.unhealthyThresholdCount",
				Status:       StatusChanged,
			},
			wantExpected: true,
		},
		{
			name: "TargetGroup matcher httpCode drift",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroup,
				Field:        "spec.healthCheckConfig.matcher.httpCode",
				Ingress:      "200",
				Gateway:      "200-399",
				Status:       StatusChanged,
			},
			wantExpected: true,
			wantReason:   "Controller default differs (Ingress=200, Gateway=200-399)",
		},
		{
			name: "healthCheck default drift on non-TargetGroup — not classified",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeListener,
				Field:        "spec.healthCheckConfig.healthyThresholdCount",
				Status:       StatusChanged,
			},
			wantExpected: false,
		},
		{
			name: "added ListenerRule weight",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeListenerRule,
				Field:        "spec.actions[0].forwardConfig.targetGroups[0].weight",
				Gateway:      float64(1),
				Status:       StatusAdded,
			},
			wantExpected: true,
			wantReason:   "Gateway API always sets forward weight",
		},
		{
			name: "non-forwardConfig weight — not classified",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeListenerRule,
				Field:        "spec.weight",
				Status:       StatusAdded,
			},
			wantExpected: false,
		},
		{
			name: "same status never marked expected",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeLoadBalancer,
				Field:        "spec.tags.gateway.k8s.aws/migrated-from",
				Status:       StatusSame,
			},
			wantExpected: false,
		},
		{
			name: "TargetGroupBinding spec.template.metadata.name with generated format",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroupBinding,
				Field:        "spec.template.metadata.name",
				Ingress:      "k8s-consolet-echoserv-3bc6d40162",
				Gateway:      "k8s-consolet-echorout-9a9e0d3ac4",
				Status:       StatusChanged,
			},
			wantExpected: true,
			wantReason:   "Controller-generated name; format preserved",
		},
		{
			name: "TargetGroupBinding spec.template.metadata.name with custom value — not expected",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroupBinding,
				Field:        "spec.template.metadata.name",
				Ingress:      "my-custom-tgb",
				Gateway:      "k8s-consolet-echorout-9a9e0d3ac4",
				Status:       StatusChanged,
			},
			wantExpected: false,
		},
		{
			name: "TargetGroupBinding targetGroupARN.$ref change — expected",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroupBinding,
				Field:        "spec.template.spec.targetGroupARN.$ref",
				Ingress:      "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/console-test/echo-echoserver:80/status/targetGroupARN",
				Gateway:      "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/console-test/echo-gateway-ad59fa8d22:console-test-echo-route-4f063e00c7:HTTPRoute-console-test-echoserver:80/status/targetGroupARN",
				Status:       StatusChanged,
			},
			wantExpected: true,
			wantReason:   "Points at the correlated TargetGroup; see that row for real field diffs",
		},
		{
			name: "TargetGroupBinding targetType change — user-visible, not classified",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroupBinding,
				Field:        "spec.template.spec.targetType",
				Ingress:      "ip",
				Gateway:      "instance",
				Status:       StatusChanged,
			},
			wantExpected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := classifyEntry(tt.entry, nil)
			assert.Equal(t, tt.wantExpected, c.Expected)
			if tt.wantReason != "" {
				assert.Equal(t, tt.wantReason, c.Reason)
			}
		})
	}
}

func TestClassifyEntry_UserSpecifiedHealthCheck(t *testing.T) {
	tests := []struct {
		name          string
		entry         DiffEntry
		userSpecified UserSpecifiedFields
		wantExpected  bool
	}{
		{
			name: "healthyThresholdCount NOT expected when user explicitly set it",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroup,
				Field:        "spec.healthCheckConfig.healthyThresholdCount",
				Ingress:      float64(2),
				Gateway:      float64(3),
				Status:       StatusChanged,
			},
			userSpecified: UserSpecifiedFields{
				"spec.healthCheckConfig.healthyThresholdCount": true,
			},
			wantExpected: false,
		},
		{
			name: "unhealthyThresholdCount NOT expected when user explicitly set it",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroup,
				Field:        "spec.healthCheckConfig.unhealthyThresholdCount",
				Ingress:      float64(2),
				Gateway:      float64(3),
				Status:       StatusChanged,
			},
			userSpecified: UserSpecifiedFields{
				"spec.healthCheckConfig.unhealthyThresholdCount": true,
			},
			wantExpected: false,
		},
		{
			name: "matcher.httpCode NOT expected when user explicitly set success-codes",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroup,
				Field:        "spec.healthCheckConfig.matcher.httpCode",
				Ingress:      "200",
				Gateway:      "200-399",
				Status:       StatusChanged,
			},
			userSpecified: UserSpecifiedFields{
				"spec.healthCheckConfig.matcher.httpCode": true,
			},
			wantExpected: false,
		},
		{
			name: "healthyThresholdCount still expected when a different field is user-specified",
			entry: DiffEntry{
				ResourceType: utils.StackResTypeTargetGroup,
				Field:        "spec.healthCheckConfig.healthyThresholdCount",
				Ingress:      float64(2),
				Gateway:      float64(3),
				Status:       StatusChanged,
			},
			userSpecified: UserSpecifiedFields{
				"spec.healthCheckConfig.matcher.httpCode": true,
			},
			wantExpected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := classifyEntry(tt.entry, tt.userSpecified)
			assert.Equal(t, tt.wantExpected, c.Expected)
		})
	}
}

func TestBuildUserSpecifiedFields(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantFields  UserSpecifiedFields
	}{
		{
			name:        "no annotations",
			annotations: map[string]string{},
			wantFields:  UserSpecifiedFields{},
		},
		{
			name: "healthy-threshold-count set",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/healthy-threshold-count": "5",
			},
			wantFields: UserSpecifiedFields{
				"spec.healthCheckConfig.healthyThresholdCount": true,
			},
		},
		{
			name: "all three health check annotations set",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/healthy-threshold-count":   "5",
				"alb.ingress.kubernetes.io/unhealthy-threshold-count": "3",
				"alb.ingress.kubernetes.io/success-codes":             "200",
			},
			wantFields: UserSpecifiedFields{
				"spec.healthCheckConfig.healthyThresholdCount":   true,
				"spec.healthCheckConfig.unhealthyThresholdCount": true,
				"spec.healthCheckConfig.matcher.httpCode":        true,
			},
		},
		{
			name: "unrelated annotations ignored",
			annotations: map[string]string{
				"alb.ingress.kubernetes.io/scheme":     "internet-facing",
				"alb.ingress.kubernetes.io/group.name": "my-group",
			},
			wantFields: UserSpecifiedFields{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildUserSpecifiedFields(tt.annotations)
			assert.Equal(t, tt.wantFields, got)
		})
	}
}
