package scheduler

import (
	"errors"
	"sync"
	"time"

	"veloxmesh/internal/config"
)

const (
	RolloutAlertMAPEDegradation     = "mape_degradation"
	RolloutAlertSchedulerErrorSpike = "scheduler_error_spike"
)

type SchedulerRolloutAlert struct {
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type SchedulerRolloutStatus struct {
	ONNXRolloutPercent      int                     `json:"onnx_rollout_percent"`
	HeuristicEnabled        bool                    `json:"heuristic_enabled"`
	ONNXEnabled             bool                    `json:"onnx_enabled"`
	QualityMAPEAlertPercent float64                 `json:"quality_mape_alert_percent"`
	ErrorSpikeAlertRate     float64                 `json:"error_spike_alert_rate"`
	Alerts                  []SchedulerRolloutAlert `json:"alerts"`
}

type SchedulerRolloutController struct {
	mu     sync.RWMutex
	status SchedulerRolloutStatus
}

func NewSchedulerRolloutController(cfg config.SchedulerConfig) *SchedulerRolloutController {
	return &SchedulerRolloutController{status: SchedulerRolloutStatus{
		ONNXRolloutPercent:      cfg.ONNXRolloutPercent,
		HeuristicEnabled:        cfg.Enabled && (cfg.HeuristicEndpoint != "" || cfg.Endpoint != ""),
		ONNXEnabled:             cfg.Enabled && cfg.ONNXEndpoint != "",
		QualityMAPEAlertPercent: cfg.QualityMAPEAlertPercent,
		ErrorSpikeAlertRate:     cfg.ErrorSpikeAlertRate,
		Alerts:                  []SchedulerRolloutAlert{},
	}}
}

func (c *SchedulerRolloutController) Snapshot() SchedulerRolloutStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneRolloutStatus(c.status)
}

func (c *SchedulerRolloutController) SetONNXRolloutPercent(percent int) (SchedulerRolloutStatus, error) {
	if percent < 0 || percent > 100 {
		return SchedulerRolloutStatus{}, errors.New("onnx_rollout_percent must be between 0 and 100")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	next := cloneRolloutStatus(c.status)
	next.ONNXRolloutPercent = percent
	c.status = next
	return cloneRolloutStatus(c.status), nil
}

func (c *SchedulerRolloutController) RecordAlert(reason string, message string) SchedulerRolloutStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := cloneRolloutStatus(c.status)
	next.Alerts = append(next.Alerts, SchedulerRolloutAlert{Reason: reason, Message: message, CreatedAt: time.Now().UTC()})
	c.status = next
	return cloneRolloutStatus(c.status)
}

func (c *SchedulerRolloutController) RolloutPercent() int {
	return c.Snapshot().ONNXRolloutPercent
}

func cloneRolloutStatus(status SchedulerRolloutStatus) SchedulerRolloutStatus {
	status.Alerts = append([]SchedulerRolloutAlert(nil), status.Alerts...)
	return status
}
