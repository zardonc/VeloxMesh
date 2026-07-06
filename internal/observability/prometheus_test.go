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

	forbiddenLabels := []string{"reqID", "requestID", "user", "api" + "_key", "pro" + "mpt"}

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
	m.IncQueueAdmission("redis", "normal", "accepted", "none")
	m.IncQueueAdmission("redis", "high", "throttled", "soft_limit")
	m.RecordTaskWait("normal", 12)
	m.RecordSchedulerCall("ok", 4)
	m.IncSchedulerError("timeout")
	m.RecordSchedulerBreakerState("closed")
	m.IncPriorityDowngrade("quota", "high", "normal")
	m.IncSchedulerClassificationSource("structured")
	m.IncSchedulerTaskLockSkip("redis", "lock_exists")

	for _, name := range []string{
		"gateway_queue_depth",
		"gateway_queue_admission_total",
		"gateway_task_wait_duration_ms",
		"gateway_scheduler_call_duration_ms",
		"gateway_scheduler_errors_total",
		"gateway_circuit_breaker_state",
		"gateway_priority_downgrade_total",
		"gateway_scheduler_classification_source_total",
		"gateway_scheduler_task_lock_skips_total",
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

func TestPrometheusQueueAdmissionLabelsAreBounded(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrometheusMetrics(reg)

	m.IncQueueAdmission("redis://secret", "tenant-priority", "raw-prompt", "api-key")
	labels := labelsForMetric(t, reg, "gateway_queue_admission_total")
	if got := labels[0]; got["backend"] != "memory" || got["priority"] != "normal" || got["outcome"] != "accepted" || got["reason"] != "none" {
		t.Fatalf("unexpected sanitized admission labels: %#v", got)
	}
	forbiddenMetricLabels(t, labels[0])
}

func TestPrometheusSchedulerPredictionQualityLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrometheusMetrics(reg)

	m.RecordSchedulerPredictionMAPE("onnx", "v1", "code_gen", "tenant", "ood", 25)
	m.RecordSchedulerComparisonWait("onnx", "v1", "code_gen", "tenant", "ood", 12)
	m.RecordSchedulerComparisonCall("onnx", "v1", "code_gen", "tenant", "ood", 4)
	m.IncSchedulerComparisonError("onnx", "v1", "code_gen")

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	qualityMetrics := map[string]bool{
		"gateway_scheduler_prediction_mape_percent":         true,
		"gateway_scheduler_comparison_wait_duration_ms":     true,
		"gateway_scheduler_comparison_call_duration_ms":     true,
		"gateway_scheduler_comparison_errors_total":         true,
		"gateway_scheduler_comparison_errors_created":       true,
		"gateway_scheduler_prediction_mape_percent_created": true,
	}
	for _, mf := range mfs {
		if !qualityMetrics[mf.GetName()] {
			continue
		}
		for _, metric := range mf.Metric {
			for _, label := range metric.Label {
				switch label.GetName() {
				case "scheduler_type", "scheduler_version", "task_type", "coverage_level", "anomaly_status", "le":
				default:
					t.Fatalf("unexpected quality metric label %q on %s", label.GetName(), mf.GetName())
				}
			}
		}
	}
}

func TestPrometheusSchedulerAnomalyStatusLabelsAreBounded(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrometheusMetrics(reg)

	m.IncSchedulerAnomalyStatus("v1", "tenant-task-type", "tenant-123", "threshold=secret")

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "gateway_scheduler_anomaly_status_total" {
			continue
		}
		for _, metric := range mf.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["task_type"] != "simple_qa" || labels["coverage_level"] != "none" || labels["anomaly_status"] != "normal" {
				t.Fatalf("unexpected sanitized labels: %#v", labels)
			}
		}
	}
}

func TestPrometheusSchedulerSLAPromotionOutcomes(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrometheusMetrics(reg)

	for _, outcome := range []string{"promoted", "not_eligible", "blocked_by_priority_or_quota", "disabled", "error"} {
		m.IncSchedulerSLAPromotion("policy-a", "gold", "large", "code_gen", "normal", outcome)
	}
	labels := labelsForMetric(t, reg, "gateway_scheduler_sla_promotion_total")
	seen := map[string]bool{}
	for _, labelSet := range labels {
		seen[labelSet["outcome"]] = true
		forbiddenMetricLabels(t, labelSet)
	}
	for _, outcome := range []string{"promoted", "not_eligible", "blocked_by_priority_or_quota", "disabled", "error"} {
		if !seen[outcome] {
			t.Fatalf("missing SLA promotion outcome label %q in %#v", outcome, labels)
		}
	}
}

func TestPrometheusSchedulerSLAPromotionLabelsAreSanitized(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrometheusMetrics(reg)

	m.IncSchedulerSLAPromotion("policy id with spaces", "tenant-123", "model with spaces", "prompt", "urgent", "surprise")
	labels := labelsForMetric(t, reg, "gateway_scheduler_sla_promotion_total")
	if len(labels) != 1 {
		t.Fatalf("expected one metric, got %#v", labels)
	}
	got := labels[0]
	want := map[string]string{
		"policy":       "unknown",
		"tenant_class": "tenant-123",
		"model_class":  "unknown",
		"request_kind": "simple_qa",
		"priority":     "normal",
		"outcome":      "error",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("label %s=%q, want %q in %#v", key, got[key], value, got)
		}
	}
	forbiddenMetricLabels(t, got)
}

func labelsForMetric(t *testing.T, reg *prometheus.Registry, name string) []map[string]string {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}
	var labels []map[string]string
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, metric := range mf.Metric {
			labelSet := map[string]string{}
			for _, label := range metric.Label {
				labelSet[label.GetName()] = label.GetValue()
			}
			labels = append(labels, labelSet)
		}
	}
	if len(labels) == 0 {
		t.Fatalf("metric %s not gathered", name)
	}
	return labels
}

func forbiddenMetricLabels(t *testing.T, labels map[string]string) {
	t.Helper()
	for _, forbidden := range []string{"tenant_id", "task_id", "prompt", "message", "api_key", "authorization", "secret", "provider_payload", "embedding", "semantic_cache_payload", "raw_task_text"} {
		if _, ok := labels[forbidden]; ok {
			t.Fatalf("forbidden label %q found in %#v", forbidden, labels)
		}
	}
}
