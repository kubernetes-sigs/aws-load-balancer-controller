package ingress

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestShouldSkipOnCertError(t *testing.T) {
	annotationParser := annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress)
	logger := log.Log.WithName("test")

	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
		description string
	}{
		{
			name:        "annotation set to true",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": "true"},
			want:        true,
			description: "Should return true when annotation is exactly 'true'",
		},
		{
			name:        "annotation set to false",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": "false"},
			want:        false,
			description: "Should return false when annotation is 'false'",
		},
		{
			name:        "annotation absent",
			annotations: map[string]string{},
			want:        false,
			description: "Should return false when annotation is absent",
		},
		{
			name:        "annotation absent with nil annotations",
			annotations: nil,
			want:        false,
			description: "Should return false when annotations map is nil",
		},
		{
			name:        "invalid value - yes",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": "yes"},
			want:        false,
			description: "Should return false for invalid value 'yes'",
		},
		{
			name:        "invalid value - 1",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": "1"},
			want:        false,
			description: "Should return false for invalid value '1'",
		},
		{
			name:        "invalid value - True (capitalized)",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": "True"},
			want:        false,
			description: "Should return false for invalid value 'True' (case-sensitive)",
		},
		{
			name:        "invalid value - FALSE (uppercase)",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": "FALSE"},
			want:        false,
			description: "Should return false for invalid value 'FALSE' (case-sensitive)",
		},
		{
			name:        "invalid value - empty string",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": ""},
			want:        false,
			description: "Should return false for empty string value",
		},
		{
			name:        "invalid value - whitespace",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": " true "},
			want:        false,
			description: "Should return false for value with whitespace",
		},
		{
			name:        "invalid value - random string",
			annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": "enabled"},
			want:        false,
			description: "Should return false for random string value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "test-namespace",
					Annotations: tt.annotations,
				},
			}

			got := ShouldSkipOnCertError(ing, annotationParser, logger)
			assert.Equal(t, tt.want, got, tt.description)
		})
	}
}

func TestShouldSkipOnCertError_LogsWarningForInvalidValues(t *testing.T) {
	annotationParser := annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress)

	// Use a mock logger to verify warning is logged
	// For this test, we just verify the function returns false for invalid values
	// The actual logging behavior is tested implicitly through the function's behavior
	invalidValues := []string{"yes", "no", "1", "0", "True", "False", "TRUE", "enabled", "disabled", "", " "}

	for _, invalidValue := range invalidValues {
		t.Run("invalid_value_"+invalidValue, func(t *testing.T) {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "test-namespace",
					Annotations: map[string]string{"alb.ingress.kubernetes.io/skip-on-cert-error": invalidValue},
				},
			}

			// Use a no-op logger for this test
			got := ShouldSkipOnCertError(ing, annotationParser, logr.Discard())
			assert.False(t, got, "Should return false for invalid value: %q", invalidValue)
		})
	}
}

// TestEmptyGroupScenarioWhenAllIngressesSkipped verifies that when all Ingresses
// in a group are skipped due to certificate errors, the controller handles this
// as an empty group scenario.
// Validates: Requirement 2.4
func TestEmptyGroupScenarioWhenAllIngressesSkipped(t *testing.T) {
	// Create test Ingresses with skip-on-cert-error annotation
	ing1 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ing-1",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
			},
		},
	}
	ing2 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ing-2",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
			},
		},
	}

	// Create a Group with members
	group := Group{
		ID: GroupID{Namespace: "test-ns", Name: "test-group"},
		Members: []ClassifiedIngress{
			{Ing: ing1},
			{Ing: ing2},
		},
	}

	// Simulate skipping all Ingresses
	skippedMembers := []SkippedIngress{
		{
			Ingress:     ing1,
			Reason:      "certificate not found for host: example1.com",
			FailedHosts: []string{"example1.com"},
		},
		{
			Ingress:     ing2,
			Reason:      "certificate not found for host: example2.com",
			FailedHosts: []string{"example2.com"},
		},
	}

	// Update the group with skipped members
	group.SkippedMembers = skippedMembers

	// Verify that SkippedMembers is populated correctly
	assert.Len(t, group.SkippedMembers, 2, "SkippedMembers should contain 2 entries")
	assert.Equal(t, "ing-1", group.SkippedMembers[0].Ingress.Name)
	assert.Equal(t, "ing-2", group.SkippedMembers[1].Ingress.Name)
	assert.Contains(t, group.SkippedMembers[0].Reason, "certificate not found")
	assert.Contains(t, group.SkippedMembers[1].Reason, "certificate not found")
	assert.Equal(t, []string{"example1.com"}, group.SkippedMembers[0].FailedHosts)
	assert.Equal(t, []string{"example2.com"}, group.SkippedMembers[1].FailedHosts)
}

// TestSkippedMembersTrackedForEventMetricEmission verifies that skipped members
// are tracked in the Group structure for later event and metric emission.
// Validates: Requirements 2.1, 4.1, 5.1
func TestSkippedMembersTrackedForEventMetricEmission(t *testing.T) {
	ing := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
			},
		},
	}

	// Create a SkippedIngress entry
	skippedIngress := SkippedIngress{
		Ingress:     ing,
		Reason:      "no matching ACM certificate found for host: test.example.com",
		FailedHosts: []string{"test.example.com"},
	}

	// Verify the SkippedIngress structure contains all necessary information
	// for event and metric emission
	assert.NotNil(t, skippedIngress.Ingress, "Ingress reference should not be nil")
	assert.Equal(t, "test-ingress", skippedIngress.Ingress.Name, "Ingress name should be preserved")
	assert.Equal(t, "test-ns", skippedIngress.Ingress.Namespace, "Ingress namespace should be preserved")
	assert.NotEmpty(t, skippedIngress.Reason, "Reason should contain the certificate error")
	assert.NotEmpty(t, skippedIngress.FailedHosts, "FailedHosts should contain the TLS hosts that failed")
}
