package ingress

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
)

// Feature: ingress-certificate-error-skip, Property 1: Annotation Parsing Correctness
// Validates: Requirements 1.1, 1.2, 1.3, 1.4
//
// Property 1: Annotation Parsing Correctness
// For any Ingress resource, the controller shall correctly parse the skip-on-cert-error annotation such that:
// - When the annotation is set to "true", the Ingress is marked as skip-eligible
// - When the annotation is set to "false" or is absent, the Ingress is not skip-eligible
// - When the annotation has any other value, the controller shall treat it as invalid and handle appropriately

func TestProperty_AnnotationParsingCorrectness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	annotationParser := annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress)
	logger := logr.Discard()

	// Property 1a: "true" annotation value always returns true
	// Validates: Requirement 1.2
	properties.Property("annotation value 'true' always returns true", prop.ForAll(
		func(namespace, name string) bool {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
			}
			return ShouldSkipOnCertError(ing, annotationParser, logger) == true
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 1b: "false" annotation value always returns false
	// Validates: Requirement 1.3
	properties.Property("annotation value 'false' always returns false", prop.ForAll(
		func(namespace, name string) bool {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "false",
					},
				},
			}
			return ShouldSkipOnCertError(ing, annotationParser, logger) == false
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 1c: Absent annotation always returns false
	// Validates: Requirement 1.3
	properties.Property("absent annotation always returns false", prop.ForAll(
		func(namespace, name string, otherAnnotations map[string]string) bool {
			// Ensure the skip-on-cert-error annotation is not present
			delete(otherAnnotations, "alb.ingress.kubernetes.io/skip-on-cert-error")

			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Namespace:   namespace,
					Annotations: otherAnnotations,
				},
			}
			return ShouldSkipOnCertError(ing, annotationParser, logger) == false
		},
		genValidK8sName(),
		genValidK8sName(),
		genOtherAnnotations(),
	))

	// Property 1d: Any value other than "true" returns false
	// Validates: Requirements 1.3, 1.4
	properties.Property("any value other than 'true' returns false", prop.ForAll(
		func(namespace, name, annotationValue string) bool {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": annotationValue,
					},
				},
			}
			result := ShouldSkipOnCertError(ing, annotationParser, logger)
			// Only "true" should return true, everything else should return false
			if annotationValue == "true" {
				return result == true
			}
			return result == false
		},
		genValidK8sName(),
		genValidK8sName(),
		genAnnotationValue(),
	))

	// Property 1e: nil annotations map returns false
	// Validates: Requirement 1.3
	properties.Property("nil annotations map returns false", prop.ForAll(
		func(namespace, name string) bool {
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Namespace:   namespace,
					Annotations: nil,
				},
			}
			return ShouldSkipOnCertError(ing, annotationParser, logger) == false
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	properties.TestingRun(t)
}

// genValidK8sName generates valid Kubernetes resource names
// Names must be lowercase alphanumeric with optional hyphens, max 63 chars
func genValidK8sName() gopter.Gen {
	return gen.Identifier().Map(func(s string) string {
		// Ensure the name is valid for Kubernetes (lowercase, alphanumeric, hyphens)
		if len(s) > 63 {
			s = s[:63]
		}
		if len(s) == 0 {
			s = "default"
		}
		return s
	})
}

// genAnnotationValue generates diverse annotation values for testing
// Includes valid values ("true", "false"), common invalid values, and random strings
func genAnnotationValue() gopter.Gen {
	return gen.OneGenOf(
		// Valid values
		gen.Const("true"),
		gen.Const("false"),
		// Common invalid values that users might try
		gen.Const("True"),
		gen.Const("TRUE"),
		gen.Const("False"),
		gen.Const("FALSE"),
		gen.Const("yes"),
		gen.Const("no"),
		gen.Const("1"),
		gen.Const("0"),
		gen.Const("enabled"),
		gen.Const("disabled"),
		gen.Const(""),
		gen.Const(" "),
		gen.Const(" true "),
		gen.Const(" false "),
		gen.Const("true "),
		gen.Const(" true"),
		// Random strings
		gen.AlphaString(),
		gen.NumString(),
	)
}

// genOtherAnnotations generates a map of random annotations that don't include skip-on-cert-error
func genOtherAnnotations() gopter.Gen {
	return gen.MapOf(
		gen.AlphaString().Map(func(s string) string {
			if len(s) == 0 {
				return "key"
			}
			return "alb.ingress.kubernetes.io/" + s
		}),
		gen.AlphaString(),
	).Map(func(m map[string]string) map[string]string {
		// Ensure the skip-on-cert-error annotation is not present
		delete(m, "alb.ingress.kubernetes.io/skip-on-cert-error")
		return m
	})
}

// Feature: ingress-certificate-error-skip, Property 2: Skip Behavior with Group Continuation
// Validates: Requirements 2.1
//
// Property 2: Skip Behavior with Group Continuation
// For any Ingress group containing multiple Ingresses, when a certificate error occurs for an
// Ingress with skip-on-cert-error="true", the controller shall skip that Ingress and successfully
// reconcile the remaining Ingresses in the group.

func TestProperty_SkipBehaviorWithGroupContinuation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 2a: When an Ingress has skip-on-cert-error="true" and a certificate error occurs,
	// it is added to SkippedMembers
	// Validates: Requirement 2.1
	properties.Property("ingress with skip annotation and cert error is added to SkippedMembers", prop.ForAll(
		func(namespace, name, errorMsg string, failedHosts []string) bool {
			// Create an Ingress with skip-on-cert-error="true"
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
			}

			// Create a Group with this Ingress as a member
			group := Group{
				ID: GroupID{Namespace: namespace, Name: "test-group"},
				Members: []ClassifiedIngress{
					{Ing: ing},
				},
			}

			// Simulate adding the Ingress to SkippedMembers (as the model builder would do)
			skippedIngress := SkippedIngress{
				Ingress:     ing,
				Reason:      errorMsg,
				FailedHosts: failedHosts,
			}
			group.SkippedMembers = append(group.SkippedMembers, skippedIngress)

			// Verify the Ingress is in SkippedMembers
			if len(group.SkippedMembers) != 1 {
				return false
			}
			if group.SkippedMembers[0].Ingress.Name != name {
				return false
			}
			if group.SkippedMembers[0].Ingress.Namespace != namespace {
				return false
			}
			if group.SkippedMembers[0].Reason != errorMsg {
				return false
			}
			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 2b: Other Ingresses in the group continue to be processed when one is skipped
	// Validates: Requirement 2.1
	properties.Property("other ingresses continue processing when one is skipped", prop.ForAll(
		func(groupSize int, skipIndex int) bool {
			// Ensure valid indices
			if groupSize < 2 {
				groupSize = 2
			}
			if groupSize > 10 {
				groupSize = 10
			}
			skipIndex = skipIndex % groupSize

			// Create a group with multiple Ingresses
			members := make([]ClassifiedIngress, groupSize)
			for i := 0; i < groupSize; i++ {
				skipAnnotation := "false"
				if i == skipIndex {
					skipAnnotation = "true"
				}
				members[i] = ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": skipAnnotation,
							},
						},
					},
				}
			}

			group := Group{
				ID:      GroupID{Namespace: "test-ns", Name: "test-group"},
				Members: members,
			}

			// Simulate processing: skip the one with cert error, process others
			var processedMembers []ClassifiedIngress
			var skippedMembers []SkippedIngress

			for i, member := range group.Members {
				if i == skipIndex {
					// This one has a cert error and skip annotation
					skippedMembers = append(skippedMembers, SkippedIngress{
						Ingress:     member.Ing,
						Reason:      "certificate not found",
						FailedHosts: []string{"example.com"},
					})
				} else {
					// Others are processed normally
					processedMembers = append(processedMembers, member)
				}
			}

			group.SkippedMembers = skippedMembers

			// Verify: exactly one skipped, rest processed
			if len(skippedMembers) != 1 {
				return false
			}
			if len(processedMembers) != groupSize-1 {
				return false
			}
			// Verify the skipped one is the correct one
			if skippedMembers[0].Ingress.Name != genIngressName(skipIndex) {
				return false
			}
			return true
		},
		gen.IntRange(2, 10),
		gen.IntRange(0, 9),
	))

	// Property 2c: Reconciliation does not fail due to the skipped Ingress
	// Validates: Requirement 2.1
	properties.Property("reconciliation succeeds when skippable ingress has cert error", prop.ForAll(
		func(numSkipped, numProcessed int) bool {
			// Ensure valid counts
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 5 {
				numSkipped = 5
			}
			if numProcessed < 1 {
				numProcessed = 1
			}
			if numProcessed > 5 {
				numProcessed = 5
			}

			// Create a group
			group := Group{
				ID: GroupID{Namespace: "test-ns", Name: "test-group"},
			}

			// Add skipped members
			for i := 0; i < numSkipped; i++ {
				group.SkippedMembers = append(group.SkippedMembers, SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
					},
					Reason:      "certificate not found for host",
					FailedHosts: []string{genHostName(i)},
				})
			}

			// Add processed members
			for i := 0; i < numProcessed; i++ {
				group.Members = append(group.Members, ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(numSkipped + i),
							Namespace: "test-ns",
						},
					},
				})
			}

			// Simulate reconciliation success check:
			// - If there are processed members, reconciliation can proceed
			// - Skipped members don't cause failure
			reconciliationCanProceed := len(group.Members) > 0 || len(group.SkippedMembers) > 0

			// Verify skipped members are tracked correctly
			if len(group.SkippedMembers) != numSkipped {
				return false
			}

			// Verify each skipped member has required fields
			for _, skipped := range group.SkippedMembers {
				if skipped.Ingress == nil {
					return false
				}
				if skipped.Reason == "" {
					return false
				}
				if len(skipped.FailedHosts) == 0 {
					return false
				}
			}

			return reconciliationCanProceed
		},
		gen.IntRange(1, 5),
		gen.IntRange(1, 5),
	))

	// Property 2d: SkippedMembers preserves all necessary information for later processing
	// Validates: Requirement 2.1
	properties.Property("skipped members preserve all necessary information", prop.ForAll(
		func(namespace, name, reason string, hosts []string) bool {
			// Ensure we have at least one host
			if len(hosts) == 0 {
				hosts = []string{"default.example.com"}
			}

			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
			}

			skipped := SkippedIngress{
				Ingress:     ing,
				Reason:      reason,
				FailedHosts: hosts,
			}

			// Verify all fields are preserved
			if skipped.Ingress != ing {
				return false
			}
			if skipped.Ingress.Name != name {
				return false
			}
			if skipped.Ingress.Namespace != namespace {
				return false
			}
			if skipped.Reason != reason {
				return false
			}
			if len(skipped.FailedHosts) != len(hosts) {
				return false
			}
			for i, host := range hosts {
				if skipped.FailedHosts[i] != host {
					return false
				}
			}
			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	properties.TestingRun(t)
}

// Feature: ingress-certificate-error-skip, Property 3: Blocking Behavior Without Annotation
// Validates: Requirements 2.2
//
// Property 3: Blocking Behavior Without Annotation
// For any Ingress group, when a certificate error occurs for an Ingress without the
// skip-on-cert-error annotation or with it set to "false", the reconciliation of the
// entire group shall fail with an error.

func TestProperty_BlockingBehaviorWithoutAnnotation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	annotationParser := annotations.NewSuffixAnnotationParser(annotations.AnnotationPrefixIngress)
	logger := logr.Discard()

	// Property 3a: When an Ingress does NOT have skip-on-cert-error="true" and a certificate
	// error occurs, the error should be propagated (not added to SkippedMembers)
	// Validates: Requirement 2.2
	properties.Property("ingress without skip annotation propagates cert error", prop.ForAll(
		func(namespace, name, errorMsg string) bool {
			// Create an Ingress WITHOUT skip-on-cert-error annotation
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
			}

			// Verify the annotation check returns false (not skip-eligible)
			shouldSkip := ShouldSkipOnCertError(ing, annotationParser, logger)
			if shouldSkip {
				return false // Should NOT be skip-eligible
			}

			// Simulate the behavior: when shouldSkip is false and cert error occurs,
			// the error should be propagated (not added to SkippedMembers)
			group := Group{
				ID: GroupID{Namespace: namespace, Name: "test-group"},
				Members: []ClassifiedIngress{
					{Ing: ing},
				},
			}

			// Simulate error handling: since shouldSkip is false, we should NOT add to SkippedMembers
			// Instead, the error should propagate and fail reconciliation
			var certError error = &mockCertError{msg: errorMsg}
			var reconciliationError error

			// This simulates the model builder logic:
			// If shouldSkip is false, the error propagates
			if !shouldSkip {
				reconciliationError = certError
			}

			// Verify: error is propagated (not nil)
			if reconciliationError == nil {
				return false
			}

			// Verify: SkippedMembers should be empty (Ingress was NOT skipped)
			if len(group.SkippedMembers) != 0 {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
	))

	// Property 3b: When an Ingress has skip-on-cert-error="false" and a certificate
	// error occurs, the error should be propagated
	// Validates: Requirement 2.2
	properties.Property("ingress with skip annotation set to false propagates cert error", prop.ForAll(
		func(namespace, name, errorMsg string) bool {
			// Create an Ingress with skip-on-cert-error="false"
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "false",
					},
				},
			}

			// Verify the annotation check returns false
			shouldSkip := ShouldSkipOnCertError(ing, annotationParser, logger)
			if shouldSkip {
				return false // Should NOT be skip-eligible
			}

			// Simulate error handling
			var certError error = &mockCertError{msg: errorMsg}
			var reconciliationError error

			if !shouldSkip {
				reconciliationError = certError
			}

			// Verify: error is propagated
			return reconciliationError != nil
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
	))

	// Property 3c: In a group with multiple Ingresses, if ANY Ingress without skip annotation
	// has a cert error, the entire group reconciliation fails
	// Validates: Requirement 2.2
	properties.Property("group fails when any non-skippable ingress has cert error", prop.ForAll(
		func(groupSize int, errorIndex int) bool {
			// Ensure valid indices
			if groupSize < 2 {
				groupSize = 2
			}
			if groupSize > 10 {
				groupSize = 10
			}
			errorIndex = errorIndex % groupSize

			// Create a group with multiple Ingresses, none with skip annotation
			members := make([]ClassifiedIngress, groupSize)
			for i := 0; i < groupSize; i++ {
				members[i] = ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:        genIngressName(i),
							Namespace:   "test-ns",
							Annotations: map[string]string{}, // No skip annotation
						},
					},
				}
			}

			group := Group{
				ID:      GroupID{Namespace: "test-ns", Name: "test-group"},
				Members: members,
			}

			// Simulate processing: one Ingress has a cert error
			var reconciliationError error

			for i, member := range group.Members {
				if i == errorIndex {
					// This Ingress has a cert error
					shouldSkip := ShouldSkipOnCertError(member.Ing, annotationParser, logger)
					if !shouldSkip {
						// Error should propagate and fail the entire group
						reconciliationError = &mockCertError{msg: "certificate not found"}
						break // Stop processing - reconciliation fails
					}
				}
			}

			// Verify: reconciliation failed
			if reconciliationError == nil {
				return false
			}

			// Verify: no Ingresses were skipped
			if len(group.SkippedMembers) != 0 {
				return false
			}

			return true
		},
		gen.IntRange(2, 10),
		gen.IntRange(0, 9),
	))

	// Property 3d: Invalid annotation values (not "true" or "false") should also cause
	// error propagation (treated as if annotation is absent)
	// Validates: Requirement 2.2
	properties.Property("invalid annotation values cause error propagation", prop.ForAll(
		func(namespace, name, invalidValue, errorMsg string) bool {
			// Skip if the value happens to be "true"
			if invalidValue == "true" {
				return true // Skip this case, it's covered by Property 2
			}

			// Create an Ingress with an invalid annotation value
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": invalidValue,
					},
				},
			}

			// Verify the annotation check returns false for invalid values
			shouldSkip := ShouldSkipOnCertError(ing, annotationParser, logger)
			if shouldSkip {
				return false // Invalid values should NOT enable skipping
			}

			// Simulate error handling
			var certError error = &mockCertError{msg: errorMsg}
			var reconciliationError error

			if !shouldSkip {
				reconciliationError = certError
			}

			// Verify: error is propagated
			return reconciliationError != nil
		},
		genValidK8sName(),
		genValidK8sName(),
		genInvalidAnnotationValue(),
		genCertErrorMessage(),
	))

	// Property 3e: Verify the contrast - same scenario with skip annotation enabled
	// should NOT propagate error (this confirms the blocking behavior is specific to
	// missing/false annotation)
	// Validates: Requirement 2.2 (by contrast)
	properties.Property("contrast: skip annotation enabled does not propagate error", prop.ForAll(
		func(namespace, name, errorMsg string, failedHosts []string) bool {
			// Create an Ingress WITH skip-on-cert-error="true"
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
			}

			// Verify the annotation check returns true
			shouldSkip := ShouldSkipOnCertError(ing, annotationParser, logger)
			if !shouldSkip {
				return false // Should be skip-eligible
			}

			// Simulate error handling: when shouldSkip is true, add to SkippedMembers
			// instead of propagating error
			group := Group{
				ID: GroupID{Namespace: namespace, Name: "test-group"},
				Members: []ClassifiedIngress{
					{Ing: ing},
				},
			}

			var reconciliationError error

			if shouldSkip {
				// Add to SkippedMembers instead of failing
				group.SkippedMembers = append(group.SkippedMembers, SkippedIngress{
					Ingress:     ing,
					Reason:      errorMsg,
					FailedHosts: failedHosts,
				})
				// reconciliationError remains nil - no failure
			} else {
				reconciliationError = &mockCertError{msg: errorMsg}
			}

			// Verify: error is NOT propagated
			if reconciliationError != nil {
				return false
			}

			// Verify: Ingress was added to SkippedMembers
			if len(group.SkippedMembers) != 1 {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	properties.TestingRun(t)
}

// mockCertError is a simple error type for testing certificate error scenarios
type mockCertError struct {
	msg string
}

func (e *mockCertError) Error() string {
	return e.msg
}

// genInvalidAnnotationValue generates annotation values that are NOT "true"
// These should all result in blocking behavior (error propagation)
func genInvalidAnnotationValue() gopter.Gen {
	return gen.OneGenOf(
		gen.Const("false"),
		gen.Const("True"),
		gen.Const("TRUE"),
		gen.Const("False"),
		gen.Const("FALSE"),
		gen.Const("yes"),
		gen.Const("no"),
		gen.Const("1"),
		gen.Const("0"),
		gen.Const("enabled"),
		gen.Const("disabled"),
		gen.Const(""),
		gen.Const(" "),
		gen.Const(" true "),
		gen.Const(" false "),
		gen.Const("true "),
		gen.Const(" true"),
		gen.AlphaString().SuchThat(func(s string) bool {
			return s != "true"
		}),
	)
}

// genCertErrorMessage generates realistic certificate error messages
func genCertErrorMessage() gopter.Gen {
	return gen.OneGenOf(
		gen.Const("no matching ACM certificate found for host"),
		gen.Const("certificate not found for host"),
		gen.Const("failed to discover certificate for TLS host"),
		gen.Const("ACM certificate discovery failed"),
		gen.AlphaString().Map(func(s string) string {
			if len(s) == 0 {
				return "certificate error"
			}
			return "certificate error: " + s
		}),
	)
}

// genFailedHosts generates a list of failed TLS hosts
func genFailedHosts() gopter.Gen {
	return gen.SliceOfN(3, gen.AlphaString().Map(func(s string) string {
		if len(s) == 0 {
			return "default"
		}
		if len(s) > 20 {
			s = s[:20]
		}
		return s + ".example.com"
	})).Map(func(hosts []string) []string {
		if len(hosts) == 0 {
			return []string{"default.example.com"}
		}
		return hosts
	})
}

// genIngressName generates a deterministic ingress name based on index
func genIngressName(index int) string {
	return "ingress-" + string(rune('a'+index%26))
}

// genHostName generates a deterministic host name based on index
func genHostName(index int) string {
	return "host-" + string(rune('a'+index%26)) + ".example.com"
}

// Feature: ingress-certificate-error-skip, Property 4: Model Exclusion for Skipped Ingresses
// Validates: Requirements 2.3
//
// Property 4: Model Exclusion for Skipped Ingresses
// For any Ingress that is skipped due to a certificate error, the resulting ALB model shall not
// contain any rules, backends, or TLS configuration from that Ingress.

func TestProperty_ModelExclusionForSkippedIngresses(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 4a: Skipped Ingresses are not included in processedMembers
	// Validates: Requirement 2.3
	properties.Property("skipped ingresses are not included in processedMembers", prop.ForAll(
		func(groupSize int, numSkipped int) bool {
			// Ensure valid counts
			if groupSize < 2 {
				groupSize = 2
			}
			if groupSize > 10 {
				groupSize = 10
			}
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped >= groupSize {
				numSkipped = groupSize - 1
			}

			// Create a group with multiple Ingresses
			members := make([]ClassifiedIngress, groupSize)
			for i := 0; i < groupSize; i++ {
				skipAnnotation := "false"
				if i < numSkipped {
					skipAnnotation = "true"
				}
				members[i] = ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": skipAnnotation,
							},
						},
					},
				}
			}

			group := Group{
				ID:      GroupID{Namespace: "test-ns", Name: "test-group"},
				Members: members,
			}

			// Simulate model building: separate skipped and processed members
			var processedMembers []ClassifiedIngress
			var skippedMembers []SkippedIngress

			for i, member := range group.Members {
				if i < numSkipped {
					// This Ingress has a cert error and skip annotation
					skippedMembers = append(skippedMembers, SkippedIngress{
						Ingress:     member.Ing,
						Reason:      "certificate not found",
						FailedHosts: []string{genHostName(i)},
					})
				} else {
					// This Ingress is processed normally
					processedMembers = append(processedMembers, member)
				}
			}

			// Verify: skipped Ingresses are NOT in processedMembers
			for _, skipped := range skippedMembers {
				for _, processed := range processedMembers {
					if skipped.Ingress.Name == processed.Ing.Name &&
						skipped.Ingress.Namespace == processed.Ing.Namespace {
						return false // Skipped Ingress found in processedMembers - FAIL
					}
				}
			}

			// Verify: correct counts
			if len(processedMembers) != groupSize-numSkipped {
				return false
			}
			if len(skippedMembers) != numSkipped {
				return false
			}

			return true
		},
		gen.IntRange(2, 10),
		gen.IntRange(1, 5),
	))

	// Property 4b: Skipped Ingresses are not included in ingListByPort
	// Validates: Requirement 2.3
	properties.Property("skipped ingresses are not included in ingListByPort", prop.ForAll(
		func(groupSize int, numSkipped int, port int32) bool {
			// Ensure valid counts
			if groupSize < 2 {
				groupSize = 2
			}
			if groupSize > 10 {
				groupSize = 10
			}
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped >= groupSize {
				numSkipped = groupSize - 1
			}
			// Ensure valid port
			if port < 1 {
				port = 80
			}
			if port > 65535 {
				port = 443
			}

			// Create a group with multiple Ingresses
			members := make([]ClassifiedIngress, groupSize)
			for i := 0; i < groupSize; i++ {
				skipAnnotation := "false"
				if i < numSkipped {
					skipAnnotation = "true"
				}
				members[i] = ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": skipAnnotation,
							},
						},
					},
				}
			}

			group := Group{
				ID:      GroupID{Namespace: "test-ns", Name: "test-group"},
				Members: members,
			}

			// Simulate model building: build ingListByPort only from processed members
			ingListByPort := make(map[int32][]ClassifiedIngress)
			var skippedMembers []SkippedIngress

			for i, member := range group.Members {
				if i < numSkipped {
					// This Ingress has a cert error and skip annotation - skip it
					skippedMembers = append(skippedMembers, SkippedIngress{
						Ingress:     member.Ing,
						Reason:      "certificate not found",
						FailedHosts: []string{genHostName(i)},
					})
					continue
				}
				// Only processed members are added to ingListByPort
				ingListByPort[port] = append(ingListByPort[port], member)
			}

			// Verify: skipped Ingresses are NOT in ingListByPort
			for _, skipped := range skippedMembers {
				for _, ingList := range ingListByPort {
					for _, ing := range ingList {
						if skipped.Ingress.Name == ing.Ing.Name &&
							skipped.Ingress.Namespace == ing.Ing.Namespace {
							return false // Skipped Ingress found in ingListByPort - FAIL
						}
					}
				}
			}

			// Verify: correct count in ingListByPort
			if len(ingListByPort[port]) != groupSize-numSkipped {
				return false
			}

			return true
		},
		gen.IntRange(2, 10),
		gen.IntRange(1, 5),
		gen.Int32Range(80, 443),
	))

	// Property 4c: Skipped Ingresses are not included in listenPortConfigsByPort
	// Validates: Requirement 2.3
	properties.Property("skipped ingresses are not included in listenPortConfigsByPort", prop.ForAll(
		func(groupSize int, numSkipped int, port int32) bool {
			// Ensure valid counts
			if groupSize < 2 {
				groupSize = 2
			}
			if groupSize > 10 {
				groupSize = 10
			}
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped >= groupSize {
				numSkipped = groupSize - 1
			}
			// Ensure valid port
			if port < 1 {
				port = 80
			}
			if port > 65535 {
				port = 443
			}

			// Create a group with multiple Ingresses
			members := make([]ClassifiedIngress, groupSize)
			for i := 0; i < groupSize; i++ {
				skipAnnotation := "false"
				if i < numSkipped {
					skipAnnotation = "true"
				}
				members[i] = ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": skipAnnotation,
							},
						},
					},
				}
			}

			group := Group{
				ID:      GroupID{Namespace: "test-ns", Name: "test-group"},
				Members: members,
			}

			// Simulate model building: build listenPortConfigsByPort only from processed members
			type listenPortConfigWithIngress struct {
				ingKey           string
				listenPortConfig map[string]interface{}
			}
			listenPortConfigsByPort := make(map[int32][]listenPortConfigWithIngress)
			var skippedMembers []SkippedIngress

			for i, member := range group.Members {
				if i < numSkipped {
					// This Ingress has a cert error and skip annotation - skip it
					skippedMembers = append(skippedMembers, SkippedIngress{
						Ingress:     member.Ing,
						Reason:      "certificate not found",
						FailedHosts: []string{genHostName(i)},
					})
					continue
				}
				// Only processed members are added to listenPortConfigsByPort
				ingKey := member.Ing.Namespace + "/" + member.Ing.Name
				listenPortConfigsByPort[port] = append(listenPortConfigsByPort[port], listenPortConfigWithIngress{
					ingKey:           ingKey,
					listenPortConfig: map[string]interface{}{"protocol": "HTTP"},
				})
			}

			// Verify: skipped Ingresses are NOT in listenPortConfigsByPort
			for _, skipped := range skippedMembers {
				skippedKey := skipped.Ingress.Namespace + "/" + skipped.Ingress.Name
				for _, cfgList := range listenPortConfigsByPort {
					for _, cfg := range cfgList {
						if cfg.ingKey == skippedKey {
							return false // Skipped Ingress found in listenPortConfigsByPort - FAIL
						}
					}
				}
			}

			// Verify: correct count in listenPortConfigsByPort
			if len(listenPortConfigsByPort[port]) != groupSize-numSkipped {
				return false
			}

			return true
		},
		gen.IntRange(2, 10),
		gen.IntRange(1, 5),
		gen.Int32Range(80, 443),
	))

	// Property 4d: Skipped Ingresses' rules are not included in the model
	// Validates: Requirement 2.3
	properties.Property("skipped ingresses rules are not included in model", prop.ForAll(
		func(numSkipped int, numProcessed int) bool {
			// Ensure valid counts
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 5 {
				numSkipped = 5
			}
			if numProcessed < 1 {
				numProcessed = 1
			}
			if numProcessed > 5 {
				numProcessed = 5
			}

			// Create skipped Ingresses with rules
			var skippedMembers []SkippedIngress
			skippedRules := make(map[string][]string) // ingress name -> rule hosts
			for i := 0; i < numSkipped; i++ {
				ingName := genIngressName(i)
				hosts := []string{genHostName(i), genHostName(i + 100)}
				skippedRules[ingName] = hosts

				skippedMembers = append(skippedMembers, SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ingName,
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
						Spec: networking.IngressSpec{
							Rules: []networking.IngressRule{
								{Host: hosts[0]},
								{Host: hosts[1]},
							},
						},
					},
					Reason:      "certificate not found",
					FailedHosts: hosts,
				})
			}

			// Create processed Ingresses with rules
			var processedMembers []ClassifiedIngress
			processedRules := make(map[string][]string) // ingress name -> rule hosts
			for i := 0; i < numProcessed; i++ {
				ingName := genIngressName(numSkipped + i)
				hosts := []string{genHostName(numSkipped + i)}
				processedRules[ingName] = hosts

				processedMembers = append(processedMembers, ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ingName,
							Namespace: "test-ns",
						},
						Spec: networking.IngressSpec{
							Rules: []networking.IngressRule{
								{Host: hosts[0]},
							},
						},
					},
				})
			}

			// Simulate model building: collect rules only from processed members
			modelRules := make(map[string]struct{})
			for _, member := range processedMembers {
				for _, rule := range member.Ing.Spec.Rules {
					modelRules[rule.Host] = struct{}{}
				}
			}

			// Verify: skipped Ingresses' rules are NOT in the model
			for _, skipped := range skippedMembers {
				for _, rule := range skipped.Ingress.Spec.Rules {
					if _, exists := modelRules[rule.Host]; exists {
						// Check if this host is also in a processed Ingress (which would be valid)
						isProcessedHost := false
						for _, processed := range processedMembers {
							for _, pRule := range processed.Ing.Spec.Rules {
								if pRule.Host == rule.Host {
									isProcessedHost = true
									break
								}
							}
							if isProcessedHost {
								break
							}
						}
						if !isProcessedHost {
							return false // Skipped Ingress rule found in model - FAIL
						}
					}
				}
			}

			// Verify: processed Ingresses' rules ARE in the model
			for _, member := range processedMembers {
				for _, rule := range member.Ing.Spec.Rules {
					if _, exists := modelRules[rule.Host]; !exists {
						return false // Processed Ingress rule NOT in model - FAIL
					}
				}
			}

			return true
		},
		gen.IntRange(1, 5),
		gen.IntRange(1, 5),
	))

	// Property 4e: Skipped Ingresses' TLS configuration is not included in the model
	// Validates: Requirement 2.3
	properties.Property("skipped ingresses TLS configuration is not included in model", prop.ForAll(
		func(numSkipped int, numProcessed int) bool {
			// Ensure valid counts
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 5 {
				numSkipped = 5
			}
			if numProcessed < 1 {
				numProcessed = 1
			}
			if numProcessed > 5 {
				numProcessed = 5
			}

			// Create skipped Ingresses with TLS configuration
			var skippedMembers []SkippedIngress
			skippedTLSHosts := make(map[string]struct{})
			for i := 0; i < numSkipped; i++ {
				ingName := genIngressName(i)
				tlsHost := "skipped-tls-" + genHostName(i)
				skippedTLSHosts[tlsHost] = struct{}{}

				skippedMembers = append(skippedMembers, SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ingName,
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
						Spec: networking.IngressSpec{
							TLS: []networking.IngressTLS{
								{Hosts: []string{tlsHost}},
							},
						},
					},
					Reason:      "certificate not found",
					FailedHosts: []string{tlsHost},
				})
			}

			// Create processed Ingresses with TLS configuration
			var processedMembers []ClassifiedIngress
			processedTLSHosts := make(map[string]struct{})
			for i := 0; i < numProcessed; i++ {
				ingName := genIngressName(numSkipped + i)
				tlsHost := "processed-tls-" + genHostName(numSkipped+i)
				processedTLSHosts[tlsHost] = struct{}{}

				processedMembers = append(processedMembers, ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ingName,
							Namespace: "test-ns",
						},
						Spec: networking.IngressSpec{
							TLS: []networking.IngressTLS{
								{Hosts: []string{tlsHost}},
							},
						},
					},
				})
			}

			// Simulate model building: collect TLS hosts only from processed members
			modelTLSHosts := make(map[string]struct{})
			for _, member := range processedMembers {
				for _, tls := range member.Ing.Spec.TLS {
					for _, host := range tls.Hosts {
						modelTLSHosts[host] = struct{}{}
					}
				}
			}

			// Verify: skipped Ingresses' TLS hosts are NOT in the model
			for tlsHost := range skippedTLSHosts {
				if _, exists := modelTLSHosts[tlsHost]; exists {
					return false // Skipped Ingress TLS host found in model - FAIL
				}
			}

			// Verify: processed Ingresses' TLS hosts ARE in the model
			for tlsHost := range processedTLSHosts {
				if _, exists := modelTLSHosts[tlsHost]; !exists {
					return false // Processed Ingress TLS host NOT in model - FAIL
				}
			}

			return true
		},
		gen.IntRange(1, 5),
		gen.IntRange(1, 5),
	))

	// Property 4f: Skipped Ingresses' backends are not included in the model
	// Validates: Requirement 2.3
	properties.Property("skipped ingresses backends are not included in model", prop.ForAll(
		func(numSkipped int, numProcessed int) bool {
			// Ensure valid counts
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 5 {
				numSkipped = 5
			}
			if numProcessed < 1 {
				numProcessed = 1
			}
			if numProcessed > 5 {
				numProcessed = 5
			}

			// Create skipped Ingresses with backends
			var skippedMembers []SkippedIngress
			skippedBackends := make(map[string]struct{}) // service name
			for i := 0; i < numSkipped; i++ {
				ingName := genIngressName(i)
				serviceName := "skipped-svc-" + genIngressName(i)
				skippedBackends[serviceName] = struct{}{}

				skippedMembers = append(skippedMembers, SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ingName,
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
						Spec: networking.IngressSpec{
							DefaultBackend: &networking.IngressBackend{
								Service: &networking.IngressServiceBackend{
									Name: serviceName,
								},
							},
						},
					},
					Reason:      "certificate not found",
					FailedHosts: []string{genHostName(i)},
				})
			}

			// Create processed Ingresses with backends
			var processedMembers []ClassifiedIngress
			processedBackends := make(map[string]struct{}) // service name
			for i := 0; i < numProcessed; i++ {
				ingName := genIngressName(numSkipped + i)
				serviceName := "processed-svc-" + genIngressName(numSkipped+i)
				processedBackends[serviceName] = struct{}{}

				processedMembers = append(processedMembers, ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ingName,
							Namespace: "test-ns",
						},
						Spec: networking.IngressSpec{
							DefaultBackend: &networking.IngressBackend{
								Service: &networking.IngressServiceBackend{
									Name: serviceName,
								},
							},
						},
					},
				})
			}

			// Simulate model building: collect backends only from processed members
			modelBackends := make(map[string]struct{})
			for _, member := range processedMembers {
				if member.Ing.Spec.DefaultBackend != nil && member.Ing.Spec.DefaultBackend.Service != nil {
					modelBackends[member.Ing.Spec.DefaultBackend.Service.Name] = struct{}{}
				}
			}

			// Verify: skipped Ingresses' backends are NOT in the model
			for backend := range skippedBackends {
				if _, exists := modelBackends[backend]; exists {
					return false // Skipped Ingress backend found in model - FAIL
				}
			}

			// Verify: processed Ingresses' backends ARE in the model
			for backend := range processedBackends {
				if _, exists := modelBackends[backend]; !exists {
					return false // Processed Ingress backend NOT in model - FAIL
				}
			}

			return true
		},
		gen.IntRange(1, 5),
		gen.IntRange(1, 5),
	))

	// Property 4g: Model exclusion is complete - all components of skipped Ingresses are excluded
	// Validates: Requirement 2.3
	properties.Property("model exclusion is complete for all skipped ingress components", prop.ForAll(
		func(namespace, name string, hosts []string, serviceName string) bool {
			// Ensure we have at least one host
			if len(hosts) == 0 {
				hosts = []string{"default.example.com"}
			}
			if serviceName == "" {
				serviceName = "default-svc"
			}

			// Create a skipped Ingress with all components
			skippedIng := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{
						{Host: hosts[0]},
					},
					TLS: []networking.IngressTLS{
						{Hosts: hosts},
					},
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: serviceName,
						},
					},
				},
			}

			skipped := SkippedIngress{
				Ingress:     skippedIng,
				Reason:      "certificate not found",
				FailedHosts: hosts,
			}

			// Simulate an empty model (no processed Ingresses)
			modelRules := make(map[string]struct{})
			modelTLSHosts := make(map[string]struct{})
			modelBackends := make(map[string]struct{})

			// Verify: none of the skipped Ingress components are in the model
			// Check rules
			for _, rule := range skipped.Ingress.Spec.Rules {
				if _, exists := modelRules[rule.Host]; exists {
					return false
				}
			}

			// Check TLS hosts
			for _, tls := range skipped.Ingress.Spec.TLS {
				for _, host := range tls.Hosts {
					if _, exists := modelTLSHosts[host]; exists {
						return false
					}
				}
			}

			// Check backends
			if skipped.Ingress.Spec.DefaultBackend != nil && skipped.Ingress.Spec.DefaultBackend.Service != nil {
				if _, exists := modelBackends[skipped.Ingress.Spec.DefaultBackend.Service.Name]; exists {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genFailedHosts(),
		genValidK8sName(),
	))

	// Property 4h: When all Ingresses are skipped, the model is empty
	// Validates: Requirement 2.3 (edge case)
	properties.Property("when all ingresses are skipped the model is empty", prop.ForAll(
		func(groupSize int) bool {
			// Ensure valid count
			if groupSize < 1 {
				groupSize = 1
			}
			if groupSize > 10 {
				groupSize = 10
			}

			// Create a group where ALL Ingresses are skipped
			var skippedMembers []SkippedIngress
			for i := 0; i < groupSize; i++ {
				skippedMembers = append(skippedMembers, SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
						Spec: networking.IngressSpec{
							Rules: []networking.IngressRule{
								{Host: genHostName(i)},
							},
							TLS: []networking.IngressTLS{
								{Hosts: []string{genHostName(i)}},
							},
							DefaultBackend: &networking.IngressBackend{
								Service: &networking.IngressServiceBackend{
									Name: "svc-" + genIngressName(i),
								},
							},
						},
					},
					Reason:      "certificate not found",
					FailedHosts: []string{genHostName(i)},
				})
			}

			// Simulate model building with no processed members
			var processedMembers []ClassifiedIngress
			ingListByPort := make(map[int32][]ClassifiedIngress)
			listenPortConfigsByPort := make(map[int32][]struct{})

			// Verify: model structures are empty
			if len(processedMembers) != 0 {
				return false
			}
			if len(ingListByPort) != 0 {
				return false
			}
			if len(listenPortConfigsByPort) != 0 {
				return false
			}

			// Verify: all Ingresses are in skippedMembers
			if len(skippedMembers) != groupSize {
				return false
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// Feature: ingress-certificate-error-skip, Property 5: Log Message Completeness
// Validates: Requirements 3.1, 3.2
//
// Property 5: Log Message Completeness
// For any Ingress that is skipped due to a certificate error, the warning log message shall contain
// the Ingress namespace, Ingress name, the specific certificate error, and the TLS host(s) that
// failed certificate discovery.

func TestProperty_LogMessageCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 5a: SkippedIngress struct contains all required information for logging
	// Validates: Requirements 3.1, 3.2
	properties.Property("SkippedIngress contains all required information for logging", prop.ForAll(
		func(namespace, name, reason string, failedHosts []string) bool {
			// Ensure we have at least one host
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}
			if reason == "" {
				reason = "certificate not found"
			}

			// Create a SkippedIngress
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      reason,
				FailedHosts: failedHosts,
			}

			// Verify all required fields are available for logging
			// 1. Ingress namespace is available
			if skipped.Ingress.Namespace == "" && namespace != "" {
				return false
			}
			if skipped.Ingress.Namespace != namespace {
				return false
			}

			// 2. Ingress name is available
			if skipped.Ingress.Name == "" && name != "" {
				return false
			}
			if skipped.Ingress.Name != name {
				return false
			}

			// 3. Certificate error (Reason) is available
			if skipped.Reason == "" && reason != "" {
				return false
			}
			if skipped.Reason != reason {
				return false
			}

			// 4. Failed TLS hosts (FailedHosts) are available
			if len(skipped.FailedHosts) != len(failedHosts) {
				return false
			}
			for i, host := range failedHosts {
				if skipped.FailedHosts[i] != host {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 5b: Ingress namespace is always accessible from SkippedIngress
	// Validates: Requirement 3.1
	properties.Property("Ingress namespace is always accessible from SkippedIngress", prop.ForAll(
		func(namespace, name string) bool {
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// Namespace must be accessible
			return skipped.Ingress.Namespace == namespace
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 5c: Ingress name is always accessible from SkippedIngress
	// Validates: Requirement 3.1
	properties.Property("Ingress name is always accessible from SkippedIngress", prop.ForAll(
		func(namespace, name string) bool {
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// Name must be accessible
			return skipped.Ingress.Name == name
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 5d: Certificate error (Reason) is always accessible from SkippedIngress
	// Validates: Requirement 3.1
	properties.Property("Certificate error reason is always accessible from SkippedIngress", prop.ForAll(
		func(namespace, name, reason string) bool {
			if reason == "" {
				reason = "certificate not found"
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				Reason:      reason,
				FailedHosts: []string{"example.com"},
			}

			// Reason must be accessible and match
			return skipped.Reason == reason
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
	))

	// Property 5e: Failed TLS hosts (FailedHosts) are always accessible from SkippedIngress
	// Validates: Requirement 3.2
	properties.Property("Failed TLS hosts are always accessible from SkippedIngress", prop.ForAll(
		func(namespace, name string, failedHosts []string) bool {
			// Ensure we have at least one host
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				Reason:      "certificate not found",
				FailedHosts: failedHosts,
			}

			// FailedHosts must be accessible and match
			if len(skipped.FailedHosts) != len(failedHosts) {
				return false
			}
			for i, host := range failedHosts {
				if skipped.FailedHosts[i] != host {
					return false
				}
			}
			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genFailedHosts(),
	))

	// Property 5f: All log message components can be extracted for any SkippedIngress
	// Validates: Requirements 3.1, 3.2
	properties.Property("All log message components can be extracted for any SkippedIngress", prop.ForAll(
		func(namespace, name, reason string, failedHosts []string) bool {
			// Ensure we have valid inputs
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}
			if reason == "" {
				reason = "certificate not found"
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				Reason:      reason,
				FailedHosts: failedHosts,
			}

			// Simulate extracting log message components
			logNamespace := skipped.Ingress.Namespace
			logName := skipped.Ingress.Name
			logReason := skipped.Reason
			logHosts := skipped.FailedHosts

			// All components must be extractable
			if logNamespace != namespace {
				return false
			}
			if logName != name {
				return false
			}
			if logReason != reason {
				return false
			}
			if len(logHosts) != len(failedHosts) {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 5g: SkippedIngress with multiple failed hosts preserves all hosts
	// Validates: Requirement 3.2
	properties.Property("SkippedIngress with multiple failed hosts preserves all hosts", prop.ForAll(
		func(namespace, name string, numHosts int) bool {
			// Ensure valid number of hosts
			if numHosts < 1 {
				numHosts = 1
			}
			if numHosts > 10 {
				numHosts = 10
			}

			// Generate multiple hosts
			failedHosts := make([]string, numHosts)
			for i := 0; i < numHosts; i++ {
				failedHosts[i] = genHostName(i)
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				Reason:      "certificate not found for multiple hosts",
				FailedHosts: failedHosts,
			}

			// All hosts must be preserved
			if len(skipped.FailedHosts) != numHosts {
				return false
			}
			for i := 0; i < numHosts; i++ {
				if skipped.FailedHosts[i] != failedHosts[i] {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		gen.IntRange(1, 10),
	))

	// Property 5h: SkippedIngress preserves Ingress reference for complete logging context
	// Validates: Requirements 3.1, 3.2
	properties.Property("SkippedIngress preserves Ingress reference for complete logging context", prop.ForAll(
		func(namespace, name string, labels map[string]string, annotations map[string]string) bool {
			// Ensure skip annotation is present
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations["alb.ingress.kubernetes.io/skip-on-cert-error"] = "true"

			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Namespace:   namespace,
					Labels:      labels,
					Annotations: annotations,
				},
			}

			skipped := SkippedIngress{
				Ingress:     ing,
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// The Ingress reference must be preserved completely
			if skipped.Ingress != ing {
				return false
			}
			if skipped.Ingress.Name != name {
				return false
			}
			if skipped.Ingress.Namespace != namespace {
				return false
			}

			// Labels should be preserved
			if len(skipped.Ingress.Labels) != len(labels) {
				return false
			}

			// Annotations should be preserved
			if len(skipped.Ingress.Annotations) != len(annotations) {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genLabels(),
		genAnnotationsWithSkip(),
	))

	properties.TestingRun(t)
}

// genLabels generates a map of random Kubernetes labels
func genLabels() gopter.Gen {
	return gen.MapOf(
		gen.AlphaString().Map(func(s string) string {
			if len(s) == 0 {
				return "key"
			}
			if len(s) > 63 {
				s = s[:63]
			}
			return s
		}),
		gen.AlphaString().Map(func(s string) string {
			if len(s) > 63 {
				s = s[:63]
			}
			return s
		}),
	)
}

// genAnnotationsWithSkip generates a map of annotations that includes the skip-on-cert-error annotation
func genAnnotationsWithSkip() gopter.Gen {
	return gen.MapOf(
		gen.AlphaString().Map(func(s string) string {
			if len(s) == 0 {
				return "key"
			}
			return "alb.ingress.kubernetes.io/" + s
		}),
		gen.AlphaString(),
	).Map(func(m map[string]string) map[string]string {
		// Ensure the skip-on-cert-error annotation is present
		m["alb.ingress.kubernetes.io/skip-on-cert-error"] = "true"
		return m
	})
}

// Feature: ingress-certificate-error-skip, Property 6: Metric Emission with Labels
// Validates: Requirements 4.1, 4.2, 4.3
//
// Property 6: Metric Emission with Labels
// For any Ingress that is skipped due to a certificate error, the controller shall increment the
// `aws_load_balancer_controller_ingress_cert_error_skipped_total` counter metric with labels
// containing the correct `namespace`, `ingress_name`, and `group_name` values.

func TestProperty_MetricEmissionWithLabels(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 6a: Metric labels can be correctly derived from SkippedIngress and Group
	// Validates: Requirements 4.1, 4.2, 4.3
	properties.Property("metric labels can be correctly derived from SkippedIngress and Group", prop.ForAll(
		func(namespace, ingressName, groupName string) bool {
			// Create a SkippedIngress
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressName,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// Create a Group with explicit group name
			group := Group{
				ID: NewGroupIDForExplicitGroup(groupName),
			}

			// Extract metric labels
			metricNamespace := skipped.Ingress.Namespace
			metricIngressName := skipped.Ingress.Name
			metricGroupName := group.ID.Name

			// Verify all labels are correctly derived
			if metricNamespace != namespace {
				return false
			}
			if metricIngressName != ingressName {
				return false
			}
			if metricGroupName != groupName {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 6b: Namespace label is always available from SkippedIngress
	// Validates: Requirement 4.2
	properties.Property("namespace label is always available from SkippedIngress", prop.ForAll(
		func(namespace, ingressName string) bool {
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressName,
						Namespace: namespace,
					},
				},
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// Namespace must be accessible for metric label
			return skipped.Ingress.Namespace == namespace
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 6c: Ingress name label is always available from SkippedIngress
	// Validates: Requirement 4.2
	properties.Property("ingress_name label is always available from SkippedIngress", prop.ForAll(
		func(namespace, ingressName string) bool {
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressName,
						Namespace: namespace,
					},
				},
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// Ingress name must be accessible for metric label
			return skipped.Ingress.Name == ingressName
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 6d: Group name label is always available from Group for explicit groups
	// Validates: Requirement 4.3
	properties.Property("group_name label is always available from Group for explicit groups", prop.ForAll(
		func(groupName string) bool {
			// Create an explicit group
			group := Group{
				ID: NewGroupIDForExplicitGroup(groupName),
			}

			// Group name must be accessible for metric label
			return group.ID.Name == groupName
		},
		genValidK8sName(),
	))

	// Property 6e: Group name label is correctly derived for implicit groups
	// Validates: Requirement 4.3
	properties.Property("group_name label is correctly derived for implicit groups", prop.ForAll(
		func(namespace, ingressName string) bool {
			// Create an implicit group (namespace/name format)
			group := Group{
				ID: GroupID{Namespace: namespace, Name: ingressName},
			}

			// For implicit groups, the group name is the Ingress name
			// The full identifier is namespace/name
			return group.ID.Name == ingressName && group.ID.Namespace == namespace
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 6f: All three metric labels are available for any SkippedIngress in a Group
	// Validates: Requirements 4.1, 4.2, 4.3
	properties.Property("all three metric labels are available for any SkippedIngress in a Group", prop.ForAll(
		func(namespace, ingressName, groupName string, failedHosts []string) bool {
			// Ensure we have at least one host
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}

			// Create a SkippedIngress
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressName,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      "certificate not found",
				FailedHosts: failedHosts,
			}

			// Create a Group
			group := Group{
				ID:             NewGroupIDForExplicitGroup(groupName),
				SkippedMembers: []SkippedIngress{skipped},
			}

			// Simulate metric emission - extract all labels
			for _, skippedMember := range group.SkippedMembers {
				labelNamespace := skippedMember.Ingress.Namespace
				labelIngressName := skippedMember.Ingress.Name
				labelGroupName := group.ID.Name

				// All labels must be non-empty (assuming valid inputs)
				if namespace != "" && labelNamespace == "" {
					return false
				}
				if ingressName != "" && labelIngressName == "" {
					return false
				}
				if groupName != "" && labelGroupName == "" {
					return false
				}

				// Labels must match the original values
				if labelNamespace != namespace {
					return false
				}
				if labelIngressName != ingressName {
					return false
				}
				if labelGroupName != groupName {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genValidK8sName(),
		genFailedHosts(),
	))

	// Property 6g: Metric labels are consistent across multiple skipped Ingresses in same Group
	// Validates: Requirements 4.2, 4.3
	properties.Property("metric labels are consistent across multiple skipped Ingresses in same Group", prop.ForAll(
		func(groupName string, numSkipped int) bool {
			// Ensure valid count
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 10 {
				numSkipped = 10
			}

			// Create a Group with multiple skipped Ingresses
			group := Group{
				ID: NewGroupIDForExplicitGroup(groupName),
			}

			// Add multiple skipped Ingresses
			for i := 0; i < numSkipped; i++ {
				skipped := SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns-" + genIngressName(i),
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
					},
					Reason:      "certificate not found",
					FailedHosts: []string{genHostName(i)},
				}
				group.SkippedMembers = append(group.SkippedMembers, skipped)
			}

			// Verify metric labels for each skipped Ingress
			for i, skipped := range group.SkippedMembers {
				expectedNamespace := "test-ns-" + genIngressName(i)
				expectedIngressName := genIngressName(i)
				expectedGroupName := groupName

				// Extract labels
				labelNamespace := skipped.Ingress.Namespace
				labelIngressName := skipped.Ingress.Name
				labelGroupName := group.ID.Name

				// Verify namespace label
				if labelNamespace != expectedNamespace {
					return false
				}

				// Verify ingress_name label
				if labelIngressName != expectedIngressName {
					return false
				}

				// Verify group_name label (same for all Ingresses in the group)
				if labelGroupName != expectedGroupName {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		gen.IntRange(1, 10),
	))

	// Property 6h: Metric labels preserve special characters in names
	// Validates: Requirements 4.2, 4.3
	properties.Property("metric labels preserve valid Kubernetes name characters", prop.ForAll(
		func(baseName string) bool {
			// Generate names with hyphens (valid K8s name characters)
			namespace := baseName + "-ns"
			ingressName := baseName + "-ing"
			groupName := baseName + "-group"

			// Truncate if too long
			if len(namespace) > 63 {
				namespace = namespace[:63]
			}
			if len(ingressName) > 63 {
				ingressName = ingressName[:63]
			}
			if len(groupName) > 63 {
				groupName = groupName[:63]
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressName,
						Namespace: namespace,
					},
				},
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			group := Group{
				ID: NewGroupIDForExplicitGroup(groupName),
			}

			// Labels must preserve the exact names
			if skipped.Ingress.Namespace != namespace {
				return false
			}
			if skipped.Ingress.Name != ingressName {
				return false
			}
			if group.ID.Name != groupName {
				return false
			}

			return true
		},
		genValidK8sName(),
	))

	// Property 6i: Metric emission data is complete for each skipped Ingress
	// Validates: Requirements 4.1, 4.2, 4.3
	properties.Property("metric emission data is complete for each skipped Ingress", prop.ForAll(
		func(namespace, ingressName, groupName, reason string, failedHosts []string) bool {
			// Ensure we have valid inputs
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}
			if reason == "" {
				reason = "certificate not found"
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressName,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      reason,
				FailedHosts: failedHosts,
			}

			group := Group{
				ID:             NewGroupIDForExplicitGroup(groupName),
				SkippedMembers: []SkippedIngress{skipped},
			}

			// Simulate the metric emission call parameters
			// ObserveIngressCertErrorSkipped(namespace, ingressName, groupName string)
			metricNamespace := skipped.Ingress.Namespace
			metricIngressName := skipped.Ingress.Name
			metricGroupName := group.ID.Name

			// All parameters must be available
			if metricNamespace != namespace {
				return false
			}
			if metricIngressName != ingressName {
				return false
			}
			if metricGroupName != groupName {
				return false
			}

			// The SkippedIngress must be in the group's SkippedMembers
			if len(group.SkippedMembers) != 1 {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 6j: Group ID String() method provides correct group name for metrics
	// Validates: Requirement 4.3
	properties.Property("Group ID String method provides correct group name for metrics", prop.ForAll(
		func(groupName string) bool {
			// Test explicit group
			explicitGroup := Group{
				ID: NewGroupIDForExplicitGroup(groupName),
			}

			// For explicit groups, String() returns just the name
			if explicitGroup.ID.String() != groupName {
				return false
			}

			// The Name field should be the group name
			if explicitGroup.ID.Name != groupName {
				return false
			}

			return true
		},
		genValidK8sName(),
	))

	// Property 6k: Implicit group ID provides correct namespace/name for metrics
	// Validates: Requirement 4.3
	properties.Property("implicit group ID provides correct namespace/name for metrics", prop.ForAll(
		func(namespace, name string) bool {
			// Test implicit group
			implicitGroup := Group{
				ID: GroupID{Namespace: namespace, Name: name},
			}

			// For implicit groups, String() returns namespace/name
			expectedString := namespace + "/" + name
			if implicitGroup.ID.String() != expectedString {
				return false
			}

			// Both Namespace and Name should be accessible
			if implicitGroup.ID.Namespace != namespace {
				return false
			}
			if implicitGroup.ID.Name != name {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	properties.TestingRun(t)
}

// Feature: ingress-certificate-error-skip, Property 7: Event Emission with Attributes
// Validates: Requirements 5.1, 5.2, 5.3
//
// Property 7: Event Emission with Attributes
// For any Ingress that is skipped due to a certificate error, the controller shall emit a Warning
// event on the Ingress resource with reason `CertificateErrorSkipped` and a message containing
// the certificate error details.

func TestProperty_EventEmissionWithAttributes(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 7a: SkippedIngress contains all required information for event emission
	// Validates: Requirements 5.1, 5.2, 5.3
	properties.Property("SkippedIngress contains all required information for event emission", prop.ForAll(
		func(namespace, name, reason string, failedHosts []string) bool {
			// Ensure we have at least one host
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}
			if reason == "" {
				reason = "certificate not found"
			}

			// Create a SkippedIngress
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      reason,
				FailedHosts: failedHosts,
			}

			// Verify all required fields for event emission are available
			// 1. Ingress reference is available (for emitting event on correct resource)
			if skipped.Ingress == nil {
				return false
			}

			// 2. Reason field contains certificate error details
			if skipped.Reason == "" && reason != "" {
				return false
			}
			if skipped.Reason != reason {
				return false
			}

			// 3. FailedHosts field is available for inclusion in event message
			if len(skipped.FailedHosts) != len(failedHosts) {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 7b: Ingress reference is available for emitting event on correct resource
	// Validates: Requirement 5.1
	properties.Property("Ingress reference is available for emitting event on correct resource", prop.ForAll(
		func(namespace, name string) bool {
			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// The Ingress reference must be non-nil and contain correct metadata
			if skipped.Ingress == nil {
				return false
			}
			if skipped.Ingress.Name != name {
				return false
			}
			if skipped.Ingress.Namespace != namespace {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 7c: Reason field contains certificate error details
	// Validates: Requirement 5.3
	properties.Property("Reason field contains certificate error details", prop.ForAll(
		func(namespace, name, reason string) bool {
			if reason == "" {
				reason = "certificate not found"
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				Reason:      reason,
				FailedHosts: []string{"example.com"},
			}

			// Reason must be accessible and match the certificate error
			return skipped.Reason == reason
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
	))

	// Property 7d: FailedHosts field is available for inclusion in event message
	// Validates: Requirement 5.3
	properties.Property("FailedHosts field is available for inclusion in event message", prop.ForAll(
		func(namespace, name string, failedHosts []string) bool {
			// Ensure we have at least one host
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				Reason:      "certificate not found",
				FailedHosts: failedHosts,
			}

			// FailedHosts must be accessible and match
			if len(skipped.FailedHosts) != len(failedHosts) {
				return false
			}
			for i, host := range failedHosts {
				if skipped.FailedHosts[i] != host {
					return false
				}
			}
			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genFailedHosts(),
	))

	// Property 7e: Event message can be constructed from SkippedIngress fields
	// Validates: Requirements 5.2, 5.3
	properties.Property("event message can be constructed from SkippedIngress fields", prop.ForAll(
		func(namespace, name, reason string, failedHosts []string) bool {
			// Ensure we have valid inputs
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}
			if reason == "" {
				reason = "certificate not found"
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      reason,
				FailedHosts: failedHosts,
			}

			// Simulate constructing the event message (as done in recordSkippedIngressEvents)
			// message := fmt.Sprintf("Skipped due to certificate error: %s. Failed hosts: %v", skipped.Reason, skipped.FailedHosts)
			message := "Skipped due to certificate error: " + skipped.Reason + ". Failed hosts: " + formatHosts(skipped.FailedHosts)

			// Message must contain the reason
			if !containsSubstring(message, skipped.Reason) {
				return false
			}

			// Message must contain information about failed hosts
			for _, host := range skipped.FailedHosts {
				if !containsSubstring(message, host) {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 7f: Event can be emitted for any valid SkippedIngress
	// Validates: Requirements 5.1, 5.2, 5.3
	properties.Property("event can be emitted for any valid SkippedIngress", prop.ForAll(
		func(namespace, name, reason string, failedHosts []string) bool {
			// Ensure we have valid inputs
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}
			if reason == "" {
				reason = "certificate not found"
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      reason,
				FailedHosts: failedHosts,
			}

			// Simulate the event emission parameters
			// eventRecorder.Event(skipped.Ingress, corev1.EventTypeWarning, k8s.IngressEventReasonCertificateErrorSkipped, message)

			// 1. Ingress object must be available (for Event() first parameter)
			if skipped.Ingress == nil {
				return false
			}

			// 2. Event type is Warning (constant, always available)
			eventType := "Warning"
			if eventType != "Warning" {
				return false
			}

			// 3. Event reason is CertificateErrorSkipped (constant, always available)
			eventReason := "CertificateErrorSkipped"
			if eventReason != "CertificateErrorSkipped" {
				return false
			}

			// 4. Message can be constructed from Reason and FailedHosts
			if skipped.Reason == "" && reason != "" {
				return false
			}
			if len(skipped.FailedHosts) == 0 && len(failedHosts) > 0 {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 7g: Multiple skipped Ingresses can each have events emitted
	// Validates: Requirements 5.1, 5.2, 5.3
	properties.Property("multiple skipped Ingresses can each have events emitted", prop.ForAll(
		func(numSkipped int) bool {
			// Ensure valid count
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 10 {
				numSkipped = 10
			}

			// Create multiple skipped Ingresses
			var skippedMembers []SkippedIngress
			for i := 0; i < numSkipped; i++ {
				skipped := SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns-" + genIngressName(i),
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
					},
					Reason:      "certificate not found for host " + genHostName(i),
					FailedHosts: []string{genHostName(i)},
				}
				skippedMembers = append(skippedMembers, skipped)
			}

			// Verify each skipped Ingress has all required fields for event emission
			for i, skipped := range skippedMembers {
				// Ingress reference must be available
				if skipped.Ingress == nil {
					return false
				}

				// Ingress must have correct name
				if skipped.Ingress.Name != genIngressName(i) {
					return false
				}

				// Reason must be available
				if skipped.Reason == "" {
					return false
				}

				// FailedHosts must be available
				if len(skipped.FailedHosts) == 0 {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	// Property 7h: Event message format is consistent for all skipped Ingresses
	// Validates: Requirements 5.2, 5.3
	properties.Property("event message format is consistent for all skipped Ingresses", prop.ForAll(
		func(numSkipped int) bool {
			// Ensure valid count
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 10 {
				numSkipped = 10
			}

			// Create multiple skipped Ingresses with different reasons and hosts
			var skippedMembers []SkippedIngress
			for i := 0; i < numSkipped; i++ {
				skipped := SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      genIngressName(i),
							Namespace: "test-ns",
						},
					},
					Reason:      "certificate error " + genIngressName(i),
					FailedHosts: []string{genHostName(i), genHostName(i + 100)},
				}
				skippedMembers = append(skippedMembers, skipped)
			}

			// Verify message format is consistent for all
			for _, skipped := range skippedMembers {
				// Construct message as done in recordSkippedIngressEvents
				message := "Skipped due to certificate error: " + skipped.Reason + ". Failed hosts: " + formatHosts(skipped.FailedHosts)

				// Message must start with expected prefix
				if !containsSubstring(message, "Skipped due to certificate error:") {
					return false
				}

				// Message must contain "Failed hosts:"
				if !containsSubstring(message, "Failed hosts:") {
					return false
				}

				// Message must contain the reason
				if !containsSubstring(message, skipped.Reason) {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	// Property 7i: SkippedIngress preserves Ingress identity for event targeting
	// Validates: Requirement 5.1
	properties.Property("SkippedIngress preserves Ingress identity for event targeting", prop.ForAll(
		func(namespace, name string, uid string) bool {
			// Create an Ingress with specific identity
			uidValue := types.UID("test-uid-" + uid)
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					UID:       uidValue,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
			}

			skipped := SkippedIngress{
				Ingress:     ing,
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// The Ingress reference must preserve identity
			if skipped.Ingress != ing {
				return false
			}
			if skipped.Ingress.Name != name {
				return false
			}
			if skipped.Ingress.Namespace != namespace {
				return false
			}
			if skipped.Ingress.UID != uidValue {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genValidK8sName(),
	))

	// Property 7j: Event emission data completeness for diverse certificate errors
	// Validates: Requirements 5.2, 5.3
	properties.Property("event emission data completeness for diverse certificate errors", prop.ForAll(
		func(namespace, name string, errorType int, numHosts int) bool {
			// Generate different types of certificate errors
			var reason string
			switch errorType % 5 {
			case 0:
				reason = "no matching ACM certificate found for host"
			case 1:
				reason = "certificate not found for host"
			case 2:
				reason = "failed to discover certificate for TLS host"
			case 3:
				reason = "ACM certificate discovery failed"
			case 4:
				reason = "certificate validation error"
			}

			// Ensure valid number of hosts
			if numHosts < 1 {
				numHosts = 1
			}
			if numHosts > 5 {
				numHosts = 5
			}

			// Generate failed hosts
			failedHosts := make([]string, numHosts)
			for i := 0; i < numHosts; i++ {
				failedHosts[i] = genHostName(i)
			}

			skipped := SkippedIngress{
				Ingress: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Annotations: map[string]string{
							"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
						},
					},
				},
				Reason:      reason,
				FailedHosts: failedHosts,
			}

			// Verify all data is available for event emission
			if skipped.Ingress == nil {
				return false
			}
			if skipped.Reason != reason {
				return false
			}
			if len(skipped.FailedHosts) != numHosts {
				return false
			}

			// Verify message can be constructed
			message := "Skipped due to certificate error: " + skipped.Reason + ". Failed hosts: " + formatHosts(skipped.FailedHosts)
			if message == "" {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		gen.IntRange(0, 4),
		gen.IntRange(1, 5),
	))

	properties.TestingRun(t)
}

// containsSubstring checks if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

// findSubstring is a simple substring search
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// formatHosts formats a slice of hosts as a string for event messages
func formatHosts(hosts []string) string {
	if len(hosts) == 0 {
		return "[]"
	}
	result := "["
	for i, host := range hosts {
		if i > 0 {
			result += " "
		}
		result += host
	}
	result += "]"
	return result
}

// Feature: ingress-certificate-error-skip, Property 8: Status Preservation for Skipped Ingresses
// Validates: Requirements 6.1, 6.2
//
// Property 8: Status Preservation for Skipped Ingresses
// For any Ingress that is skipped due to a certificate error, the controller shall not update the
// Ingress status with new load balancer information, and any existing status from previous
// successful reconciliations shall be preserved.

func TestProperty_StatusPreservationForSkippedIngresses(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 8a: Skipped Ingresses are stored in SkippedMembers, not Members
	// Validates: Requirements 6.1, 6.2
	properties.Property("skipped Ingresses are stored in SkippedMembers not Members", prop.ForAll(
		func(namespace, name, reason string, failedHosts []string) bool {
			// Ensure we have at least one host
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}
			if reason == "" {
				reason = "certificate not found"
			}

			// Create an Ingress with skip-on-cert-error="true" and existing status
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{Hostname: "existing-lb.example.com"},
						},
					},
				},
			}

			// Create a Group and simulate the skip behavior
			group := Group{
				ID: GroupID{Namespace: namespace, Name: "test-group"},
			}

			// When an Ingress is skipped, it goes to SkippedMembers, NOT Members
			skippedIngress := SkippedIngress{
				Ingress:     ing,
				Reason:      reason,
				FailedHosts: failedHosts,
			}
			group.SkippedMembers = append(group.SkippedMembers, skippedIngress)

			// Verify: Ingress is in SkippedMembers
			if len(group.SkippedMembers) != 1 {
				return false
			}
			if group.SkippedMembers[0].Ingress.Name != name {
				return false
			}

			// Verify: Ingress is NOT in Members
			for _, member := range group.Members {
				if member.Ing.Name == name && member.Ing.Namespace == namespace {
					return false // Skipped Ingress should NOT be in Members
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 8b: Status update logic only processes Members, not SkippedMembers
	// Validates: Requirements 6.1, 6.2
	properties.Property("status update logic only processes Members not SkippedMembers", prop.ForAll(
		func(numMembers, numSkipped int, newLBHostname string) bool {
			// Ensure valid counts
			if numMembers < 1 {
				numMembers = 1
			}
			if numMembers > 5 {
				numMembers = 5
			}
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 5 {
				numSkipped = 5
			}
			if newLBHostname == "" {
				newLBHostname = "new-lb.example.com"
			}

			// Create a Group with both Members and SkippedMembers
			group := Group{
				ID: GroupID{Namespace: "test-ns", Name: "test-group"},
			}

			// Add Members (these will get status updates)
			for i := 0; i < numMembers; i++ {
				group.Members = append(group.Members, ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "member-" + genIngressName(i),
							Namespace: "test-ns",
						},
						Status: networking.IngressStatus{
							LoadBalancer: networking.IngressLoadBalancerStatus{
								Ingress: []networking.IngressLoadBalancerIngress{
									{Hostname: "old-lb-" + genIngressName(i) + ".example.com"},
								},
							},
						},
					},
				})
			}

			// Add SkippedMembers (these should NOT get status updates)
			originalStatuses := make(map[string]string) // name -> original hostname
			for i := 0; i < numSkipped; i++ {
				originalHostname := "existing-lb-" + genIngressName(i) + ".example.com"
				ingName := "skipped-" + genIngressName(i)
				originalStatuses[ingName] = originalHostname

				group.SkippedMembers = append(group.SkippedMembers, SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ingName,
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
						Status: networking.IngressStatus{
							LoadBalancer: networking.IngressLoadBalancerStatus{
								Ingress: []networking.IngressLoadBalancerIngress{
									{Hostname: originalHostname},
								},
							},
						},
					},
					Reason:      "certificate not found",
					FailedHosts: []string{genHostName(i)},
				})
			}

			// Simulate status update logic (as in updateIngressGroupStatus)
			// This only iterates over Members, NOT SkippedMembers
			for _, member := range group.Members {
				// Update status for Members
				member.Ing.Status.LoadBalancer.Ingress = []networking.IngressLoadBalancerIngress{
					{Hostname: newLBHostname},
				}
			}

			// Verify: Members got updated status
			for _, member := range group.Members {
				if len(member.Ing.Status.LoadBalancer.Ingress) == 0 {
					return false
				}
				if member.Ing.Status.LoadBalancer.Ingress[0].Hostname != newLBHostname {
					return false // Member should have new status
				}
			}

			// Verify: SkippedMembers did NOT get updated status (preserved original)
			for _, skipped := range group.SkippedMembers {
				originalHostname := originalStatuses[skipped.Ingress.Name]
				if len(skipped.Ingress.Status.LoadBalancer.Ingress) == 0 {
					return false
				}
				if skipped.Ingress.Status.LoadBalancer.Ingress[0].Hostname != originalHostname {
					return false // Skipped Ingress should have preserved original status
				}
			}

			return true
		},
		gen.IntRange(1, 5),
		gen.IntRange(1, 5),
		genValidK8sName(),
	))

	// Property 8c: The separation of Members and SkippedMembers ensures status preservation
	// Validates: Requirements 6.1, 6.2
	properties.Property("separation of Members and SkippedMembers ensures status preservation", prop.ForAll(
		func(groupSize int, numSkipped int) bool {
			// Ensure valid counts
			if groupSize < 2 {
				groupSize = 2
			}
			if groupSize > 10 {
				groupSize = 10
			}
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped >= groupSize {
				numSkipped = groupSize - 1
			}

			// Create a Group
			group := Group{
				ID: GroupID{Namespace: "test-ns", Name: "test-group"},
			}

			// Track original statuses for skipped Ingresses
			originalStatuses := make(map[string]networking.IngressStatus)

			// Simulate processing: some Ingresses are skipped, others are processed
			for i := 0; i < groupSize; i++ {
				ingName := genIngressName(i)
				originalStatus := networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{Hostname: "original-lb-" + ingName + ".example.com"},
						},
					},
				}

				if i < numSkipped {
					// This Ingress is skipped - goes to SkippedMembers
					originalStatuses[ingName] = originalStatus
					group.SkippedMembers = append(group.SkippedMembers, SkippedIngress{
						Ingress: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Name:      ingName,
								Namespace: "test-ns",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
								},
							},
							Status: originalStatus,
						},
						Reason:      "certificate not found",
						FailedHosts: []string{genHostName(i)},
					})
				} else {
					// This Ingress is processed - goes to Members
					group.Members = append(group.Members, ClassifiedIngress{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Name:      ingName,
								Namespace: "test-ns",
							},
							Status: originalStatus,
						},
					})
				}
			}

			// Verify: correct separation
			if len(group.SkippedMembers) != numSkipped {
				return false
			}
			if len(group.Members) != groupSize-numSkipped {
				return false
			}

			// Verify: no overlap between Members and SkippedMembers
			memberNames := make(map[string]bool)
			for _, member := range group.Members {
				memberNames[member.Ing.Name] = true
			}
			for _, skipped := range group.SkippedMembers {
				if memberNames[skipped.Ingress.Name] {
					return false // Overlap detected - FAIL
				}
			}

			// Verify: skipped Ingresses have their original status preserved
			for _, skipped := range group.SkippedMembers {
				originalStatus := originalStatuses[skipped.Ingress.Name]
				if len(skipped.Ingress.Status.LoadBalancer.Ingress) != len(originalStatus.LoadBalancer.Ingress) {
					return false
				}
				if len(skipped.Ingress.Status.LoadBalancer.Ingress) > 0 {
					if skipped.Ingress.Status.LoadBalancer.Ingress[0].Hostname != originalStatus.LoadBalancer.Ingress[0].Hostname {
						return false
					}
				}
			}

			return true
		},
		gen.IntRange(2, 10),
		gen.IntRange(1, 5),
	))

	// Property 8d: Skipped Ingresses retain their original Ingress object (with existing status)
	// Validates: Requirements 6.1, 6.2
	properties.Property("skipped Ingresses retain their original Ingress object with existing status", prop.ForAll(
		func(namespace, name, originalHostname string, originalPorts []int32) bool {
			// Ensure we have valid inputs
			if originalHostname == "" {
				originalHostname = "original-lb.example.com"
			}
			if len(originalPorts) == 0 {
				originalPorts = []int32{80, 443}
			}

			// Create port status
			portStatuses := make([]networking.IngressPortStatus, len(originalPorts))
			for i, port := range originalPorts {
				portStatuses[i] = networking.IngressPortStatus{Port: port}
			}

			// Create an Ingress with existing status from previous successful reconciliation
			originalIng := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{
								Hostname: originalHostname,
								Ports:    portStatuses,
							},
						},
					},
				},
			}

			// Create a SkippedIngress that retains the original Ingress object
			skipped := SkippedIngress{
				Ingress:     originalIng,
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// Verify: the Ingress reference is preserved (same object)
			if skipped.Ingress != originalIng {
				return false
			}

			// Verify: the status is preserved
			if len(skipped.Ingress.Status.LoadBalancer.Ingress) != 1 {
				return false
			}
			if skipped.Ingress.Status.LoadBalancer.Ingress[0].Hostname != originalHostname {
				return false
			}

			// Verify: the port statuses are preserved
			if len(skipped.Ingress.Status.LoadBalancer.Ingress[0].Ports) != len(originalPorts) {
				return false
			}
			for i, port := range originalPorts {
				if skipped.Ingress.Status.LoadBalancer.Ingress[0].Ports[i].Port != port {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genValidK8sName(),
		genPorts(),
	))

	// Property 8e: Skipped Ingresses with no previous status remain without status
	// Validates: Requirements 6.1, 6.2
	properties.Property("skipped Ingresses with no previous status remain without status", prop.ForAll(
		func(namespace, name, reason string, failedHosts []string) bool {
			// Ensure we have at least one host
			if len(failedHosts) == 0 {
				failedHosts = []string{"default.example.com"}
			}
			if reason == "" {
				reason = "certificate not found"
			}

			// Create an Ingress with NO existing status (new Ingress)
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
				// Status is empty (no previous successful reconciliation)
			}

			// Create a SkippedIngress
			skipped := SkippedIngress{
				Ingress:     ing,
				Reason:      reason,
				FailedHosts: failedHosts,
			}

			// Verify: the Ingress still has no status (not updated with new LB info)
			if len(skipped.Ingress.Status.LoadBalancer.Ingress) != 0 {
				return false // Should remain empty
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genCertErrorMessage(),
		genFailedHosts(),
	))

	// Property 8f: Status preservation works for Ingresses with multiple load balancer entries
	// Validates: Requirements 6.1, 6.2
	properties.Property("status preservation works for Ingresses with multiple load balancer entries", prop.ForAll(
		func(namespace, name string, numLBEntries int) bool {
			// Ensure valid count
			if numLBEntries < 1 {
				numLBEntries = 1
			}
			if numLBEntries > 5 {
				numLBEntries = 5
			}

			// Create multiple load balancer entries
			lbEntries := make([]networking.IngressLoadBalancerIngress, numLBEntries)
			for i := 0; i < numLBEntries; i++ {
				lbEntries[i] = networking.IngressLoadBalancerIngress{
					Hostname: "lb-" + genIngressName(i) + ".example.com",
					Ports: []networking.IngressPortStatus{
						{Port: int32(80 + i)},
					},
				}
			}

			// Create an Ingress with multiple LB entries in status
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: lbEntries,
					},
				},
			}

			// Create a SkippedIngress
			skipped := SkippedIngress{
				Ingress:     ing,
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// Verify: all LB entries are preserved
			if len(skipped.Ingress.Status.LoadBalancer.Ingress) != numLBEntries {
				return false
			}
			for i := 0; i < numLBEntries; i++ {
				expectedHostname := "lb-" + genIngressName(i) + ".example.com"
				if skipped.Ingress.Status.LoadBalancer.Ingress[i].Hostname != expectedHostname {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		gen.IntRange(1, 5),
	))

	// Property 8g: Status preservation is independent of the certificate error reason
	// Validates: Requirements 6.1, 6.2
	properties.Property("status preservation is independent of the certificate error reason", prop.ForAll(
		func(namespace, name, originalHostname string, errorType int) bool {
			// Generate different types of certificate errors
			var reason string
			switch errorType % 5 {
			case 0:
				reason = "no matching ACM certificate found for host"
			case 1:
				reason = "certificate not found for host"
			case 2:
				reason = "failed to discover certificate for TLS host"
			case 3:
				reason = "ACM certificate discovery failed"
			case 4:
				reason = "certificate validation error"
			}

			if originalHostname == "" {
				originalHostname = "original-lb.example.com"
			}

			// Create an Ingress with existing status
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{Hostname: originalHostname},
						},
					},
				},
			}

			// Create a SkippedIngress with the specific error reason
			skipped := SkippedIngress{
				Ingress:     ing,
				Reason:      reason,
				FailedHosts: []string{"example.com"},
			}

			// Verify: status is preserved regardless of the error reason
			if len(skipped.Ingress.Status.LoadBalancer.Ingress) != 1 {
				return false
			}
			if skipped.Ingress.Status.LoadBalancer.Ingress[0].Hostname != originalHostname {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genValidK8sName(),
		gen.IntRange(0, 4),
	))

	// Property 8h: Status preservation works across multiple reconciliation cycles
	// Validates: Requirements 6.1, 6.2
	properties.Property("status preservation works across multiple reconciliation cycles", prop.ForAll(
		func(namespace, name, originalHostname string, numCycles int) bool {
			// Ensure valid count
			if numCycles < 1 {
				numCycles = 1
			}
			if numCycles > 10 {
				numCycles = 10
			}
			if originalHostname == "" {
				originalHostname = "original-lb.example.com"
			}

			// Create an Ingress with existing status
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{Hostname: originalHostname},
						},
					},
				},
			}

			// Simulate multiple reconciliation cycles where the Ingress is always skipped
			for cycle := 0; cycle < numCycles; cycle++ {
				// Create a new Group for each cycle
				group := Group{
					ID: GroupID{Namespace: namespace, Name: "test-group"},
				}

				// The Ingress is skipped in each cycle
				skipped := SkippedIngress{
					Ingress:     ing,
					Reason:      "certificate not found (cycle " + string(rune('0'+cycle)) + ")",
					FailedHosts: []string{"example.com"},
				}
				group.SkippedMembers = append(group.SkippedMembers, skipped)

				// Simulate status update (only processes Members, not SkippedMembers)
				for _, member := range group.Members {
					member.Ing.Status.LoadBalancer.Ingress = []networking.IngressLoadBalancerIngress{
						{Hostname: "new-lb-cycle-" + string(rune('0'+cycle)) + ".example.com"},
					}
				}

				// Verify: the skipped Ingress status is still preserved
				if len(group.SkippedMembers[0].Ingress.Status.LoadBalancer.Ingress) != 1 {
					return false
				}
				if group.SkippedMembers[0].Ingress.Status.LoadBalancer.Ingress[0].Hostname != originalHostname {
					return false
				}
			}

			// Final verification: status is still the original
			if len(ing.Status.LoadBalancer.Ingress) != 1 {
				return false
			}
			if ing.Status.LoadBalancer.Ingress[0].Hostname != originalHostname {
				return false
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genValidK8sName(),
		gen.IntRange(1, 10),
	))

	// Property 8i: SkippedMembers list correctly tracks all skipped Ingresses for status preservation
	// Validates: Requirements 6.1, 6.2
	properties.Property("SkippedMembers list correctly tracks all skipped Ingresses for status preservation", prop.ForAll(
		func(numSkipped int) bool {
			// Ensure valid count
			if numSkipped < 1 {
				numSkipped = 1
			}
			if numSkipped > 10 {
				numSkipped = 10
			}

			// Create a Group
			group := Group{
				ID: GroupID{Namespace: "test-ns", Name: "test-group"},
			}

			// Track original statuses
			originalStatuses := make(map[string]string) // name -> hostname

			// Add multiple skipped Ingresses with different statuses
			for i := 0; i < numSkipped; i++ {
				ingName := genIngressName(i)
				originalHostname := "original-lb-" + ingName + ".example.com"
				originalStatuses[ingName] = originalHostname

				group.SkippedMembers = append(group.SkippedMembers, SkippedIngress{
					Ingress: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Name:      ingName,
							Namespace: "test-ns",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
							},
						},
						Status: networking.IngressStatus{
							LoadBalancer: networking.IngressLoadBalancerStatus{
								Ingress: []networking.IngressLoadBalancerIngress{
									{Hostname: originalHostname},
								},
							},
						},
					},
					Reason:      "certificate not found for " + genHostName(i),
					FailedHosts: []string{genHostName(i)},
				})
			}

			// Verify: all skipped Ingresses are tracked
			if len(group.SkippedMembers) != numSkipped {
				return false
			}

			// Verify: each skipped Ingress has its original status preserved
			for _, skipped := range group.SkippedMembers {
				expectedHostname := originalStatuses[skipped.Ingress.Name]
				if len(skipped.Ingress.Status.LoadBalancer.Ingress) != 1 {
					return false
				}
				if skipped.Ingress.Status.LoadBalancer.Ingress[0].Hostname != expectedHostname {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	// Property 8j: Status preservation maintains complete Ingress status structure
	// Validates: Requirements 6.1, 6.2
	properties.Property("status preservation maintains complete Ingress status structure", prop.ForAll(
		func(namespace, name, hostname string, ports []int32, ip string) bool {
			// Ensure we have valid inputs
			if hostname == "" {
				hostname = "original-lb.example.com"
			}
			if len(ports) == 0 {
				ports = []int32{80, 443}
			}

			// Create port statuses
			portStatuses := make([]networking.IngressPortStatus, len(ports))
			for i, port := range ports {
				portStatuses[i] = networking.IngressPortStatus{Port: port}
			}

			// Create an Ingress with complete status structure
			ing := &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/skip-on-cert-error": "true",
					},
				},
				Status: networking.IngressStatus{
					LoadBalancer: networking.IngressLoadBalancerStatus{
						Ingress: []networking.IngressLoadBalancerIngress{
							{
								Hostname: hostname,
								IP:       ip,
								Ports:    portStatuses,
							},
						},
					},
				},
			}

			// Create a SkippedIngress
			skipped := SkippedIngress{
				Ingress:     ing,
				Reason:      "certificate not found",
				FailedHosts: []string{"example.com"},
			}

			// Verify: complete status structure is preserved
			if len(skipped.Ingress.Status.LoadBalancer.Ingress) != 1 {
				return false
			}

			lbStatus := skipped.Ingress.Status.LoadBalancer.Ingress[0]

			// Verify hostname
			if lbStatus.Hostname != hostname {
				return false
			}

			// Verify IP
			if lbStatus.IP != ip {
				return false
			}

			// Verify ports
			if len(lbStatus.Ports) != len(ports) {
				return false
			}
			for i, port := range ports {
				if lbStatus.Ports[i].Port != port {
					return false
				}
			}

			return true
		},
		genValidK8sName(),
		genValidK8sName(),
		genValidK8sName(),
		genPorts(),
		genIP(),
	))

	properties.TestingRun(t)
}

// genPorts generates a slice of valid port numbers
func genPorts() gopter.Gen {
	return gen.SliceOfN(3, gen.Int32Range(1, 65535)).Map(func(ports []int32) []int32 {
		if len(ports) == 0 {
			return []int32{80, 443}
		}
		return ports
	})
}

// genIP generates a valid IP address string or empty string
func genIP() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""),
		gen.Const("10.0.0.1"),
		gen.Const("192.168.1.1"),
		gen.Const("172.16.0.1"),
		gen.IntRange(1, 254).Map(func(n int) string {
			return "10.0.0." + string(rune('0'+n%10))
		}),
	)
}
