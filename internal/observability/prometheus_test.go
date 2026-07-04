package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusMetrics_Labels(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrometheusMetrics(reg)

	m.RecordRequestOutcome("req-123", "openai-1", "gpt-4", "priority", 200, "", "hit", 150.5)

	// We expect the metric to be registered and correctly labeled.
	// Since testutil.GatherAndCount is easy to use for full counts.
	metricsCount, err := testutil.GatherAndCount(reg, "veloxmesh_request_outcome_total")
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if metricsCount != 1 {
		t.Errorf("Expected 1 request outcome metric, got %d", metricsCount)
	}

	// Verify that the "reqID" was not stored as a label (which would blow up cardinality).
	// Gather all metric families to ensure no forbidden labels.
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	forbiddenLabels := []string{"reqID", "requestID", "user", "api_key", "prompt"}

	for _, mf := range mfs {
		for _, m := range mf.Metric {
			for _, lp := range m.Label {
				labelName := lp.GetName()
				for _, forbidden := range forbiddenLabels {
					if labelName == forbidden {
						t.Errorf("Found forbidden label %s in metric %s", labelName, mf.GetName())
					}
				}
			}
		}
	}
}

func TestPrometheusMetricsSchedulerMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrometheusMetrics(reg)

	m.RecordQueueDepth("redis", "normal", 3)
	m.RecordTaskWait("normal", 12)
	m.RecordSchedulerCall("ok", 4)
	m.IncSchedulerError("timeout")
	m.RecordSchedulerBreakerState("closed")
	m.IncPriorityDowngrade("quota", "high", "normal")
	m.IncSchedulerClassificationSource("structured")

	for _, name := range []string{
		"gateway_queue_depth",
		"gateway_task_wait_duration_ms",
		"gateway_scheduler_call_duration_ms",
		"gateway_scheduler_errors_total",
		"gateway_circuit_breaker_state",
		"gateway_priority_downgrade_total",
		"gateway_scheduler_classification_source_total",
	} {
		count, err := testutil.GatherAndCount(reg, name)
		if err != nil {
			t.Fatalf("gather %s: %v", name, err)
		}
		if count == 0 {
			t.Fatalf("metric %s was not gathered", name)
		}
	}
}
