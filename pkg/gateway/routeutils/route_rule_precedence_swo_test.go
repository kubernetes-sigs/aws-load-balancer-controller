package routeutils

// This file adds fast, deterministic "ordering-contract" gates for the Gateway
// API rule-precedence comparator. They target the class of defect:
// a comparator that is NOT a strict weak ordering (SWO), which makes
// sort.Slice produce an undefined, input-order-dependent result, plus semantic
// ordering regressions (e.g. a catch-all outranking a more specific path).
//
//
// Four complementary gates:
//
//  1. SWO property tests (axiom-based), exhaustive over a static corpus:
//       - getHostnamePrecedenceOrder (the hostname comparator, incl. ""=catch-all),
//         the three-way comparator whose non-transitive/empty handling matters most.
//       - compareRulePrecedenceUnified over HTTP-only, GRPC-only, and MIXED static
//         corpora, verifying the single comparator is a strict weak ordering.
//
//  2. Permutation-invariance of sort output over the static corpus: sorting the
//     corpus under identity/reverse/rotations/~1000 shuffles must always produce
//     the same key sequence, and sort.Slice must agree with sort.SliceStable.
//
//  3. Permutation-invariance of the full end-to-end SortAllRulesByPrecedence on
//     realistic MIXED (HTTP+GRPC) static route sets.
//
//  4. Golden ordering fixtures for SortAllRulesByPrecedence, including the exact
//     cross-route catch-all-vs-specific-path scenario from the gateway-api
//     conformance test httproute-matching-across-routes.

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func swoSign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	default:
		return 0
	}
}

// permStabilityShuffles is how many fixed-seed shuffles each permutation-invariance
// test applies to its static input. Combined with the identity, full-reverse and
// every-rotation permutations, this is the "run it many times, the ordering must
// never change" guarantee — with static data and a fixed seed, so it is both
// high-volume and fully reproducible.
const permStabilityShuffles = 1000

// permStabilitySeed seeds the shuffler used by the permutation-invariance tests.
// It only permutes the VISITING ORDER of static inputs; the scenario data itself
// is hand-authored and never randomized. Fixing the seed makes every "shuffle"
// identical across runs, so a failure always reproduces.
const permStabilitySeed int64 = 0x5303173 // any fixed value works

var swoTimes = []time.Time{
	time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
}

// assertBoolComparatorIsSWO verifies the four axioms a `less` comparator must
// satisfy to be a strict weak ordering (what sort.Slice requires):
//   - irreflexivity:            !less(a,a)
//   - asymmetry:                less(a,b) => !less(b,a)
//   - transitivity of <:        less(a,b) && less(b,c) => less(a,c)
//   - transitivity of ~ (equal): eq(a,b) && eq(b,c) => eq(a,c), where
//     eq(a,b) := !less(a,b) && !less(b,a)
//
// It is run EXHAUSTIVELY over every pair and triple of a static corpus, which is
// a strictly stronger guarantee than sampling random inputs: for the curated
// scenarios there is no pair or triple that can violate the contract. A violation
// is reported with the concrete indices/labels so failures are debuggable.
func assertBoolComparatorIsSWO(t *testing.T, items []RulePrecedence, less func(a, b RulePrecedence) bool, label func(i int) string) {
	t.Helper()
	n := len(items)

	// Precompute the pairwise "less" matrix once.
	lm := make([][]bool, n)
	for i := 0; i < n; i++ {
		lm[i] = make([]bool, n)
		for j := 0; j < n; j++ {
			lm[i][j] = less(items[i], items[j])
		}
	}
	eq := func(i, j int) bool { return !lm[i][j] && !lm[j][i] }

	for i := 0; i < n; i++ {
		if lm[i][i] {
			t.Fatalf("irreflexivity violated: less(x,x)=true for %s", label(i))
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if lm[i][j] && lm[j][i] {
				t.Fatalf("asymmetry violated: less(a,b) && less(b,a)\n a=%s\n b=%s", label(i), label(j))
			}
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if !lm[i][j] {
				continue
			}
			for k := 0; k < n; k++ {
				if lm[j][k] && !lm[i][k] {
					t.Fatalf("transitivity of < violated: less(a,b) && less(b,c) but !less(a,c)\n a=%s\n b=%s\n c=%s",
						label(i), label(j), label(k))
				}
			}
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if !eq(i, j) {
				continue
			}
			for k := 0; k < n; k++ {
				if eq(j, k) && !eq(i, k) {
					t.Fatalf("transitivity of equivalence violated: eq(a,b) && eq(b,c) but !eq(a,c)\n a=%s\n b=%s\n c=%s",
						label(i), label(j), label(k))
				}
			}
		}
	}
}

// assertSortOrderIsPermutationInvariant sorts `items` under many deterministic
// permutations of the INPUT ORDER and asserts the sorted key sequence never
// changes. This is the core stability guarantee: for a valid strict weak
// ordering the output of sort.Slice is a function of the multiset only, so every
// permutation of the same static input must sort to the same sequence. If the
// comparator is not an SWO, different input permutations can sort differently and
// this fails.
//
// The permutation set is: the identity, the full reverse, every single rotation,
// and permStabilityShuffles fixed-seed Fisher-Yates shuffles. Everything is
// deterministic, so a failure reproduces exactly.
//
// Keys are derived from the comparator's tiebreaker fields (see rpKey), so two
// elements share a key only when they are equivalent under the comparator;
// reordering equivalent elements therefore does not change the key sequence and
// cannot cause a spurious failure.
func assertSortOrderIsPermutationInvariant(t *testing.T, items []RulePrecedence, less func(a, b RulePrecedence) bool, label string) {
	t.Helper()

	sortCopy := func(in []RulePrecedence) []string {
		cp := append([]RulePrecedence(nil), in...)
		sort.Slice(cp, func(i, j int) bool { return less(cp[i], cp[j]) })
		return rpKeys(cp)
	}

	baseline := sortCopy(items)

	check := func(perm []RulePrecedence, desc string) {
		got := sortCopy(perm)
		if !equalKeys(baseline, got) {
			t.Fatalf("%s: sort order depends on input order (comparator not a strict weak ordering)\n permutation=%s\n baseline=%v\n got     =%v",
				label, desc, baseline, got)
		}
	}

	n := len(items)

	// Reverse.
	rev := make([]RulePrecedence, n)
	for i := range items {
		rev[n-1-i] = items[i]
	}
	check(rev, "reverse")

	// Every rotation.
	for r := 1; r < n; r++ {
		rot := make([]RulePrecedence, 0, n)
		rot = append(rot, items[r:]...)
		rot = append(rot, items[:r]...)
		check(rot, fmt.Sprintf("rotate-%d", r))
	}

	// Fixed-seed shuffles: the data is static, only the visiting order is permuted.
	rng := rand.New(rand.NewSource(permStabilitySeed))
	work := append([]RulePrecedence(nil), items...)
	for s := 0; s < permStabilityShuffles; s++ {
		rng.Shuffle(len(work), func(i, j int) { work[i], work[j] = work[j], work[i] })
		check(work, fmt.Sprintf("shuffle-%d", s))
	}

	// sort.Slice and sort.SliceStable must agree for a valid SWO comparator.
	a := append([]RulePrecedence(nil), items...)
	b := append([]RulePrecedence(nil), items...)
	sort.Slice(a, func(i, j int) bool { return less(a[i], a[j]) })
	sort.SliceStable(b, func(i, j int) bool { return less(b[i], b[j]) })
	if !equalKeys(rpKeys(a), rpKeys(b)) {
		t.Fatalf("%s: sort.Slice and sort.SliceStable disagree (comparator not a strict weak ordering)\n slice =%v\n stable=%v",
			label, rpKeys(a), rpKeys(b))
	}
}

// ---------------------------------------------------------------------------
// 1. Hostname comparator: static corpus + exhaustive SWO axioms
// ---------------------------------------------------------------------------

// swoHostnameCorpus is a set of single hostnames chosen to cover
// every dimension the hostname comparator orders on, plus the exact shapes that
// have to be transitive:
//
//   - "" (catch-all): the least specific value; must lose to every concrete
//     hostname, including wildcards.
//   - equal-specificity equivalence classes: {example.com, example.net,
//     example.org} are all 11 chars / 1 dot, so the comparator must rank them
//     equal (cmp==0). A class of size 3 is what makes equivalence-transitivity a
//     non-trivial check.
//   - dot-count vs length ordering: more dots outranks fewer dots; among equal
//     dot counts, more characters outranks fewer.
//   - wildcard vs non-wildcard at various depths, so wildcard handling is
//     transitive against concrete hosts.
var swoHostnameCorpus = []string{
	"", // catch-all, least specific

	// 1-dot non-wildcards; the three .com/.net/.org are an equal-specificity class.
	"example.com",
	"example.net",
	"example.org",
	"example.io", // 10 chars, 1 dot -> distinct (shorter) from the class above

	// 2-dot non-wildcards; a. and b. are an equal-specificity class, api. is longer.
	"a.example.com",
	"b.example.com",
	"api.example.com",

	// 3-dot / 4-dot non-wildcards.
	"foo.bar.example.com",
	"a.b.c.example.com",

	// wildcards at various depths.
	"*.io",
	"*.example.com",
	"*.sub.example.com",
}

// Test_getHostnamePrecedenceOrder_SWO_Static verifies the three-way hostname
// comparator is a strict weak ordering EXHAUSTIVELY over the static corpus:
// every pair (antisymmetry, reflexive-equality) and every triple (transitivity
// of strict order and of equivalence). "" (catch-all) is included so its
// least-specific handling is covered against concrete and wildcard hosts alike.
func Test_getHostnamePrecedenceOrder_SWO_Static(t *testing.T) {
	cmp := getHostnamePrecedenceOrder
	hosts := swoHostnameCorpus
	n := len(hosts)
	label := func(i int) string { return fmt.Sprintf("%q", hosts[i]) }

	// Reflexivity of equality and antisymmetry over all pairs.
	for i := 0; i < n; i++ {
		if cmp(hosts[i], hosts[i]) != 0 {
			t.Fatalf("cmp(x,x) != 0 for %s", label(i))
		}
		for j := 0; j < n; j++ {
			if swoSign(cmp(hosts[i], hosts[j])) != -swoSign(cmp(hosts[j], hosts[i])) {
				t.Fatalf("antisymmetry violated\n a=%s\n b=%s", label(i), label(j))
			}
		}
	}
	// Transitivity of strict order and of equivalence over all triples.
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			sij := swoSign(cmp(hosts[i], hosts[j]))
			for k := 0; k < n; k++ {
				sjk := swoSign(cmp(hosts[j], hosts[k]))
				sik := swoSign(cmp(hosts[i], hosts[k]))
				if sij < 0 && sjk < 0 && !(sik < 0) {
					t.Fatalf("strict transitivity violated\n a=%s\n b=%s\n c=%s", label(i), label(j), label(k))
				}
				if sij == 0 && sjk == 0 && sik != 0 {
					t.Fatalf("equivalence transitivity violated\n a=%s\n b=%s\n c=%s", label(i), label(j), label(k))
				}
			}
		}
	}
}

// Test_getHostnamePrecedenceOrder_Static_KnownOrder pins the semantic ordering
// of the corpus so a regression that reshuffles hostname precedence (not just
// one that breaks SWO) is caught. Equal-specificity hosts are grouped; within a
// group the relative order is "don't care" (cmp==0), so we assert group
// boundaries rather than a strict total order.
func Test_getHostnamePrecedenceOrder_Static_KnownOrder(t *testing.T) {
	// Groups from most specific to least specific. Hosts inside a group compare
	// equal (cmp==0); hosts in an earlier group outrank hosts in a later group.
	groups := [][]string{
		{"a.b.c.example.com"},                         // non-wildcard, 4 dots
		{"foo.bar.example.com"},                       // non-wildcard, 3 dots
		{"api.example.com"},                           // non-wildcard, 2 dots, 15 chars
		{"a.example.com", "b.example.com"},            // non-wildcard, 2 dots, 13 chars (equal class)
		{"example.com", "example.net", "example.org"}, // non-wildcard, 1 dot, 11 chars (equal class)
		{"example.io"},                                // non-wildcard, 1 dot, 10 chars
		{"*.sub.example.com"},                         // wildcard, 3 dots
		{"*.example.com"},                             // wildcard, 2 dots
		{"*.io"},                                      // wildcard, 1 dot
		{""},                                          // catch-all, least specific
	}

	// Within-group: every pair compares equal.
	for gi, g := range groups {
		for _, x := range g {
			for _, y := range g {
				if got := getHostnamePrecedenceOrder(x, y); got != 0 {
					t.Fatalf("group %d: expected %q ~ %q (cmp==0), got %d", gi, x, y, got)
				}
			}
		}
	}
	// Across groups: every member of an earlier group outranks every member of a
	// later group (cmp<0 in earlier-vs-later order, cmp>0 reversed).
	for gi := 0; gi < len(groups); gi++ {
		for gj := gi + 1; gj < len(groups); gj++ {
			for _, x := range groups[gi] {
				for _, y := range groups[gj] {
					if got := getHostnamePrecedenceOrder(x, y); got >= 0 {
						t.Fatalf("expected %q (group %d) to outrank %q (group %d): want <0, got %d", x, gi, y, gj, got)
					}
					if got := getHostnamePrecedenceOrder(y, x); got <= 0 {
						t.Fatalf("expected %q (group %d) to lose to %q (group %d): want >0, got %d", y, gj, x, gi, got)
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Unified comparator: static RulePrecedence corpora + SWO axioms + stability
// ---------------------------------------------------------------------------

// rp builds a RulePrecedence from an explicit common block and specificity
// factor. Every corpus element below is constructed this way so the full value
// of each scenario is visible in the source.
func rp(common CommonRulePrecedence, f RulePrecedenceFactor) RulePrecedence {
	factor := f // copy so &factor is unique per element
	return RulePrecedence{
		CommonRulePrecedence: common,
		PrecedenceFactor:     &factor,
	}
}

// The identity fields (name/rule/match/timestamp) below are chosen so that the
// unified comparator is a strict TOTAL order over the distinct corpus elements
// (its tiebreakers fully separate them), which makes the sorted output unique
// and therefore permutation-invariant. Deliberate exact-duplicate elements
// (identical in every field) are added where noted to exercise the
// equivalence-transitivity axiom; duplicates share an rpKey, so reordering them
// never changes the key sequence.

// buildHTTPCorpus returns HTTP-shaped rules (SecondaryLength always 0) spanning a
// deterministic product of explicit hostname / path-type / path-length / method /
// header / query-param value sets. It is generated by enumerating static value
// lists — not randomized — so coverage is visible and reproducible.
func buildHTTPCorpus() []RulePrecedence {
	hosts := []string{"", "example.com", "*.example.com", "api.example.com"}
	pathTypes := []int{0, 1, 2, 3}
	// Consecutive values (1,2,3) so a threshold-style ("within 1") non-transitive
	// comparison on the primary length is caught, not just widely-spaced values.
	pathLengths := []int{1, 2, 3}
	out := make([]RulePrecedence, 0, 128)
	idx := 0
	for _, host := range hosts {
		for _, pt := range pathTypes {
			for _, pl := range pathLengths {
				for _, hasMethod := range []bool{false, true} {
					for _, hdr := range []int{0, 2} {
						for _, qp := range []int{0, 1} {
							out = append(out, rp(
								CommonRulePrecedence{
									Hostname:             host,
									RouteNamespacedName:  fmt.Sprintf("ns/http-%d", idx),
									RouteCreateTimestamp: swoTimes[idx%len(swoTimes)],
									RuleIndexInRoute:     idx % 2,
									MatchIndexInRule:     idx,
								},
								RulePrecedenceFactor{
									PathType:        pt,
									PathLength:      pl,
									HasMethod:       hasMethod,
									HeaderCount:     hdr,
									QueryParamCount: qp,
								},
							))
							idx++
						}
					}
				}
			}
		}
	}
	return out
}

// buildGRPCCorpus returns GRPC-shaped rules: SecondaryLength (method length) is
// used, HasMethod is always false and QueryParamCount always 0, matching how
// getGrpcMatchPrecedenceInfo populates the factor.
func buildGRPCCorpus() []RulePrecedence {
	hosts := []string{"", "example.com", "*.example.com", "api.example.com"}
	pathTypes := []int{0, 1, 3} // GRPC has no prefix(2)
	// Consecutive values so threshold-style non-transitivity on either the
	// primary (service) or secondary (method) length is caught.
	serviceLengths := []int{4, 5, 6}
	methodLengths := []int{0, 3, 4}
	out := make([]RulePrecedence, 0, 128)
	idx := 0
	for _, host := range hosts {
		for _, pt := range pathTypes {
			for _, sl := range serviceLengths {
				for _, ml := range methodLengths {
					for _, hdr := range []int{0, 2} {
						out = append(out, rp(
							CommonRulePrecedence{
								Hostname:             host,
								RouteNamespacedName:  fmt.Sprintf("ns/grpc-%d", idx),
								RouteCreateTimestamp: swoTimes[idx%len(swoTimes)],
								RuleIndexInRoute:     idx % 2,
								MatchIndexInRule:     idx,
							},
							RulePrecedenceFactor{
								PathType:        pt,
								PathLength:      sl,
								SecondaryLength: ml,
								HeaderCount:     hdr,
							},
						))
						idx++
					}
				}
			}
		}
	}
	return out
}

// swoAdversarialTriple is the exact cross-kind scenario that cycled under the
// old (pre-unified) comparator. All three share a hostname and path type so the
// ordering falls entirely to the length keys:
//
//	a = GRPC{service=5, method=0}
//	b = GRPC{service=0, method=7}
//	c = HTTP{pathLength=7}
func swoAdversarialTriple() []RulePrecedence {
	const host = "shared.example.com"
	const pt = 3 // same path type for all three
	return []RulePrecedence{
		rp(CommonRulePrecedence{Hostname: host, RouteNamespacedName: "ns/adv-a", RouteCreateTimestamp: swoTimes[0], MatchIndexInRule: 1001},
			RulePrecedenceFactor{PathType: pt, PathLength: 5, SecondaryLength: 0}), // GRPC service=5
		rp(CommonRulePrecedence{Hostname: host, RouteNamespacedName: "ns/adv-b", RouteCreateTimestamp: swoTimes[0], MatchIndexInRule: 1002},
			RulePrecedenceFactor{PathType: pt, PathLength: 0, SecondaryLength: 7}), // GRPC method=7
		rp(CommonRulePrecedence{Hostname: host, RouteNamespacedName: "ns/adv-c", RouteCreateTimestamp: swoTimes[0], MatchIndexInRule: 1003},
			RulePrecedenceFactor{PathType: pt, PathLength: 7}), // HTTP pathLength=7
	}
}

// swoEquivalenceGroup returns three elements identical in EVERY compared field,
// so the unified comparator ranks them equal (eq==true) — a mutual-equivalence
// class of size 3 that makes the equivalence-transitivity axiom non-trivial.
// They share an rpKey, so they are interchangeable in any sorted output.
func swoEquivalenceGroup() []RulePrecedence {
	mk := func() RulePrecedence {
		return rp(
			CommonRulePrecedence{Hostname: "example.com", RouteNamespacedName: "ns/dup", RouteCreateTimestamp: swoTimes[0], RuleIndexInRoute: 0, MatchIndexInRule: 2000},
			RulePrecedenceFactor{PathType: 3, PathLength: 4, HeaderCount: 1},
		)
	}
	return []RulePrecedence{mk(), mk(), mk()}
}

// buildMixedCorpus is the full cross-kind corpus: the HTTP and GRPC corpora plus
// the adversarial triple and the equivalence group. This is the input the
// unified comparator must order as a strict weak ordering.
func buildMixedCorpus() []RulePrecedence {
	out := make([]RulePrecedence, 0, 300)
	out = append(out, buildHTTPCorpus()...)
	out = append(out, buildGRPCCorpus()...)
	out = append(out, swoAdversarialTriple()...)
	out = append(out, swoEquivalenceGroup()...)
	return out
}

func Test_compareRulePrecedenceUnified_SWO_Static_HTTPOnly(t *testing.T) {
	corpus := buildHTTPCorpus()
	assertBoolComparatorIsSWO(t, corpus, compareRulePrecedenceUnified, func(i int) string {
		return fmt.Sprintf("idx %d %s", i, describeRule(corpus[i]))
	})
	assertSortOrderIsPermutationInvariant(t, corpus, compareRulePrecedenceUnified, "http-only corpus")
}

func Test_compareRulePrecedenceUnified_SWO_Static_GRPCOnly(t *testing.T) {
	corpus := buildGRPCCorpus()
	assertBoolComparatorIsSWO(t, corpus, compareRulePrecedenceUnified, func(i int) string {
		return fmt.Sprintf("idx %d %s", i, describeRule(corpus[i]))
	})
	assertSortOrderIsPermutationInvariant(t, corpus, compareRulePrecedenceUnified, "grpc-only corpus")
}

// Test_compareRulePrecedenceUnified_SWO_Static_Mixed is the regression gate for
// the cross-kind ordering bug. It checks the SWO axioms exhaustively over the
// mixed corpus (which contains the adversarial triple and the equivalence group)
// and asserts the sorted output is invariant across ~1000 deterministic input
// permutations.
func Test_compareRulePrecedenceUnified_SWO_Static_Mixed(t *testing.T) {
	corpus := buildMixedCorpus()
	assertBoolComparatorIsSWO(t, corpus, compareRulePrecedenceUnified, func(i int) string {
		return fmt.Sprintf("idx %d %s", i, describeRule(corpus[i]))
	})
	assertSortOrderIsPermutationInvariant(t, corpus, compareRulePrecedenceUnified, "mixed corpus")
}

// describeRule renders the salient factors of a RulePrecedence for failure output.
func describeRule(r RulePrecedence) string {
	c := r.CommonRulePrecedence
	f := r.PrecedenceFactor
	return fmt.Sprintf("{host=%q pt=%d pl=%d sec=%d hm=%t hdr=%d qp=%d name=%s ri=%d mi=%d ts=%s}",
		c.Hostname, f.PathType, f.PathLength, f.SecondaryLength, f.HasMethod, f.HeaderCount, f.QueryParamCount,
		c.RouteNamespacedName, c.RuleIndexInRoute, c.MatchIndexInRule, c.RouteCreateTimestamp.Format("2006"))
}

// rpKey uniquely and stably identifies a flattened rule by the comparator's
// tiebreaker fields, including the single hostname it was split to. Two elements
// share a key only when they tie on all of these fields — i.e. only when they
// are equivalent under the comparator — so a key sequence is a faithful witness
// of sort order for permutation-invariance checks.
func rpKey(r RulePrecedence) string {
	c := r.CommonRulePrecedence
	return fmt.Sprintf("%s#r%d#m%d#h%q", c.RouteNamespacedName, c.RuleIndexInRoute, c.MatchIndexInRule, c.Hostname)
}

func rpKeys(rs []RulePrecedence) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = rpKey(r)
	}
	return out
}

func keysOf(rs []RulePrecedence) []string { return rpKeys(rs) }

func equalKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// 3. End-to-end SortAllRulesByPrecedence: static route sets + stability
// ---------------------------------------------------------------------------

func cloneRoutes(in []RouteDescriptor) []RouteDescriptor {
	return append([]RouteDescriptor(nil), in...)
}

func swoHosts(hosts ...string) []gwv1.Hostname {
	out := make([]gwv1.Hostname, len(hosts))
	for i, h := range hosts {
		out[i] = gwv1.Hostname(h)
	}
	return out
}

func swoHTTPPath(matchType, value string) gwv1.HTTPRouteMatch {
	return gwv1.HTTPRouteMatch{
		Path: &gwv1.HTTPPathMatch{Type: (*gwv1.PathMatchType)(awssdk.String(matchType)), Value: awssdk.String(value)},
	}
}

func swoGRPCExact(service, method string) gwv1.GRPCRouteMatch {
	mm := &gwv1.GRPCMethodMatch{Type: (*gwv1.GRPCMethodMatchType)(awssdk.String("Exact"))}
	if service != "" {
		mm.Service = awssdk.String(service)
	}
	if method != "" {
		mm.Method = awssdk.String(method)
	}
	return gwv1.GRPCRouteMatch{Method: mm}
}

func swoHTTPRoute(name, ns string, ts time.Time, hosts []gwv1.Hostname, matches ...gwv1.HTTPRouteMatch) *httpRouteDescription {
	return &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, CreationTimestamp: metav1.Time{Time: ts}},
			Spec:       gwv1.HTTPRouteSpec{Hostnames: hosts},
		},
		rules: []RouteRule{&convertedHTTPRouteRule{rule: &gwv1.HTTPRouteRule{Matches: matches}}},
	}
}

func swoGRPCRoute(name, ns string, ts time.Time, hosts []gwv1.Hostname, matches ...gwv1.GRPCRouteMatch) *grpcRouteDescription {
	return &grpcRouteDescription{
		route: &gwv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, CreationTimestamp: metav1.Time{Time: ts}},
			Spec:       gwv1.GRPCRouteSpec{Hostnames: hosts},
		},
		rules: []RouteRule{&convertedGRPCRouteRule{rule: &gwv1.GRPCRouteRule{Matches: matches}}},
	}
}

// swoRouteScenarios is a set of realistic route sets. Each is a
// self-contained input to SortAllRulesByPrecedence chosen to stress a specific
// ordering interaction end-to-end:
//   - mixed HTTP + GRPC on a shared hostname (cross-kind ordering)
//   - multi-hostname routes (per-hostname split, equal-specificity host tiebreak)
//   - catch-all ("") vs host-specific and wildcard vs exact
//   - the adversarial cross-kind shapes, materialized as real routes
func swoRouteScenarios() map[string][]RouteDescriptor {
	t0 := swoTimes[0]
	t1 := swoTimes[1]

	return map[string][]RouteDescriptor{
		"mixed http+grpc shared hostname": {
			swoHTTPRoute("http-root", "team-a", t1, swoHosts("shared.example.com"), swoHTTPPath("PathPrefix", "/")),
			swoHTTPRoute("http-status", "team-a", t0, swoHosts("shared.example.com"), swoHTTPPath("Exact", "/status")),
			swoGRPCRoute("grpc-echo", "team-b", t0, swoHosts("shared.example.com"), swoGRPCExact("my.EchoService", "Echo")),
			swoGRPCRoute("grpc-catchall", "team-b", t1, swoHosts("shared.example.com")),
		},
		"multi-hostname split with wildcard and catch-all": {
			swoHTTPRoute("http-multi", "ns", t0, swoHosts("*.example.com", "api.example.com", "web.example.com"),
				swoHTTPPath("Exact", "/health"), swoHTTPPath("PathPrefix", "/")),
			swoHTTPRoute("http-nohost", "ns", t1, nil, swoHTTPPath("PathPrefix", "/")),
			swoGRPCRoute("grpc-users", "ns", t0, swoHosts("api.example.com"), swoGRPCExact("com.example.UserService", "GetUser")),
		},
		"cross-kind adversarial shapes as routes": {
			// Same hostname + exact type; distinguished only by length keys — the
			// end-to-end analogue of swoAdversarialTriple.
			swoGRPCRoute("grpc-svc5", "ns", t0, swoHosts("shared.example.com"), swoGRPCExact("abcde", "")),        // service len 5
			swoGRPCRoute("grpc-meth7", "ns", t0, swoHosts("shared.example.com"), swoGRPCExact("", "abcdefg")),     // method len 7
			swoHTTPRoute("http-path7", "ns", t0, swoHosts("shared.example.com"), swoHTTPPath("Exact", "/abcdef")), // path len 7
		},
		"specificity ladder single hostname": {
			swoHTTPRoute("http-catchall", "ns", t1, swoHosts("app.example.com"), swoHTTPPath("PathPrefix", "/")),
			swoHTTPRoute("http-api", "ns", t0, swoHosts("app.example.com"), swoHTTPPath("PathPrefix", "/api")),
			swoHTTPRoute("http-users", "ns", t0, swoHosts("app.example.com"), swoHTTPPath("Exact", "/api/users")),
		},
	}
}

// assertRouteSortPermutationInvariant sorts a static route set under identity,
// reverse, every rotation, and permStabilityShuffles fixed-seed shuffles of the
// INPUT ROUTE ORDER, asserting the flattened rule ordering is byte-identical
// every time. This reproduces the production symptom directly: if the comparator
// is not a strict weak ordering, sort.Slice yields an order that depends on the
// input permutation.
func assertRouteSortPermutationInvariant(t *testing.T, routes []RouteDescriptor, port int32, label string) {
	t.Helper()

	baseline := keysOf(SortAllRulesByPrecedence(cloneRoutes(routes), port))

	check := func(perm []RouteDescriptor, desc string) {
		got := keysOf(SortAllRulesByPrecedence(perm, port))
		if !equalKeys(baseline, got) {
			t.Fatalf("%s: sort order depends on input order (comparator not a strict weak ordering)\n permutation=%s\n baseline=%v\n got     =%v",
				label, desc, baseline, got)
		}
	}

	n := len(routes)

	rev := make([]RouteDescriptor, n)
	for i := range routes {
		rev[n-1-i] = routes[i]
	}
	check(rev, "reverse")

	for r := 1; r < n; r++ {
		rot := make([]RouteDescriptor, 0, n)
		rot = append(rot, routes[r:]...)
		rot = append(rot, routes[:r]...)
		check(rot, fmt.Sprintf("rotate-%d", r))
	}

	rng := rand.New(rand.NewSource(permStabilitySeed))
	work := cloneRoutes(routes)
	for s := 0; s < permStabilityShuffles; s++ {
		rng.Shuffle(len(work), func(i, j int) { work[i], work[j] = work[j], work[i] })
		check(cloneRoutes(work), fmt.Sprintf("shuffle-%d", s))
	}
}

// Test_SortAllRulesByPrecedence_PermutationInvariant_Static runs each static
// route scenario through ~1000 deterministic input permutations and asserts the
// resulting ALB rule ordering never changes. This is the end-to-end,
// static-data replacement for the previous randomly-generated shuffle test.
func Test_SortAllRulesByPrecedence_PermutationInvariant_Static(t *testing.T) {
	for name, routes := range swoRouteScenarios() {
		t.Run(name, func(t *testing.T) {
			assertRouteSortPermutationInvariant(t, routes, 0, name)
		})
	}
}

// swoHostnameSeq extracts the hostname of each rule in sorted order, i.e. the ALB
// priority order projected onto its hostnames.
func swoHostnameSeq(rs []RulePrecedence) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.CommonRulePrecedence.Hostname
	}
	return out
}

// swoPermuteStrings returns every permutation of in (Heap's algorithm). Used to
// exhaustively vary the order equal-priority inputs are presented in.
func swoPermuteStrings(in []string) [][]string {
	var out [][]string
	a := append([]string(nil), in...)
	var gen func(k int)
	gen = func(k int) {
		if k == 1 {
			out = append(out, append([]string(nil), a...))
			return
		}
		for i := 0; i < k; i++ {
			gen(k - 1)
			if k%2 == 0 {
				a[i], a[k-1] = a[k-1], a[i]
			} else {
				a[0], a[k-1] = a[k-1], a[0]
			}
		}
	}
	gen(len(a))
	return out
}

// Test_SortAllRulesByPrecedence_StableOrder_EqualPriorityHostnames covers the
// anti-flapping contract: a set of rules with the SAME relative priority — path
// "/" on the equally-specific hostnames foo.com / bar.com / baz.com — must be
// assigned a STABLE, deterministic ALB priority order no matter what order the
// hostnames or routes are presented in. If equal-priority rules reordered across
// reconciles, the controller would rewrite ALB rule priorities for no functional
// reason (pure churn). The comparator's final deterministic tiebreaker (ascending
// hostname string) is what prevents that, and this test pins it.
func Test_SortAllRulesByPrecedence_StableOrder_EqualPriorityHostnames(t *testing.T) {
	created := swoTimes[0]
	hostnames := []string{"foo.com", "bar.com", "baz.com"}

	// Premise: all three hostnames are equally specific (each is non-wildcard with
	// 1 dot and 7 characters), so hostname precedence ties and, with an identical
	// path, the rules genuinely share the same relative priority.
	for _, a := range hostnames {
		for _, b := range hostnames {
			assert.Equalf(t, 0, getHostnamePrecedenceOrder(a, b),
				"premise: %q and %q must be equally specific (same relative priority)", a, b)
		}
	}

	// The one stable order the sort must always converge to: ascending hostname,
	// the comparator's final tiebreaker once every specificity factor ties.
	wantOrder := []string{"bar.com", "baz.com", "foo.com"}

	// Case 1: ONE route declaring the three hostnames. Each split unit is identical
	// in every field EXCEPT the hostname, so the equal-priority hostname tiebreaker
	// is the sole determinant of order. Vary the declaration order every possible
	// way (all permutations) and then across many fixed-seed shuffles; the ALB
	// order must never change.
	t.Run("single route, hostnames declared in any order", func(t *testing.T) {
		build := func(order []string) []RouteDescriptor {
			return []RouteDescriptor{swoHTTPRoute("route", "ns", created, swoHosts(order...), swoHTTPPath("PathPrefix", "/"))}
		}

		assert.Equal(t, wantOrder, swoHostnameSeq(SortAllRulesByPrecedence(build(hostnames), 0)),
			"stable order must be ascending hostname")

		for _, p := range swoPermuteStrings(hostnames) {
			got := swoHostnameSeq(SortAllRulesByPrecedence(build(p), 0))
			assert.Equalf(t, wantOrder, got, "declaration order %v must not change ALB priority order", p)
		}

		rng := rand.New(rand.NewSource(permStabilitySeed))
		order := append([]string(nil), hostnames...)
		for s := 0; s < permStabilityShuffles; s++ {
			rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
			got := swoHostnameSeq(SortAllRulesByPrecedence(build(order), 0))
			assert.Equalf(t, wantOrder, got, "shuffle %d changed ALB priority order", s)
		}
	})

	// Case 2: THREE separate single-hostname routes. They share the same route
	// name/namespace, creation timestamp, rule and match index, so — exactly as in
	// Case 1 — the only field that differs is the hostname. Feeding the routes in
	// any order must still yield the same stable ALB order.
	t.Run("three separate routes, fed in any order", func(t *testing.T) {
		mk := func(host string) RouteDescriptor {
			return swoHTTPRoute("route", "ns", created, swoHosts(host), swoHTTPPath("PathPrefix", "/"))
		}
		routes := []RouteDescriptor{mk("foo.com"), mk("bar.com"), mk("baz.com")}

		assert.Equal(t, wantOrder, swoHostnameSeq(SortAllRulesByPrecedence(cloneRoutes(routes), 0)),
			"stable order must be ascending hostname")

		// Permutation-invariance across all input orderings (reverse, rotations,
		// and ~1000 fixed-seed shuffles).
		assertRouteSortPermutationInvariant(t, routes, 0, "equal-priority separate routes")
	})
}

// ---------------------------------------------------------------------------
// 4. Golden ordering fixtures
// ---------------------------------------------------------------------------

// Test_SortAllRulesByPrecedence_Golden_MatchingAcrossRoutes encodes the exact
// gateway-api conformance scenario httproute-matching-across-routes as a fast
// unit test. part1 has TWO hostnames (example.com, example.net) and part2 has
// ONE (example.com); the merged-PR bug let part1's larger hostname list outrank
// part2, pushing part2's specific "/v2" path below part1's catch-all "/". The
// correct Gateway API order compares hostname specificity first (a tie here) and
// then PATH, so the "/v2" rule must be the highest-precedence rule.
func Test_SortAllRulesByPrecedence_Golden_MatchingAcrossRoutes(t *testing.T) {
	// part1 created earlier than part2 so ties break deterministically toward part1.
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	pathPrefix := (*gwv1.PathMatchType)(awssdk.String("PathPrefix"))

	part1 := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "matching-part1",
				Namespace:         "gateway-conformance-infra",
				CreationTimestamp: metav1.Time{Time: t1},
			},
			Spec: gwv1.HTTPRouteSpec{
				Hostnames: []gwv1.Hostname{"example.com", "example.net"},
			},
		},
		rules: []RouteRule{
			&convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Matches: []gwv1.HTTPRouteMatch{
						// m0: path "/" (catch-all)
						{Path: &gwv1.HTTPPathMatch{Type: pathPrefix, Value: awssdk.String("/")}},
						// m1: path "/" (defaulted) + header version=one
						{
							Path:    &gwv1.HTTPPathMatch{Type: pathPrefix, Value: awssdk.String("/")},
							Headers: []gwv1.HTTPHeaderMatch{{Name: "version", Value: "one"}},
						},
					},
				},
			},
		},
	}

	part2 := &httpRouteDescription{
		route: &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "matching-part2",
				Namespace:         "gateway-conformance-infra",
				CreationTimestamp: metav1.Time{Time: t2},
			},
			Spec: gwv1.HTTPRouteSpec{
				Hostnames: []gwv1.Hostname{"example.com"},
			},
		},
		rules: []RouteRule{
			&convertedHTTPRouteRule{
				rule: &gwv1.HTTPRouteRule{
					Matches: []gwv1.HTTPRouteMatch{
						// m0: path "/v2" (specific)
						{Path: &gwv1.HTTPPathMatch{Type: pathPrefix, Value: awssdk.String("/v2")}},
						// m1: path "/" (defaulted) + header version=two
						{
							Path:    &gwv1.HTTPPathMatch{Type: pathPrefix, Value: awssdk.String("/")},
							Headers: []gwv1.HTTPHeaderMatch{{Name: "version", Value: "two"}},
						},
					},
				},
			},
		},
	}

	// Feed in shuffled order to also exercise sort robustness.
	input := []RouteDescriptor{part2, part1}
	result := SortAllRulesByPrecedence(input, 0)

	// Per-hostname split: each (route, match, hostname) is its own unit. part1 has
	// 2 hostnames × 2 matches = 4 units; part2 has 1 hostname × 2 matches = 2 → 6.
	assert.Equal(t, 6, len(result), "per-hostname split: part1(2 hosts×2 matches)+part2(1 host×2 matches)=6 rules")

	type want struct {
		name     string
		matchIdx int
		hostname string
		pathLen  int
		headers  int
	}
	// example.com and example.net are equally specific, so hostname precedence is a
	// tie; ordering falls to path length, then header count, then timestamp
	// (part1 older than part2), then the hostname-string tiebreaker.
	expected := []want{
		{"gateway-conformance-infra/matching-part2", 0, "example.com", 3, 0}, // "/v2" -> most specific path
		{"gateway-conformance-infra/matching-part1", 1, "example.com", 1, 1}, // "/" + version=one
		{"gateway-conformance-infra/matching-part1", 1, "example.net", 1, 1}, // "/" + version=one (other host)
		{"gateway-conformance-infra/matching-part2", 1, "example.com", 1, 1}, // "/" + version=two (later route)
		{"gateway-conformance-infra/matching-part1", 0, "example.com", 1, 0}, // "/" catch-all
		{"gateway-conformance-infra/matching-part1", 0, "example.net", 1, 0}, // "/" catch-all (other host)
	}
	for i, w := range expected {
		got := result[i]
		assert.Equalf(t, w.name, got.CommonRulePrecedence.RouteNamespacedName, "priority %d route", i+1)
		assert.Equalf(t, w.matchIdx, got.CommonRulePrecedence.MatchIndexInRule, "priority %d matchIndex", i+1)
		assert.Equalf(t, w.hostname, got.CommonRulePrecedence.Hostname, "priority %d hostname", i+1)
		assert.Equalf(t, w.pathLen, got.PrecedenceFactor.PathLength, "priority %d pathLen", i+1)
		assert.Equalf(t, w.headers, got.PrecedenceFactor.HeaderCount, "priority %d headerCount", i+1)
	}

	// The single most important invariant vs the reported bug: the specific
	// "/v2" rule must outrank the catch-all rules, i.e. be at priority 1.
	assert.Equal(t, "gateway-conformance-infra/matching-part2", result[0].CommonRulePrecedence.RouteNamespacedName,
		"specific /v2 path must be highest precedence, not a catch-all")
	assert.Equal(t, 3, result[0].PrecedenceFactor.PathLength,
		"priority-1 rule must be the /v2 path (len 3)")
}
