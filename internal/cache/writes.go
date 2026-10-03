package cache

import (
	"context"
	"slices"
	"time"

	"veloxmesh/internal/observability"
)

type CacheWrite struct {
	ID, Scope, Model, Text, Response string
	UsageID                          *string
	Vector                           []float32
	queuedAt                         time.Time
}

func (s *SemanticCacheService) startWorkers() {
	s.writes = make(chan CacheWrite, s.config.QueueCapacity)
	s.writeCtx, s.cancelWrites = context.WithCancel(context.Background())
	for i := 0; i < s.config.WriteWorkers; i++ {
		s.workers.Add(1)
		go s.writeWorker()
	}
}

func (s *SemanticCacheService) Enqueue(write CacheWrite) bool {
	defer observability.Stage(observability.WithTimingID(context.Background(), write.ID), "write_enqueue")()
	write.queuedAt = time.Now()
	if !s.config.Enabled {
		return false
	}
	s.queueMu.RLock()
	defer s.queueMu.RUnlock()
	if s.closed || s.writes == nil {
		recordCacheOutcome("enqueue", "closed")
		return false
	}
	if write.UsageID != nil {
		usage := *write.UsageID
		write.UsageID = &usage
	}
	write.Vector = slices.Clone(write.Vector)
	select {
	case s.writes <- write:
		recordCacheOutcome("enqueue", "accepted")
		return true
	default:
		recordCacheOutcome("enqueue", "queue_full")
		return false
	}
}

func (s *SemanticCacheService) writeWorker() {
	defer s.workers.Done()
	for write := range s.writes {
		if s.writeCtx.Err() != nil {
			recordCacheOutcome("store", "shutdown_drop")
			continue
		}
		ctx, cancel := context.WithTimeout(s.writeCtx, s.config.WriteTimeout)
		ctx = observability.WithTimingID(ctx, write.ID)
		observeQueueWait(write.ID, write.queuedAt)
		err := s.StoreWrite(ctx, write)
		if ctx.Err() == context.Canceled {
			recordCacheOutcome("store", "shutdown_cancelled")
		} else if ctx.Err() != nil {
			recordCacheOutcome("store", "timeout")
		} else if err == nil {
			recordCacheOutcome("store", "stored")
		}
		cancel()
	}
}

func observeQueueWait(id string, queuedAt time.Time) {
	if observer, ok := observability.DefaultMetrics.(interface {
		RecordStage(observability.StageMeasurement)
	}); ok {
		observer.RecordStage(observability.StageMeasurement{ID: id, Name: "write_queue_wait", Started: queuedAt, Elapsed: time.Since(queuedAt)})
	}
}

func (s *SemanticCacheService) Close() {
	s.closeOnce.Do(func() {
		s.queueMu.Lock()
		s.closed = true
		if s.writes != nil {
			close(s.writes)
		}
		s.queueMu.Unlock()
		if s.cancelWrites == nil {
			return
		}
		defer s.cancelWrites()
		done := make(chan struct{})
		go func() { s.workers.Wait(); close(done) }()
		timer := time.NewTimer(s.config.ShutdownGrace)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			s.cancelWrites()
			for range s.writes {
				recordCacheOutcome("store", "shutdown_drop")
			}
		}
	})
}
