package referencecounter

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

func TestServiceReferenceCounter(t *testing.T) {

	type insertion struct {
		svcs     []types.NamespacedName
		gateway  types.NamespacedName
		isDelete bool
	}

	type deletion struct {
		svcName          types.NamespacedName
		expectedGateways []types.NamespacedName
		expected         bool
	}

	testCases := []struct {
		name       string
		insertions []insertion
		deletions  []deletion
	}{
		{
			name: "no insertions",
			deletions: []deletion{
				{
					svcName: types.NamespacedName{
						Namespace: "test-ns",
						Name:      "test-svc",
					},
					expectedGateways: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-gw",
						},
					},
					expected: false,
				},
				{
					svcName: types.NamespacedName{
						Namespace: "test-ns1",
						Name:      "test-svc1",
					},
					expectedGateways: []types.NamespacedName{},
					expected:         true,
				},
				{
					svcName: types.NamespacedName{
						Namespace: "test-ns2",
						Name:      "test-svc2",
					},
					expectedGateways: []types.NamespacedName{},
					expected:         true,
				},
			},
		},
		{
			name: "some valid deletions",
			insertions: []insertion{
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
						{
							Namespace: "test-ns",
							Name:      "test-svc2",
						},
						{
							Namespace: "test-ns",
							Name:      "test-svc3",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw1",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
						{
							Namespace: "test-ns",
							Name:      "test-svc2",
						},
						{
							Namespace: "test-ns",
							Name:      "test-svc3",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw2",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
						{
							Namespace: "test-ns",
							Name:      "test-svc2",
						},
						{
							Namespace: "test-ns",
							Name:      "test-svc3",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw3",
						Namespace: "ns2",
					},
				},
			},
			deletions: []deletion{
				// Wrong number of expected.
				{
					svcName: types.NamespacedName{
						Namespace: "test-ns",
						Name:      "test-svc",
					},
					expectedGateways: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-gw",
						},
					},
					expected: false,
				},
				// Still has references
				{
					svcName: types.NamespacedName{
						Namespace: "test-ns",
						Name:      "test-svc1",
					},
					expectedGateways: []types.NamespacedName{
						{
							Name:      "gw3",
							Namespace: "ns2",
						},
						{
							Name:      "gw2",
							Namespace: "ns2",
						},
						{
							Name:      "gw1",
							Namespace: "ns2",
						},
					},
					expected: false,
				},
				// No references, valid expected gateways
				{
					svcName: types.NamespacedName{
						Namespace: "def doesnt exist",
						Name:      "test-svc1",
					},
					expectedGateways: []types.NamespacedName{
						{
							Name:      "gw3",
							Namespace: "ns2",
						},
						{
							Name:      "gw2",
							Namespace: "ns2",
						},
						{
							Name:      "gw1",
							Namespace: "ns2",
						},
					},
					expected: true,
				},
			},
		},
		{
			name: "try to delete with references around",
			insertions: []insertion{
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw1",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw2",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw3",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw4",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw2",
						Namespace: "ns2",
					},
					isDelete: true,
				},
			},
			deletions: []deletion{
				// Wrong number of expected.
				{
					svcName: types.NamespacedName{
						Namespace: "test-ns",
						Name:      "test-svc1",
					},
					expectedGateways: []types.NamespacedName{
						{
							Name:      "gw1",
							Namespace: "ns2",
						},
						{
							Name:      "gw3",
							Namespace: "ns2",
						},
						{
							Name:      "gw4",
							Namespace: "ns2",
						},
					},
					expected: false,
				},
			},
		},
		{
			name: "all references cleared",
			insertions: []insertion{
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw1",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw2",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw3",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw4",
						Namespace: "ns2",
					},
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw2",
						Namespace: "ns2",
					},
					isDelete: true,
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw1",
						Namespace: "ns2",
					},
					isDelete: true,
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw3",
						Namespace: "ns2",
					},
					isDelete: true,
				},
				{
					svcs: []types.NamespacedName{
						{
							Namespace: "test-ns",
							Name:      "test-svc1",
						},
					},
					gateway: types.NamespacedName{
						Name:      "gw4",
						Namespace: "ns2",
					},
					isDelete: true,
				},
			},
			deletions: []deletion{
				// Wrong number of expected.
				{
					svcName: types.NamespacedName{
						Namespace: "test-ns",
						Name:      "test-svc1",
					},
					expectedGateways: []types.NamespacedName{},
					expected:         true,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			refCounter := NewServiceReferenceCounter()

			for _, ii := range tc.insertions {
				refCounter.UpdateRelations(ii.svcs, ii.gateway, ii.isDelete)
			}

			for _, del := range tc.deletions {
				res := refCounter.IsEligibleForRemoval(del.svcName, del.expectedGateways)
				assert.Equal(t, del.expected, res, "tc %v", del)
			}
		})
	}

	t.Run("create -> check -> fail -> delete -> check -> work", func(t *testing.T) {
		refCounter := NewServiceReferenceCounter()
		refCounter.UpdateRelations([]types.NamespacedName{
			{Name: "svc1", Namespace: "ns"},
		}, types.NamespacedName{Name: "gw1", Namespace: "ns"}, false)
		assert.False(t, refCounter.IsEligibleForRemoval(types.NamespacedName{Name: "svc1", Namespace: "ns"}, []types.NamespacedName{
			{Name: "gw1", Namespace: "ns"},
		}))
		refCounter.UpdateRelations([]types.NamespacedName{
			{Name: "svc1", Namespace: "ns"},
		}, types.NamespacedName{Name: "gw1", Namespace: "ns"}, true)
		assert.True(t, refCounter.IsEligibleForRemoval(types.NamespacedName{Name: "svc1", Namespace: "ns"}, []types.NamespacedName{}))
	})

}
