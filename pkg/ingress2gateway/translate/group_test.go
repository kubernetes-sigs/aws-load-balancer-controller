package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestPartitionByGroup(t *testing.T) {
	tests := []struct {
		name              string
		ingresses         []networking.Ingress
		wantExplicitCount int // number of explicit groups
		wantImplicitCount int // number of implicit (single-member) groups
		wantGroupSizes    map[string]int
	}{
		{
			name: "two ingresses same group, one ungrouped",
			ingresses: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "team-a", Annotations: map[string]string{"alb.ingress.kubernetes.io/group.name": "shared"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-b", Annotations: map[string]string{"alb.ingress.kubernetes.io/group.name": "shared"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "solo", Namespace: "default"}},
			},
			wantExplicitCount: 1,
			wantImplicitCount: 1,
			wantGroupSizes:    map[string]int{"shared": 2},
		},
		{
			name: "all ungrouped",
			ingresses: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"}},
			},
			wantExplicitCount: 0,
			wantImplicitCount: 2,
		},
		{
			name: "two different groups",
			ingresses: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/group.name": "g1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/group.name": "g2"}}},
			},
			wantExplicitCount: 2,
			wantImplicitCount: 0,
			wantGroupSizes:    map[string]int{"g1": 1, "g2": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := partitionByGroup(tt.ingresses)
			explicit := 0
			implicit := 0
			for _, g := range groups {
				if g.isExplicit {
					explicit++
					if expected, ok := tt.wantGroupSizes[g.name]; ok {
						assert.Equal(t, expected, len(g.members), "group %s member count", g.name)
					}
				} else {
					implicit++
					assert.Len(t, g.members, 1)
				}
			}
			assert.Equal(t, tt.wantExplicitCount, explicit, "explicit group count")
			assert.Equal(t, tt.wantImplicitCount, implicit, "implicit group count")
		})
	}
}

func TestSortMembers(t *testing.T) {
	tests := []struct {
		name      string
		members   []networking.Ingress
		wantOrder []string // expected namespace/name order after sort
	}{
		{
			name: "sort by group.order ascending",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "high", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/group.order": "10"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "low", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/group.order": "1"}}},
			},
			wantOrder: []string{"ns/low", "ns/high"},
		},
		{
			name: "same group.order tie-breaks by namespace/name",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "z-ingress", Namespace: "ns"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "a-ingress", Namespace: "ns"}},
			},
			wantOrder: []string{"ns/a-ingress", "ns/z-ingress"},
		},
		{
			name: "same group.order tie-breaks by namespace first",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "team-b"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "team-a"}},
			},
			wantOrder: []string{"team-a/app", "team-b/app"},
		},
		{
			name: "mixed: group.order wins over lexical name",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "aaa", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/group.order": "100"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "zzz", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/group.order": "1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "mmm", Namespace: "ns"}},
			},
			wantOrder: []string{"ns/mmm", "ns/zzz", "ns/aaa"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortMembers(tt.members)
			var got []string
			for _, m := range tt.members {
				got = append(got, m.Namespace+"/"+m.Name)
			}
			assert.Equal(t, tt.wantOrder, got)
		})
	}
}

func TestPartitionByGroupDeterministicNamespace(t *testing.T) {
	tests := []struct {
		name          string
		ingresses     []networking.Ingress
		wantNamespace string // expected Gateway namespace (from sorted members[0])
	}{
		{
			name: "cross-namespace group picks lowest order member namespace",
			ingresses: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-b", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/group.name":  "shared",
					"alb.ingress.kubernetes.io/group.order": "10",
				}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "team-a", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/group.name":  "shared",
					"alb.ingress.kubernetes.io/group.order": "1",
				}}},
			},
			wantNamespace: "team-a",
		},
		{
			name: "reverse input order produces same result",
			ingresses: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "team-a", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/group.name":  "shared",
					"alb.ingress.kubernetes.io/group.order": "1",
				}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-b", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/group.name":  "shared",
					"alb.ingress.kubernetes.io/group.order": "10",
				}}},
			},
			wantNamespace: "team-a",
		},
		{
			name: "same group.order falls back to lexical namespace/name",
			ingresses: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "ing", Namespace: "zulu", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/group.name": "shared",
				}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ing", Namespace: "alpha", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/group.name": "shared",
				}}},
			},
			wantNamespace: "alpha",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := partitionByGroup(tt.ingresses)
			require.Len(t, groups, 1)
			assert.Equal(t, tt.wantNamespace, groups[0].namespace)
		})
	}
}

func TestMergeGroupLBAnnotations(t *testing.T) {
	tests := []struct {
		name    string
		members []networking.Ingress
		wantErr string
		check   func(t *testing.T, merged map[string]string)
	}{
		{
			name: "single member passthrough",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/scheme":      "internal",
					"alb.ingress.kubernetes.io/target-type": "ip",
				}}},
			},
			check: func(t *testing.T, merged map[string]string) {
				assert.Equal(t, "internal", merged["alb.ingress.kubernetes.io/scheme"])
				assert.Equal(t, "ip", merged["alb.ingress.kubernetes.io/target-type"])
			},
		},
		{
			name: "same scheme no conflict",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/scheme": "internal"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/scheme": "internal"}}},
			},
			check: func(t *testing.T, merged map[string]string) {
				assert.Equal(t, "internal", merged["alb.ingress.kubernetes.io/scheme"])
			},
		},
		{
			name: "conflicting scheme errors",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/scheme": "internal"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/scheme": "internet-facing"}}},
			},
			wantErr: "conflicting annotation",
		},
		{
			name: "certificate-arn union",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/certificate-arn": "arn:cert-a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/certificate-arn": "arn:cert-b"}}},
			},
			check: func(t *testing.T, merged map[string]string) {
				assert.Equal(t, "arn:cert-a,arn:cert-b", merged["alb.ingress.kubernetes.io/certificate-arn"])
			},
		},
		{
			name: "certificate-arn dedup",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/certificate-arn": "arn:cert-a,arn:cert-b"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/certificate-arn": "arn:cert-b,arn:cert-c"}}},
			},
			check: func(t *testing.T, merged map[string]string) {
				assert.Equal(t, "arn:cert-a,arn:cert-b,arn:cert-c", merged["alb.ingress.kubernetes.io/certificate-arn"])
			},
		},
		{
			name: "tags union no conflict",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/tags": "env=prod"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/tags": "team=platform"}}},
			},
			check: func(t *testing.T, merged map[string]string) {
				v := merged["alb.ingress.kubernetes.io/tags"]
				assert.Contains(t, v, "env=prod")
				assert.Contains(t, v, "team=platform")
			},
		},
		{
			name: "tags conflict errors",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/tags": "env=prod"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/tags": "env=staging"}}},
			},
			wantErr: "conflicting tags",
		},
		{
			name: "one member sets annotation other does not",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/scheme": "internal"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"}},
			},
			check: func(t *testing.T, merged map[string]string) {
				assert.Equal(t, "internal", merged["alb.ingress.kubernetes.io/scheme"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := mergeGroupLBAnnotations(tt.members)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, merged)
			}
		})
	}
}

func TestMergeGroupListenPorts(t *testing.T) {
	tests := []struct {
		name           string
		members        []networking.Ingress
		wantAllPorts   int
		wantMemberKeys []string
	}{
		{
			name: "union different ports",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP":80}]`,
				}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/listen-ports": `[{"HTTPS":443}]`,
				}}},
			},
			wantAllPorts:   2,
			wantMemberKeys: []string{"ns/a", "ns/b"},
		},
		{
			name: "same ports dedup",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP":80}]`,
				}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP":80}]`,
				}}},
			},
			wantAllPorts: 1,
		},
		{
			name: "default port when no annotation",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}},
			},
			wantAllPorts: 1, // HTTP:80 default
		},
		{
			name: "default HTTPS when member has cert-arn",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{
					"alb.ingress.kubernetes.io/certificate-arn": "arn:cert",
				}}},
			},
			wantAllPorts: 1, // HTTPS:443 default
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allPorts, perMember, err := mergeGroupListenPorts(tt.members)
			require.NoError(t, err)
			assert.Len(t, allPorts, tt.wantAllPorts)
			if tt.wantMemberKeys != nil {
				for _, key := range tt.wantMemberKeys {
					assert.Contains(t, perMember, key)
				}
			}
		})
	}
}

func TestResolveGroupICPs(t *testing.T) {
	icpA := &elbv2api.IngressClassParams{ObjectMeta: metav1.ObjectMeta{Name: "params-a"}}
	icpByClass := map[string]*elbv2api.IngressClassParams{
		"alb": icpA,
	}

	tests := []struct {
		name     string
		members  []networking.Ingress
		wantLen  int
		wantName string // first ICP name if wantLen > 0
	}{
		{
			name: "no ICP",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}},
			},
			wantLen: 0,
		},
		{
			name: "one member with ICP",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}, Spec: networking.IngressSpec{IngressClassName: ptr.To("alb")}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"}},
			},
			wantLen:  1,
			wantName: "params-a",
		},
		{
			name: "same ICP deduped",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}, Spec: networking.IngressSpec{IngressClassName: ptr.To("alb")}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"}, Spec: networking.IngressSpec{IngressClassName: ptr.To("alb")}},
			},
			wantLen:  1,
			wantName: "params-a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			icps := resolveGroupICPs(tt.members, icpByClass)
			assert.Len(t, icps, tt.wantLen)
			if tt.wantLen > 0 {
				assert.Equal(t, tt.wantName, icps[0].Name)
			}
		})
	}
}

func TestResolveGroupSSLRedirect(t *testing.T) {
	icpWithRedirect := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{Name: "params-redirect"},
		Spec:       elbv2api.IngressClassParamsSpec{SSLRedirectPort: "443"},
	}
	icpWithDifferentRedirect := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{Name: "params-redirect-8443"},
		Spec:       elbv2api.IngressClassParamsSpec{SSLRedirectPort: "8443"},
	}
	icpNoRedirect := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{Name: "params-no-redirect"},
	}

	tests := []struct {
		name       string
		members    []networking.Ingress
		icpByClass map[string]*elbv2api.IngressClassParams
		wantNil    bool
		wantVal    int32
		wantErr    string
	}{
		{
			name: "no ssl-redirect anywhere",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"}},
			},
			icpByClass: map[string]*elbv2api.IngressClassParams{},
			wantNil:    true,
		},
		{
			name: "one member sets ssl-redirect via annotation",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/ssl-redirect": "443"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"}},
			},
			icpByClass: map[string]*elbv2api.IngressClassParams{},
			wantVal:    443,
		},
		{
			name: "conflicting annotation values",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/ssl-redirect": "443"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/ssl-redirect": "8443"}}},
			},
			icpByClass: map[string]*elbv2api.IngressClassParams{},
			wantErr:    "conflicting ssl-redirect",
		},
		{
			name: "ICP sets ssl-redirect, annotation ignored",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/ssl-redirect": "8443"}},
					Spec: networking.IngressSpec{IngressClassName: ptr.To("alb")}},
			},
			icpByClass: map[string]*elbv2api.IngressClassParams{"alb": icpWithRedirect},
			wantVal:    443, // ICP wins over annotation's 8443
		},
		{
			name: "ICP sets ssl-redirect, member B annotation differs — error",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
					Spec: networking.IngressSpec{IngressClassName: ptr.To("alb")}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/ssl-redirect": "8443"}}},
			},
			icpByClass: map[string]*elbv2api.IngressClassParams{"alb": icpWithRedirect},
			wantErr:    "conflicting ssl-redirect",
		},
		{
			name: "two ICPs with different ssl-redirect — error",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
					Spec: networking.IngressSpec{IngressClassName: ptr.To("alb")}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns"},
					Spec: networking.IngressSpec{IngressClassName: ptr.To("alb-other")}},
			},
			icpByClass: map[string]*elbv2api.IngressClassParams{
				"alb":       icpWithRedirect,
				"alb-other": icpWithDifferentRedirect,
			},
			wantErr: "conflicting ssl-redirect",
		},
		{
			name: "ICP without ssl-redirect falls through to annotation",
			members: []networking.Ingress{
				{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", Annotations: map[string]string{"alb.ingress.kubernetes.io/ssl-redirect": "443"}},
					Spec: networking.IngressSpec{IngressClassName: ptr.To("alb")}},
			},
			icpByClass: map[string]*elbv2api.IngressClassParams{"alb": icpNoRedirect},
			wantVal:    443,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveGroupSSLRedirect(tt.members, tt.icpByClass)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.wantVal, *result)
			}
		})
	}
}

func TestBuildMemberParentRefs(t *testing.T) {
	tests := []struct {
		name             string
		memberPorts      []listenPortEntry
		sslRedirectPort  *int32
		memberNS         string
		gatewayNS        string
		wantCount        int
		wantSectionNames []string
		wantNamespace    bool
	}{
		{
			name:        "single port gets explicit sectionName",
			memberPorts: []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			memberNS:    "ns", gatewayNS: "ns",
			wantCount:        1,
			wantSectionNames: []string{"http-80"},
		},
		{
			name:        "multiple member ports each get sectionName",
			memberPorts: []listenPortEntry{{Protocol: "HTTP", Port: 80}, {Protocol: "HTTPS", Port: 443}},
			memberNS:    "ns", gatewayNS: "ns",
			wantCount:        2,
			wantSectionNames: []string{"http-80", "https-443"},
		},
		{
			name:            "ssl-redirect scopes to HTTPS only",
			memberPorts:     []listenPortEntry{{Protocol: "HTTP", Port: 80}, {Protocol: "HTTPS", Port: 443}},
			sslRedirectPort: ptr.To(int32(443)),
			memberNS:        "ns", gatewayNS: "ns",
			wantCount:        1,
			wantSectionNames: []string{"https-443"},
		},
		{
			name:        "cross-namespace adds namespace",
			memberPorts: []listenPortEntry{{Protocol: "HTTP", Port: 80}},
			memberNS:    "team-b", gatewayNS: "team-a",
			wantCount:        1,
			wantSectionNames: []string{"http-80"},
			wantNamespace:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := buildMemberParentRefs("gw", tt.gatewayNS, tt.memberNS, tt.memberPorts, tt.sslRedirectPort)
			assert.Len(t, refs, tt.wantCount)
			for i, wantSN := range tt.wantSectionNames {
				require.NotNil(t, refs[i].SectionName, "parentRef[%d] should have sectionName", i)
				assert.Equal(t, gwv1.SectionName(wantSN), *refs[i].SectionName)
			}
			if tt.wantNamespace {
				require.NotNil(t, refs[0].Namespace)
				assert.Equal(t, gwv1.Namespace(tt.gatewayNS), *refs[0].Namespace)
			} else {
				assert.Nil(t, refs[0].Namespace)
			}
		})
	}
}
