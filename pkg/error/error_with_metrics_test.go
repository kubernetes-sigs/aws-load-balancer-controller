package ctrlerrors

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/lbc"
	"testing"
	"time"
)

func Test_NewErrorWithMetrics(t *testing.T) {
	resourceType := "test-resource"
	category := "test-category"
	// The error that NamespaceLifecycle admission returns for a create in a terminating namespace.
	nsTerminatingErr := apierrors.NewForbidden(schema.GroupResource{Group: "elbv2.k8s.aws", Resource: "targetgroupbindings"}, "tgb",
		errors.New("unable to create new content in namespace ns because it is being terminated"))
	nsTerminatingErr.ErrStatus.Details.Causes = []metav1.StatusCause{{Type: corev1.NamespaceTerminatingCause}}
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
		{
			name: "create rejected in a terminating namespace",
			err:  errors.Wrap(nsTerminatingErr, "failed to create targetGroupBinding"),
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

func Test_ErrorWithMetrics_Unwrap(t *testing.T) {
	requeueErr := NewRequeueNeededAfter("waiting", time.Second)
	wrapped := &ErrorWithMetrics{Err: requeueErr}

	var target *RequeueNeededAfter
	assert.True(t, errors.As(wrapped, &target))
	assert.Equal(t, requeueErr, target)
}
