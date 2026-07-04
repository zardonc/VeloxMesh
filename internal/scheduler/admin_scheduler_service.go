package scheduler

import (
	"context"
	"fmt"
	"time"

	"veloxmesh/internal/controlstate"
	gwErr "veloxmesh/internal/errors"
)

type RolloutPatchRequest struct {
	ONNXRolloutPercent *int `json:"onnx_rollout_percent"`
}

type RolloutResponse struct {
	Status  SchedulerRolloutStatus                 `json:"status"`
	Rollups []*controlstate.SchedulerQualityRollup `json:"quality_rollups"`
}

type AdminSchedulerService struct {
	repo       controlstate.Repository
	controller *SchedulerRolloutController
}

func NewAdminSchedulerService(repo controlstate.Repository, controller *SchedulerRolloutController) *AdminSchedulerService {
	return &AdminSchedulerService{repo: repo, controller: controller}
}

func (s *AdminSchedulerService) Status(ctx context.Context) (*RolloutResponse, error) {
	status := s.controller.Snapshot()
	rollups, err := s.repo.SchedulerQualityRollups().ListByWindow(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Minute), "", "", "", 100)
	if err != nil {
		return nil, err
	}
	return &RolloutResponse{Status: status, Rollups: rollups}, nil
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
