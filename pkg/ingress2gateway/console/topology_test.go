package console

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"sigs.k8s.io/aws-load-balancer-controller/pkg/ingress2gateway/utils"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		name         string
		val          any
		expectedType string
		want         string
	}{
		{
			name:         "valid TargetGroup ref",
			val:          "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/console-test/echo-echoserver:80/status/targetGroupARN",
			expectedType: utils.StackResTypeTargetGroup,
			want:         "console-test/echo-echoserver:80",
		},
		{
			name:         "valid Listener ref",
			val:          "#/resources/AWS::ElasticLoadBalancingV2::Listener/80/status/listenerARN",
			expectedType: utils.StackResTypeListener,
			want:         "80",
		},
		{
			name:         "type mismatch",
			val:          "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns/tg/status/targetGroupARN",
			expectedType: utils.StackResTypeListener,
			want:         "",
		},
		{
			name:         "non-string value",
			val:          123,
			expectedType: utils.StackResTypeTargetGroup,
			want:         "",
		},
		{
			name:         "missing #/resources/ prefix",
			val:          "AWS::ElasticLoadBalancingV2::TargetGroup/ns/tg/status/targetGroupARN",
			expectedType: utils.StackResTypeTargetGroup,
			want:         "",
		},
		{
			name:         "no /status/ separator",
			val:          "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/ns/tg",
			expectedType: utils.StackResTypeTargetGroup,
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRef(tt.val, tt.expectedType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildTopology(t *testing.T) {
	tree := ResourceTree{
		utils.StackResTypeLoadBalancer: {
			"LoadBalancer": {
				"spec.name":   "k8s-test-echo-abcdef0123",
				"spec.scheme": "internet-facing",
				// securityGroups is an array of $ref objects (as produced by real stacks).
				"spec.securityGroups": []any{
					map[string]any{"$ref": "#/resources/AWS::EC2::SecurityGroup/ManagedLBSecurityGroup/status/groupID"},
				},
			},
		},
		utils.StackResTypeSecurityGroup: {
			"ManagedLBSecurityGroup": {
				"spec.groupName": "k8s-test-echo-abcdef0123",
			},
		},
		utils.StackResTypeListener: {
			"80": {
				"spec.port":     float64(80),
				"spec.protocol": "HTTP",
			},
		},
		utils.StackResTypeListenerRule: {
			"console-test/echo-rule": {
				"spec.listenerARN.$ref": "#/resources/AWS::ElasticLoadBalancingV2::Listener/80/status/listenerARN",
				// actions is an array value containing nested $ref objects (real stack format).
				"spec.actions": []any{
					map[string]any{
						"type": "forward",
						"forwardConfig": map[string]any{
							"targetGroups": []any{
								map[string]any{
									"targetGroupARN": map[string]any{
										"$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/console-test/echo-svc:80/status/targetGroupARN",
									},
									"weight": float64(1),
								},
							},
						},
					},
				},
			},
		},
		utils.StackResTypeTargetGroup: {
			"console-test/echo-svc:80": {
				"spec.name":       "k8s-test-echosvc-1234567890",
				"spec.targetType": "ip",
			},
		},
		utils.StackResTypeTargetGroupBinding: {
			"console-test/echo-svc:80": {
				"spec.template.spec.targetGroupARN.$ref": "#/resources/AWS::ElasticLoadBalancingV2::TargetGroup/console-test/echo-svc:80/status/targetGroupARN",
				"spec.template.spec.serviceRef.name":     "echo-svc",
				"spec.template.spec.serviceRef.port":     float64(80),
			},
		},
	}

	diff := DiffResult{
		Entries: []DiffEntry{
			{ResourceType: utils.StackResTypeLoadBalancer, CorrelationID: "LoadBalancer", GatewayResourceID: "LoadBalancer", Field: "spec.name", Status: StatusChanged},
			{ResourceType: utils.StackResTypeListener, CorrelationID: "80", GatewayResourceID: "80", Field: "spec.port", Status: StatusSame},
			{ResourceType: utils.StackResTypeTargetGroup, CorrelationID: "echo-svc:80", GatewayResourceID: "console-test/echo-svc:80", Field: "spec.targetType", Status: StatusSame},
		},
		Summary: DiffSummary{Same: 2, Changed: 1},
	}

	result := BuildTopology(tree, diff)

	// Verify nodes — should have 6 (including SecurityGroup).
	assert.Len(t, result.Nodes, 6)
	typeCount := map[string]int{}
	for _, n := range result.Nodes {
		typeCount[n.ResourceType]++
	}
	assert.Equal(t, 1, typeCount[utils.StackResTypeLoadBalancer])
	assert.Equal(t, 1, typeCount[utils.StackResTypeSecurityGroup])
	assert.Equal(t, 1, typeCount[utils.StackResTypeListener])
	assert.Equal(t, 1, typeCount[utils.StackResTypeListenerRule])
	assert.Equal(t, 1, typeCount[utils.StackResTypeTargetGroup])
	assert.Equal(t, 1, typeCount[utils.StackResTypeTargetGroupBinding])

	// Verify edges.
	edgeSet := map[string]bool{}
	for _, e := range result.Edges {
		edgeSet[e.From+"->"+e.To] = true
	}

	// LB → Listener.
	assert.True(t, edgeSet[utils.StackResTypeLoadBalancer+"|LoadBalancer->"+utils.StackResTypeListener+"|80"])
	// LB → SecurityGroup.
	assert.True(t, edgeSet[utils.StackResTypeLoadBalancer+"|LoadBalancer->"+utils.StackResTypeSecurityGroup+"|ManagedLBSecurityGroup"])
	// Listener → Rule.
	assert.True(t, edgeSet[utils.StackResTypeListener+"|80->"+utils.StackResTypeListenerRule+"|console-test/echo-rule"])
	// Rule → TargetGroup (found via nested $ref in array).
	assert.True(t, edgeSet[utils.StackResTypeListenerRule+"|console-test/echo-rule->"+utils.StackResTypeTargetGroup+"|console-test/echo-svc:80"])
	// TargetGroup → TGB.
	assert.True(t, edgeSet[utils.StackResTypeTargetGroup+"|console-test/echo-svc:80->"+utils.StackResTypeTargetGroupBinding+"|console-test/echo-svc:80"])

	// Verify status propagation.
	for _, n := range result.Nodes {
		if n.ResourceType == utils.StackResTypeLoadBalancer {
			assert.Equal(t, "changed", n.Status)
		}
		if n.ResourceType == utils.StackResTypeListener {
			assert.Equal(t, "same", n.Status)
		}
	}
}

func TestBuildTopology_EmptyTree(t *testing.T) {
	result := BuildTopology(ResourceTree{}, DiffResult{})
	assert.Empty(t, result.Nodes)
	assert.Empty(t, result.Edges)
}

func TestShortLabel(t *testing.T) {
	tree := ResourceTree{
		utils.StackResTypeLoadBalancer: {
			"LB": {"spec.name": "k8s-test-echo-abcdef0123"},
		},
		utils.StackResTypeListener: {
			"80": {"spec.port": float64(80)},
		},
		utils.StackResTypeTargetGroupBinding: {
			"tgb": {
				"spec.template.spec.serviceRef.name": "my-svc",
				"spec.template.spec.serviceRef.port": float64(8080),
			},
		},
	}

	assert.Equal(t, "k8s-test-echo-abcdef0123", shortLabel(utils.StackResTypeLoadBalancer, "LB", tree))
	assert.Equal(t, ":80", shortLabel(utils.StackResTypeListener, "80", tree))
	assert.Equal(t, "my-svc:8080", shortLabel(utils.StackResTypeTargetGroupBinding, "tgb", tree))
}
