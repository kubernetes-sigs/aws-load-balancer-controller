package annotations

import "testing"

const clusterName = "testCluster"
const ingressName = "testIngressName"
const ingressNamespace = "test-namespace"

func TestParseAnnotations(t *testing.T) {
	_, err := ParseAnnotations(nil, clusterName, ingressName, ingressNamespace)
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

		err := a.setScheme(map[string]string{schemeKey: tt.scheme}, ingressName, ingressNamespace)
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

func TestSetIpAddressType(t *testing.T) {
	var tests = []struct {
		ipAddressType string
		expected string
		pass   bool
	}{
		{"", "ipv4", true}, // ip-address-type has a sane default
		{"/", "", false},
		{"ipv4", "ipv4",true},
		{"ipv4", "dualstack",false},
		{"dualstack", "ipv4", false},
		{"dualstack", "dualstack", true},
	}

	for _, tt := range tests {
		a := &Annotations{}

		err := a.setIpAddressType(map[string]string{ipAddressTypeKey: tt.ipAddressType})
		if err != nil && tt.pass {
			t.Errorf("setIpAddressType(%v): expected %v, actual %v", tt.ipAddressType, tt.pass, err)
		}
		if err == nil && tt.pass && tt.expected != *a.IpAddressType {
			t.Errorf("setIpAddressType(%v): expected %v, actual %v", tt.ipAddressType, tt.expected, *a.IpAddressType)
		}
		if err == nil && !tt.pass && tt.expected == *a.IpAddressType {
			t.Errorf("setIpAddressType(%v): expected %v, actual %v", tt.ipAddressType, tt.expected, *a.IpAddressType)
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

// Should fail when idle timeout is not in range 1-3600. Should succeed otherwise.
func TestConnectionIdleTimeoutValidation(t *testing.T) {
	a := &Annotations{}

	err := a.setConnectionIdleTimeout(map[string]string{connectionIdleTimeoutKey: "15"})
	if err != nil || a.ConnectionIdleTimeout == 0 {
		t.Error("Failed to set connection idle timeout when value was correct.")
	}

	err = a.setConnectionIdleTimeout(map[string]string{connectionIdleTimeoutKey: "3700"})
	if err == nil {
		t.Error("Succeeded setting connection idle timeout when value was incorrect")
	}
}
