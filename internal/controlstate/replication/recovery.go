package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"veloxmesh/internal/controlstate"
)

type Applier interface {
	Apply(ctx context.Context, evt ChangeEvent) error
}

type SyncPayload struct {
	Event      ChangeEvent `json:"event"`
	RetryCount int         `json:"retry_count"`
}

type RecoveryWorker struct {
	repo    controlstate.FallbackLogRepository
	applier Applier
}

func NewRecoveryWorker(repo controlstate.FallbackLogRepository, applier Applier) *RecoveryWorker {
	return &RecoveryWorker{
		repo:    repo,
		applier: applier,
	}
}

func (w *RecoveryWorker) Start(ctx context.Context) {
	go w.loop(ctx)
}

func (w *RecoveryWorker) loop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ProcessPending(ctx)
		}
	}
}

func (w *RecoveryWorker) ProcessPending(ctx context.Context) {
	records, err := w.repo.ListPending(ctx, 50)
	if err != nil {
		return
	}

	for _, rec := range records {
		if rec.Type != "sync" {
			// Skip unknown types or handle them if needed
			continue
		}

		var payload SyncPayload
		// Try to parse as SyncPayload first
		if err := json.Unmarshal([]byte(rec.Payload), &payload); err != nil || payload.Event.Repository == "" {
			// Fallback if it was saved directly as ChangeEvent before the wrapper was introduced
			var evt ChangeEvent
			if err := json.Unmarshal([]byte(rec.Payload), &evt); err == nil && evt.Repository != "" {
				payload = SyncPayload{
					Event:      evt,
					RetryCount: 0,
				}
			} else {
				_ = w.repo.UpdateStatus(ctx, rec.ID, "failed")
				continue
			}
		}

		err := w.applier.Apply(ctx, payload.Event)
		if err == nil {
			_ = w.repo.UpdateStatus(ctx, rec.ID, "applied")
		} else {
			_ = w.repo.UpdateStatus(ctx, rec.ID, "failed")
			
			if payload.RetryCount < 3 {
				// Insert new immutable retry record
				payload.RetryCount++
				b, _ := json.Marshal(payload)
				newRecord := &controlstate.FallbackLogRecord{
					ID:        fmt.Sprintf("sync-retry-%s-%s", payload.Event.StreamID, time.Now().UTC().Format(time.RFC3339Nano)),
					Type:      "sync",
					Payload:   string(b),
					Status:    "pending",
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				}
				_ = w.repo.Insert(ctx, newRecord)
			}
		}
	}
}
