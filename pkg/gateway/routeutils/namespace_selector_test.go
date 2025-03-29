package routeutils

import (
	"context"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"testing"
)

func Test_getNamespacesFromSelector(t *testing.T) {

	testSelector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "foo",
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	}

	testCases := []struct {
		name               string
		namespacesToAdd    []*v1.Namespace
		expectedNamespaces sets.Set[string]
		expectErr          bool
	}{
		{
			name:               "no namespaces",
			expectedNamespaces: make(sets.Set[string]),
		},
		{
			name: "one namespace",
			namespacesToAdd: []*v1.Namespace{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "my-ns-1",
						Labels: map[string]string{},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ns-2",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			expectedNamespaces: sets.Set[string]{"my-ns-1": sets.Empty{}},
		},
		{
			name: "multiple namespaces",
			namespacesToAdd: []*v1.Namespace{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "my-ns-1",
						Labels: map[string]string{},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "my-ns-2",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "my-ns-3",
						Labels: map[string]string{},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "my-ns-4",
						Labels: map[string]string{},
					},
				},
			},
			expectedNamespaces: sets.Set[string]{"my-ns-1": sets.Empty{}, "my-ns-3": sets.Empty{}, "my-ns-4": sets.Empty{}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := generateTestClient()
			nsSelector := namespaceSelectorImpl{
				k8sClient: k8sClient,
			}

			for _, ns := range tc.namespacesToAdd {
				err := k8sClient.Create(context.Background(), ns)
				assert.NoError(t, err)
			}

			result, err := nsSelector.getNamespacesFromSelector(context.Background(), testSelector)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.Equal(t, tc.expectedNamespaces, result)
			assert.NoError(t, err)
		})
	}

}
