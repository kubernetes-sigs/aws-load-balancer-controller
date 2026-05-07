package console

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestParseNamespacedName(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantNS  string
		wantN   string
		wantErr string
	}{
		{
			name:   "valid",
			ref:    "my-ns/my-ingress",
			wantNS: "my-ns",
			wantN:  "my-ingress",
		},
		{
			name:    "missing slash",
			ref:     "no-slash",
			wantErr: "invalid namespaced name",
		},
		{
			name:    "empty namespace",
			ref:     "/name",
			wantErr: "invalid namespaced name",
		},
		{
			name:    "empty name",
			ref:     "ns/",
			wantErr: "invalid namespaced name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, name, err := parseNamespacedName(tt.ref)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNS, ns)
			assert.Equal(t, tt.wantN, name)
		})
	}
}

func TestReadMigratedFromTag(t *testing.T) {
	tests := []struct {
		name string
		plan string
		want string
	}{
		{
			name: "standalone ingress tag",
			plan: `{"id":"ns/gw","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"tags":{"gateway.k8s.aws/migrated-from":"ingress/my-ns/my-ingress"}}}}}}`,
			want: "ingress/my-ns/my-ingress",
		},
		{
			name: "group ingress tag",
			plan: `{"id":"ns/gw","resources":{"AWS::ElasticLoadBalancingV2::LoadBalancer":{"LoadBalancer":{"spec":{"tags":{"gateway.k8s.aws/migrated-from":"ingress-group/my-group"}}}}}}`,
			want: "ingress-group/my-group",
		},
		{
			name: "no LB resource",
			plan: `{"id":"ns/gw","resources":{}}`,
			want: "",
		},
		{
			name: "invalid JSON",
			plan: "not json",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, readMigratedFromTag(tt.plan))
		})
	}
}

// TestResolvePlanHolder exercises the loop-based plan-holder discovery. The
// single-holder happy path and the multi-holder error path are the two
// behaviors we care most about — the latter indicates stale annotations that
// the group controller should have cleaned up.
func TestResolvePlanHolder(t *testing.T) {
	const groupName = "my-group"
	groupAnnos := func(plan string) map[string]string {
		m := map[string]string{
			"alb.ingress.kubernetes.io/group.name": groupName,
		}
		if plan != "" {
			m["alb.ingress.kubernetes.io/dry-run-plan"] = plan
		}
		return m
	}

	tests := []struct {
		name       string
		tag        string
		ingresses  []*networking.Ingress
		wantHolder string
		wantErr    string
	}{
		{
			name:       "standalone tag returns the embedded ref directly",
			tag:        "ingress/other-ns/some-ing",
			wantHolder: "other-ns/some-ing",
		},
		{
			name:    "unrecognized prefix errors",
			tag:     "weird/something",
			wantErr: "unrecognized migrated-from tag",
		},
		{
			name:    "empty group name errors",
			tag:     "ingress-group/",
			wantErr: "empty ingress-group name",
		},
		{
			name: "group with one holder",
			tag:  "ingress-group/" + groupName,
			ingresses: []*networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "primary", Namespace: "demo", Annotations: groupAnnos(`{"id":"demo/primary"}`)}},
				{ObjectMeta: metav1.ObjectMeta{Name: "secondary", Namespace: "demo", Annotations: groupAnnos("")}},
			},
			wantHolder: "demo/primary",
		},
		{
			name: "group with zero holders errors",
			tag:  "ingress-group/" + groupName,
			ingresses: []*networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "demo", Annotations: groupAnnos("")}},
			},
			wantErr: "no ingress in group",
		},
		{
			name: "group with multiple holders errors and lists members",
			tag:  "ingress-group/" + groupName,
			ingresses: []*networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "demo", Annotations: groupAnnos(`{"id":"demo/a"}`)}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "demo", Annotations: groupAnnos(`{"id":"demo/b"}`)}},
			},
			wantErr: "multiple ingresses in group",
		},
		{
			name: "cross-namespace group: picks holder regardless of namespace",
			tag:  "ingress-group/" + groupName,
			ingresses: []*networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "team-a", Annotations: groupAnnos("")}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "team-b", Annotations: groupAnnos(`{"id":"team-b/b"}`)}},
			},
			wantHolder: "team-b/b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, ing := range tt.ingresses {
				builder = builder.WithObjects(ing)
			}
			k8sClient := builder.Build()

			holder, err := resolvePlanHolder(context.Background(), k8sClient, "demo", tt.tag)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHolder, holder)
		})
	}
}
