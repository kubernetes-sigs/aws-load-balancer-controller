package routeutils

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_comparePrecedence(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	earlier := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		ruleOne RulePrecedence
		ruleTwo RulePrecedence
		want    bool
		reason  string
	}{
		{
			name: "hostname - exact vs wildcard",
			ruleOne: RulePrecedence{
				Hostnames: []string{"api.example.com"},
			},
			ruleTwo: RulePrecedence{
				Hostnames: []string{"*.example.com"},
			},
			want:   true,
			reason: "exact hostname has higher precedence than wildcard",
		},
		{
			name: "path type - exact vs prefix",
			ruleOne: RulePrecedence{
				Hostnames: []string{"example.com"},
				PathType:  3, // exact
			},
			ruleTwo: RulePrecedence{
				Hostnames: []string{"example.com"},
				PathType:  1, // prefix
			},
			want:   true,
			reason: "exact path has higher precedence than prefix",
		},
		{
			name: "path length precedence",
			ruleOne: RulePrecedence{
				Hostnames:  []string{"example.com"},
				PathType:   1,
				PathLength: 10,
			},
			ruleTwo: RulePrecedence{
				Hostnames:  []string{"example.com"},
				PathType:   1,
				PathLength: 5,
			},
			want:   true,
			reason: "longer path has higher precedence",
		},
		{
			name: "method precedence",
			ruleOne: RulePrecedence{
				Hostnames:  []string{"example.com"},
				PathType:   1,
				PathLength: 5,
				HasMethod:  true,
			},
			ruleTwo: RulePrecedence{
				Hostnames:  []string{"example.com"},
				PathType:   1,
				PathLength: 5,
				HasMethod:  false,
			},
			want:   true,
			reason: "rule with method has higher precedence",
		},
		{
			name: "header count precedence",
			ruleOne: RulePrecedence{
				Hostnames:   []string{"example.com"},
				PathType:    1,
				PathLength:  5,
				HasMethod:   false,
				HeaderCount: 3,
			},
			ruleTwo: RulePrecedence{
				Hostnames:   []string{"example.com"},
				PathType:    1,
				PathLength:  5,
				HasMethod:   false,
				HeaderCount: 1,
			},
			want:   true,
			reason: "more headers has higher precedence",
		},
		{
			name: "creation timestamp precedence",
			ruleOne: RulePrecedence{
				Hostnames:            []string{"example.com"},
				PathType:             1,
				PathLength:           5,
				HasMethod:            false,
				HeaderCount:          1,
				QueryParamCount:      0,
				RouteCreateTimestamp: earlier,
			},
			ruleTwo: RulePrecedence{
				Hostnames:            []string{"example.com"},
				PathType:             1,
				PathLength:           5,
				HasMethod:            false,
				HeaderCount:          1,
				QueryParamCount:      0,
				RouteCreateTimestamp: now,
			},
			want:   true,
			reason: "earlier creation time has higher precedence",
		},
		{
			name: "rule index precedence",
			ruleOne: RulePrecedence{
				Hostnames:            []string{"example.com"},
				PathType:             1,
				PathLength:           5,
				HasMethod:            false,
				HeaderCount:          1,
				QueryParamCount:      0,
				RouteCreateTimestamp: now,
				RouteNamespacedName:  "default/route-a",
				RuleIndexInRoute:     0,
			},
			ruleTwo: RulePrecedence{
				Hostnames:            []string{"example.com"},
				PathType:             1,
				PathLength:           5,
				HasMethod:            false,
				HeaderCount:          1,
				QueryParamCount:      0,
				RouteCreateTimestamp: now,
				RouteNamespacedName:  "default/route-a",
				RuleIndexInRoute:     1,
			},
			want:   true,
			reason: "lower rule index has higher precedence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := comparePrecedence(tt.ruleOne, tt.ruleTwo)
			assert.Equal(t, tt.want, got, tt.reason)
		})
	}
}

// Test getHostnamePrecedenceOrder
func Test_getHostnamePrecedenceOrder(t *testing.T) {
	tests := []struct {
		name        string
		hostnameOne string
		hostnameTwo string
		want        int
		description string
	}{
		{
			name:        "non-wildcard vs wildcard",
			hostnameOne: "example.com",
			hostnameTwo: "*.example.com",
			want:        -1,
			description: "non-wildcard should have higher precedence than wildcard",
		},
		{
			name:        "wildcard vs non-wildcard",
			hostnameOne: "*.example.com",
			hostnameTwo: "example.com",
			want:        1,
			description: "wildcard should have lower precedence than non-wildcard",
		},
		{
			name:        "both non-wildcard - first longer",
			hostnameOne: "test.example.com",
			hostnameTwo: "example.com",
			want:        -1,
			description: "longer hostname should have higher precedence",
		},
		{
			name:        "both non-wildcard - second longer",
			hostnameOne: "example.com",
			hostnameTwo: "test.example.com",
			want:        1,
			description: "shorter hostname should have lower precedence",
		},
		{
			name:        "both wildcard - first longer",
			hostnameOne: "*.test.example.com",
			hostnameTwo: "*.example.com",
			want:        -1,
			description: "longer wildcard hostname should have higher precedence",
		},
		{
			name:        "both wildcard - second longer",
			hostnameOne: "*.example.com",
			hostnameTwo: "*.test.example.com",
			want:        1,
			description: "shorter wildcard hostname should have lower precedence",
		},
		{
			name:        "equal length non-wildcard",
			hostnameOne: "test1.com",
			hostnameTwo: "test2.com",
			want:        0,
			description: "equal length hostnames should have equal precedence",
		},
		{
			name:        "equal length wildcard",
			hostnameOne: "*.test1.com",
			hostnameTwo: "*.test2.com",
			want:        0,
			description: "equal length wildcard hostnames should have equal precedence",
		},
		{
			name:        "empty strings",
			hostnameOne: "",
			hostnameTwo: "",
			want:        0,
			description: "empty strings should have equal precedence",
		},
		{
			name:        "one empty string - first",
			hostnameOne: "",
			hostnameTwo: "example.com",
			want:        1,
			description: "empty string should have lower precedence",
		},
		{
			name:        "one empty string - second",
			hostnameOne: "example.com",
			hostnameTwo: "",
			want:        -1,
			description: "non-empty string should have higher precedence than empty",
		},
		{
			name:        "one hostname has more dots",
			hostnameOne: "*.example.com",
			hostnameTwo: "*.t.exa.com",
			want:        1,
			description: "hostname with more dots should have higher precedence even if it has less character",
		},
		{
			name:        "two hostnames have same number of dots, one has more characters",
			hostnameOne: "*.t.example.com",
			hostnameTwo: "*.t.exa.com",
			want:        -1,
			description: "hostname with more characters should have higher precedence order if they have same number of dots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getHostnamePrecedenceOrder(tt.hostnameOne, tt.hostnameTwo)
			if got != tt.want {
				t.Errorf("GetHostnamePrecedenceOrder() = %v, want %v\nDescription: %s\nHostname1: %q\nHostname2: %q",
					got, tt.want, tt.description, tt.hostnameOne, tt.hostnameTwo)
			}
		})
	}
}
