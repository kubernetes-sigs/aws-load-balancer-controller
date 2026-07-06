package lbc

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func newTestCollector() *collector {
	registry := prometheus.NewPedanticRegistry()
	return NewCollector(registry, nil, nil, logr.Discard()).(*collector)
}

func TestObserveControllerReconcileCondition(t *testing.T) {
	c := newTestCollector()
	gauge := c.instruments.controllerReconcileCondition

	c.ObserveControllerReconcileCondition("ingress", "ns-1", "ing-1", true)
	c.ObserveControllerReconcileCondition("ingress", "ns-1", "ing-2", false)

	assert.Equal(t, float64(1), testutil.ToFloat64(gauge.WithLabelValues("ingress", "ns-1", "ing-1")))
	assert.Equal(t, float64(0), testutil.ToFloat64(gauge.WithLabelValues("ingress", "ns-1", "ing-2")))
	assert.Equal(t, 2, testutil.CollectAndCount(gauge))

	// a state flip updates the existing series in place
	c.ObserveControllerReconcileCondition("ingress", "ns-1", "ing-2", true)
	assert.Equal(t, float64(1), testutil.ToFloat64(gauge.WithLabelValues("ingress", "ns-1", "ing-2")))
	assert.Equal(t, 2, testutil.CollectAndCount(gauge))
}

func TestObserveControllerReconcileCondition_metricNameAndLabels(t *testing.T) {
	c := newTestCollector()
	c.ObserveControllerReconcileCondition("service", "ns-1", "svc-1", true)

	expected := `
# HELP awslbc_controller_reconcile_condition Whether the last reconcile of the resource succeeded (1) or failed (0). The series is removed when the resource is deleted.
# TYPE awslbc_controller_reconcile_condition gauge
awslbc_controller_reconcile_condition{controller="service",name="svc-1",namespace="ns-1"} 1
`
	assert.NoError(t, testutil.CollectAndCompare(c.instruments.controllerReconcileCondition, strings.NewReader(expected)))
}

func TestDeleteControllerReconcileCondition(t *testing.T) {
	c := newTestCollector()
	gauge := c.instruments.controllerReconcileCondition

	c.ObserveControllerReconcileCondition("service", "ns-1", "svc-1", true)
	c.ObserveControllerReconcileCondition("service", "ns-1", "svc-2", false)
	c.ObserveControllerReconcileCondition("targetGroupBinding", "ns-1", "svc-1", true)

	c.DeleteControllerReconcileCondition("service", "ns-1", "svc-1")
	assert.Equal(t, 2, testutil.CollectAndCount(gauge))
	assert.Equal(t, float64(0), testutil.ToFloat64(gauge.WithLabelValues("service", "ns-1", "svc-2")))
	assert.Equal(t, float64(1), testutil.ToFloat64(gauge.WithLabelValues("targetGroupBinding", "ns-1", "svc-1")))

	// deleting a series that does not exist is a no-op
	c.DeleteControllerReconcileCondition("service", "ns-1", "does-not-exist")
	assert.Equal(t, 2, testutil.CollectAndCount(gauge))
}
