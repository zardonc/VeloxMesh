package scheduler

import (
	"context"
	"fmt"
	"time"

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	gwErr "veloxmesh/internal/errors"
)

const defaultStatusRollupLimit = 100
const defaultTrainingExportLimit = 1000
const maxTrainingExportLimit = 10000
const defaultTrainingExportWindow = 30 * 24 * time.Hour

type RolloutPatchRequest struct {
	ONNXRolloutPercent *int `json:"onnx_rollout_percent"`
}

type SLARulesReplaceRequest struct {
	Rules []config.SLAPromotionRule `json:"rules"`
}

type RolloutResponse struct {
	Status  SchedulerRolloutStatus                 `json:"status"`
	Rollups []*controlstate.SchedulerQualityRollup `json:"quality_rollups"`
}

type SchedulerRuntimeStatus struct {
	RolloutStatus  SchedulerRolloutStatus                 `json:"rollout_status"`
	QueueDepth     *int64                                 `json:"queue_depth,omitempty"`
	SlotsUsed      *int                                   `json:"slots_used,omitempty"`
	SlotsTotal     *int                                   `json:"slots_total,omitempty"`
	QualityRollups []*controlstate.SchedulerQualityRollup `json:"quality_rollups"`
	Warnings       []string                               `json:"warnings"`
}

type SafeSLARule struct {
	PolicyID       string `json:"policy_id"`
	TenantSelector string `json:"tenant_selector,omitempty"`
	TenantClass    string `json:"tenant_class,omitempty"`
	ModelClass     string `json:"model_class"`
	RequestKind    string `json:"request_kind"`
	WaitThreshold  string `json:"wait_threshold"`
}

type SLARulesResponse struct {
	Rules []SafeSLARule `json:"rules"`
}

type TrainingExportRequest struct {
	Start    time.Time
	End      time.Time
	TaskType string
	Limit    int
}

type TrainingExportResponse struct {
	Samples []TrainingExportSample `json:"samples"`
}

type TrainingExportSample struct {
	Features TrainingExportFeatures `json:"features"`
	Labels   TrainingExportLabels   `json:"labels"`
}

type TrainingExportFeatures struct {
	ModelClass       string  `json:"model_class"`
	RequestKind      string  `json:"request_kind"`
	Priority         string  `json:"priority"`
	Stream           bool    `json:"stream"`
	CoverageLevel    string  `json:"coverage_level"`
	CoverageRatio    float64 `json:"coverage_ratio"`
	SchedulerVersion string  `json:"scheduler_version"`
}

type TrainingExportLabels struct {
	Outcome         string    `json:"outcome"`
	ActualLatencyMs int64     `json:"actual_latency_ms"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	ProviderClass   string    `json:"provider_class"`
	CompletedAt     time.Time `json:"completed_at"`
}

type AdminSchedulerService struct {
	repo       controlstate.Repository
	controller *SchedulerRolloutController
	runner     *SynchronousRunner
}

func NewAdminSchedulerService(repo controlstate.Repository, controller *SchedulerRolloutController, runner *SynchronousRunner) *AdminSchedulerService {
	return &AdminSchedulerService{repo: repo, controller: controller, runner: runner}
}

func (s *AdminSchedulerService) Status(ctx context.Context) (*RolloutResponse, error) {
	status := s.controller.Snapshot()
	rollups, err := s.repo.SchedulerQualityRollups().ListByWindow(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Minute), "", "", "", 100)
	if err != nil {
		return nil, err
	}
	return &RolloutResponse{Status: status, Rollups: rollups}, nil
}

func (s *AdminSchedulerService) RuntimeStatus(ctx context.Context, limit int) *SchedulerRuntimeStatus {
	resp := &SchedulerRuntimeStatus{Warnings: []string{}}
	if s.controller != nil {
		resp.RolloutStatus = s.controller.Snapshot()
	} else {
		resp.Warnings = append(resp.Warnings, "rollout_controller_unavailable")
	}
	s.addQueueStatus(ctx, resp)
	s.addSlotStatus(resp)
	s.addQualityRollups(ctx, resp, limit)
	return resp
}

func (s *AdminSchedulerService) SLARules() *SLARulesResponse {
	promoter := s.slaPromoter()
	if promoter == nil {
		return &SLARulesResponse{Rules: []SafeSLARule{}}
	}
	return &SLARulesResponse{Rules: safeSLARules(promoter.SnapshotRules())}
}

func (s *AdminSchedulerService) ReplaceSLARules(ctx context.Context, req *SLARulesReplaceRequest) (*SLARulesResponse, error) {
	promoter := s.slaPromoter()
	if promoter == nil {
		return nil, gwErr.NewGatewayError("scheduler_unavailable", "SLA promoter is unavailable", 503)
	}
	if req == nil {
		return nil, gwErr.NewGatewayError("invalid_request", "rules are required", 400)
	}
	rules := append([]config.SLAPromotionRule(nil), req.Rules...)
	if err := config.ValidateSLAPromotionRules(rules); err != nil {
		return nil, gwErr.NewGatewayError("invalid_request", err.Error(), 400)
	}
	old := promoter.ReplaceRules(rules)
	s.recordSLARulesAudit(ctx, old, rules)
	return s.SLARules(), nil
}

func (s *AdminSchedulerService) ExportTrainingSamples(ctx context.Context, req TrainingExportRequest) (*TrainingExportResponse, error) {
	if s.repo == nil || s.repo.SchedulerTrainingSamples() == nil {
		return nil, gwErr.NewGatewayError("scheduler_unavailable", "training samples are unavailable", 503)
	}
	start, end := trainingExportWindow(req.Start, req.End)
	rows, err := s.repo.SchedulerTrainingSamples().ListByWindow(ctx, start, end, trainingExportQueryLimit(req))
	if err != nil {
		return nil, err
	}
	return &TrainingExportResponse{Samples: projectTrainingSamples(rows, req)}, nil
}

func (s *AdminSchedulerService) Update(ctx context.Context, req *RolloutPatchRequest) (_ *RolloutResponse, err error) {
	if req.ONNXRolloutPercent == nil {
		return nil, gwErr.NewGatewayError("invalid_request", "onnx_rollout_percent is required", 400)
	}
	oldPercent := s.controller.Snapshot().ONNXRolloutPercent
	newPercent := *req.ONNXRolloutPercent
	outcome := "success"
	defer func() {
		if err != nil {
			outcome = "validation_failed"
		}
		s.recordAudit(ctx, outcome, map[string]interface{}{"old_percent": oldPercent, "new_percent": newPercent})
	}()
	if _, err := s.controller.SetONNXRolloutPercent(newPercent); err != nil {
		return nil, gwErr.NewGatewayError("invalid_request", err.Error(), 400)
	}
	return s.Status(ctx)
}

func (s *AdminSchedulerService) addQueueStatus(ctx context.Context, resp *SchedulerRuntimeStatus) {
	if s.runner == nil || s.runner.Executor == nil || s.runner.Executor.Queue == nil {
		resp.Warnings = append(resp.Warnings, "queue_unavailable")
		return
	}
	depth, err := s.runner.Executor.Queue.Len(ctx)
	if err != nil {
		resp.Warnings = append(resp.Warnings, "queue_depth_unavailable")
		return
	}
	resp.QueueDepth = &depth
}

func (s *AdminSchedulerService) addSlotStatus(resp *SchedulerRuntimeStatus) {
	used, total, ok := s.runner.SlotUsage()
	if !ok {
		resp.Warnings = append(resp.Warnings, "executor_slots_unavailable")
		return
	}
	resp.SlotsUsed = &used
	resp.SlotsTotal = &total
}

func (s *AdminSchedulerService) addQualityRollups(ctx context.Context, resp *SchedulerRuntimeStatus, limit int) {
	if s.repo == nil || s.repo.SchedulerQualityRollups() == nil {
		resp.Warnings = append(resp.Warnings, "quality_rollups_unavailable")
		return
	}
	rollups, err := s.repo.SchedulerQualityRollups().ListByWindow(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Minute), "", "", "", normalizedStatusLimit(limit))
	if err != nil {
		resp.Warnings = append(resp.Warnings, "quality_rollups_query_failed")
		return
	}
	resp.QualityRollups = rollups
}

func (s *AdminSchedulerService) slaPromoter() *SLAPromoter {
	if s.runner == nil || s.runner.Executor == nil {
		return nil
	}
	return s.runner.Executor.Promoter
}

func (s *AdminSchedulerService) recordAudit(ctx context.Context, outcome string, metadata map[string]interface{}) {
	if s.repo == nil || s.repo.Audit() == nil {
		return
	}
	now := time.Now().UTC()
	_ = s.repo.Audit().Log(ctx, &controlstate.AuditEvent{
		ID:        fmt.Sprintf("scheduler.rollout.update-%d", now.UnixNano()),
		Actor:     "system",
		Action:    "scheduler.rollout.update",
		TargetID:  "scheduler-rollout",
		Outcome:   outcome,
		Metadata:  controlstate.SafeAuditMetadata(metadata),
		Timestamp: now,
	})
}

func (s *AdminSchedulerService) recordSLARulesAudit(ctx context.Context, oldRules, newRules []config.SLAPromotionRule) {
	if s.repo == nil || s.repo.Audit() == nil {
		return
	}
	now := time.Now().UTC()
	_ = s.repo.Audit().Log(ctx, &controlstate.AuditEvent{
		ID:       fmt.Sprintf("scheduler.sla_rules.replace-%d", now.UnixNano()),
		Actor:    "system",
		Action:   "scheduler.sla_rules.replace",
		TargetID: "scheduler-sla-rules",
		Outcome:  "success",
		Metadata: controlstate.SafeAuditMetadata(map[string]interface{}{
			"old_count": len(oldRules),
			"new_count": len(newRules),
			"old_rules": safeSLARules(oldRules),
			"new_rules": safeSLARules(newRules),
		}),
		Timestamp: now,
	})
}

func normalizedStatusLimit(limit int) int {
	if limit <= 0 {
		return defaultStatusRollupLimit
	}
	return limit
}

func safeSLARules(rules []config.SLAPromotionRule) []SafeSLARule {
	out := make([]SafeSLARule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, SafeSLARule{
			PolicyID:       rule.PolicyID,
			TenantSelector: safeTenantSelector(rule),
			TenantClass:    rule.TenantClass,
			ModelClass:     rule.ModelClass,
			RequestKind:    rule.RequestKind,
			WaitThreshold:  rule.WaitThreshold,
		})
	}
	return out
}

func safeTenantSelector(rule config.SLAPromotionRule) string {
	if rule.TenantClass != "" {
		return "class"
	}
	if rule.TenantID != "" {
		return "id"
	}
	return ""
}

func trainingExportWindow(start time.Time, end time.Time) (time.Time, time.Time) {
	if end.IsZero() {
		end = time.Now().UTC().Add(time.Minute)
	}
	if start.IsZero() {
		start = end.Add(-defaultTrainingExportWindow)
	}
	return start, end
}

func trainingExportLimit(limit int) int {
	if limit <= 0 {
		return defaultTrainingExportLimit
	}
	return min(limit, maxTrainingExportLimit)
}

func trainingExportQueryLimit(req TrainingExportRequest) int {
	if req.TaskType != "" {
		return maxTrainingExportLimit
	}
	return trainingExportLimit(req.Limit)
}

func projectTrainingSamples(rows []*controlstate.SchedulerTrainingSample, req TrainingExportRequest) []TrainingExportSample {
	limit := trainingExportLimit(req.Limit)
	out := make([]TrainingExportSample, 0, min(len(rows), limit))
	for _, row := range rows {
		if req.TaskType != "" && row.RequestKind != req.TaskType {
			continue
		}
		out = append(out, projectTrainingSample(row))
		if len(out) == limit {
			return out
		}
	}
	return out
}

func projectTrainingSample(row *controlstate.SchedulerTrainingSample) TrainingExportSample {
	return TrainingExportSample{
		Features: TrainingExportFeatures{
			ModelClass: row.ModelClass, RequestKind: row.RequestKind, Priority: row.Priority,
			Stream: row.Stream, CoverageLevel: row.CoverageLevel, CoverageRatio: row.CoverageRatio,
			SchedulerVersion: row.SchedulerVersion,
		},
		Labels: TrainingExportLabels{
			Outcome: row.Outcome, ActualLatencyMs: row.ActualLatencyMs, InputTokens: row.InputTokens,
			OutputTokens: row.OutputTokens, ProviderClass: row.ProviderClass, CompletedAt: row.CompletedAt,
		},
	}
}
