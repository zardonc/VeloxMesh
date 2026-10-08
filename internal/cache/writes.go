package cache

import (
	"context"
	"slices"
	"time"

	"veloxmesh/internal/observability"
)

type CacheWrite struct {
	ID, Scope, Model, Text, Response string
	TraceID                          string
	UsageID                          *string
	Vector                           []float32
	queuedAt                         time.Time
}

func (w CacheWrite) traceID() string {
	if w.TraceID != "" {
		return w.TraceID
	}
	return w.ID
}

func (s *SemanticCacheService) startWorkers() {
	s.writes = make(chan CacheWrite, s.config.QueueCapacity)
	s.writeCtx, s.cancelWrites = context.WithCancel(context.Background())
	for i := 0; i < s.config.WriteWorkers; i++ {
		s.workers.Add(1)
		go s.writeWorker(i == 0)
	}
}

func (s *SemanticCacheService) Enqueue(write CacheWrite) bool {
	defer observability.Stage(observability.WithTimingID(context.Background(), write.traceID()), "write_enqueue")()
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

func (s *SemanticCacheService) writeWorker(cleanup bool) {
	defer s.workers.Done()
	var ticks <-chan time.Time
	if cleanup {
		ticker := time.NewTicker(semanticCleanupInterval)
		defer ticker.Stop()
		ticks = ticker.C
	}
	for {
		select {
		case write, open := <-s.writes:
			if !open {
				return
			}
			s.processWrite(write)
		case <-ticks:
			s.cleanup()
		case <-s.writeCtx.Done():
			return
		}
	}
}

func (s *SemanticCacheService) processWrite(write CacheWrite) {
	if s.writeCtx.Err() != nil {
		recordCacheOutcome("store", "shutdown_drop")
		return
	}
	ctx, cancel := context.WithTimeout(s.writeCtx, s.config.WriteTimeout)
	defer cancel()
	ctx = observability.WithTimingID(ctx, write.traceID())
	observeQueueWait(write.traceID(), write.queuedAt)
	err := s.StoreWrite(ctx, write)
	if ctx.Err() == context.Canceled {
		recordCacheOutcome("store", "shutdown_cancelled")
	} else if ctx.Err() != nil {
		recordCacheOutcome("store", "timeout")
	} else if err == nil {
		recordCacheOutcome("store", "stored")
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
