package ctrlerrors

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/metrics/lbc"
	"testing"
)

func Test_NewErrorWithMetrics(t *testing.T) {
	resourceType := "test-resource"
	category := "test-category"
	testCases := []struct {
		name                string
		err                 error
		expectedInvocations int
	}{
		{
			name:                "real error",
			err:                 errors.New("bad thing"),
			expectedInvocations: 1,
		},
		{
			name: "requeue needed error",
			err:  &RequeueNeeded{},
		},
		{
			name: "requeue needed after error",
			err:  &RequeueNeededAfter{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collector := lbcmetrics.NewMockCollector()
			res := NewErrorWithMetrics(resourceType, category, tc.err, collector)
			assert.Equal(t, tc.err, res.Err)
			assert.Equal(t, resourceType, res.ResourceType)
			assert.Equal(t, category, res.ErrorCategory)
			mc := collector.(*lbcmetrics.MockCollector)
			assert.Equal(t, tc.expectedInvocations, len(mc.Invocations[lbcmetrics.MetricControllerReconcileErrors]))
		})
	}
}
