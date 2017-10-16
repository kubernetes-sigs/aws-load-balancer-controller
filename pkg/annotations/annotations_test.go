package annotations

import "testing"

const clusterName = "testCluster"

func TestParseAnnotations(t *testing.T) {
	_, err := ParseAnnotations(nil, clusterName)
	if err == nil {
		t.Fatalf("ParseAnnotations should not accept nil for annotations")
	}
}

func TestSetScheme(t *testing.T) {
	var tests = []struct {
		scheme   string
		expected string
		pass     bool
	}{
		{"", "", false},
		{"internal", "internal", true},
		{"internal", "internet-facing", false},
		{"internet-facing", "internal", false},
		{"internet-facing", "internet-facing", true},
	}

	for _, tt := range tests {
		a := &Annotations{}

		err := a.setScheme(map[string]string{schemeKey: tt.scheme})
		if err != nil && tt.pass {
			t.Errorf("setScheme(%v): expected %v, errored: %v", tt.scheme, tt.expected, err)
		}
		if err == nil && tt.pass && tt.expected != *a.Scheme {
			t.Errorf("setScheme(%v): expected %v, actual %v", tt.scheme, tt.expected, *a.Scheme)
		}
		if err == nil && !tt.pass && tt.expected == *a.Scheme {
			t.Errorf("setScheme(%v): expected %v, actual %v", tt.scheme, tt.expected, *a.Scheme)
		}
	}
}

// Should fail to create due to healthchecktimeout being greater than HealthcheckIntervalSeconds
func TestHealthcheckSecondsValidation(t *testing.T) {
	a := &Annotations{}
	if err := a.setHealthcheckIntervalSeconds(map[string]string{healthcheckIntervalSecondsKey: "5"}); err != nil {
		t.Errorf("Unexpected error seting HealthcheckIntervalSeconds. Error: %s", err.Error())
	}

	if err := a.setHealthcheckTimeoutSeconds(map[string]string{healthcheckTimeoutSecondsKey: "10"}); err == nil {
		t.Errorf("Set healthchecktimeoutSeconds when it should have failed due to being higher than HealthcheckIntervalSeconds")
	}
}
