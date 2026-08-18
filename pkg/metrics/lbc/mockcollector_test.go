package lbc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMockCollector_ReconcileCondition(t *testing.T) {
	c := NewMockCollector()
	mc := c.(*MockCollector)

	// the condition series starts empty
	assert.Empty(t, mc.Invocations[MetricControllerReconcileCondition])

	c.ObserveControllerReconcileCondition("ingress", "ns-1", "ing-1", true)
	c.ObserveControllerReconcileCondition("service", "ns-2", "svc-1", false)
	c.DeleteControllerReconcileCondition("service", "ns-2", "svc-1")

	recordings := mc.Invocations[MetricControllerReconcileCondition]
	assert.Len(t, recordings, 3)

	assert.Equal(t, MockGaugeMetric{
		labelController: "ingress",
		labelNamespace:  "ns-1",
		labelName:       "ing-1",
		value:           true,
	}, recordings[0])

	assert.Equal(t, MockGaugeMetric{
		labelController: "service",
		labelNamespace:  "ns-2",
		labelName:       "svc-1",
		value:           false,
	}, recordings[1])

	assert.Equal(t, MockGaugeMetric{
		labelController: "service",
		labelNamespace:  "ns-2",
		labelName:       "svc-1",
		deleted:         true,
	}, recordings[2])
}
