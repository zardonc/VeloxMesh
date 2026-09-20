package scheduler

import (
	"sync"
	"time"
)

const (
	breakerStateClosed   = "closed"
	breakerStateHalfOpen = "half_open"
	breakerStateOpen     = "open"
	breakerStateUnknown  = "unknown"
)

type BreakerConfig struct {
	FailureThreshold int
	RecoveryTimeout  time.Duration
}

type BreakerSnapshot struct {
	State    string
	Failures int
	Count    int
}

type Breaker struct {
	mu        sync.Mutex
	events    []bool
	next      int
	count     int
	failures  int
	openedAt  time.Time
	threshold int
	recovery  time.Duration
	probing   bool
}

func NewBreaker(cfg BreakerConfig) *Breaker {
	threshold := cfg.FailureThreshold
	if threshold < 1 {
		threshold = 3
	}
	recovery := cfg.RecoveryTimeout
	if recovery <= 0 {
		recovery = time.Minute
	}
	return &Breaker{events: make([]bool, threshold), threshold: threshold, recovery: recovery}
}

func (b *Breaker) Allow() bool {
	return b.AllowAt(time.Now())
}

func (b *Breaker) AllowAt(now time.Time) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openedAt.IsZero() {
		return true
	}
	if now.Sub(b.openedAt) < b.recovery || b.probing {
		return false
	}
	b.probing = true
	return true
}

func (b *Breaker) State() string {
	return b.SnapshotAt(time.Now()).State
}

func (b *Breaker) SnapshotAt(now time.Time) BreakerSnapshot {
	if b == nil {
		return BreakerSnapshot{State: breakerStateUnknown}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.snapshotLocked(now)
}

func (b *Breaker) Record(success bool) BreakerSnapshot {
	return b.RecordAt(time.Now(), success)
}

func (b *Breaker) RecordAt(now time.Time, success bool) BreakerSnapshot {
	if b == nil {
		return BreakerSnapshot{State: breakerStateUnknown}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.openedAt.IsZero() {
		b.probing = false
		if success {
			b.resetLocked()
			return b.snapshotLocked(now)
		}
		b.openedAt = now
		return b.snapshotLocked(now)
	}
	if b.count == b.threshold && !b.events[b.next] {
		b.failures--
	}
	b.events[b.next] = success
	b.next = (b.next + 1) % b.threshold
	if b.count < b.threshold {
		b.count++
	}
	if !success {
		b.failures++
	}
	if b.count >= b.threshold && b.failures*2 >= b.count {
		b.openedAt = now
		b.probing = false
	}
	return b.snapshotLocked(now)
}

func (b *Breaker) resetLocked() {
	for i := range b.events {
		b.events[i] = false
	}
	b.next = 0
	b.count = 0
	b.failures = 0
	b.openedAt = time.Time{}
	b.probing = false
}

func (b *Breaker) snapshotLocked(now time.Time) BreakerSnapshot {
	state := breakerStateClosed
	if !b.openedAt.IsZero() {
		state = breakerStateOpen
		if now.Sub(b.openedAt) >= b.recovery {
			state = breakerStateHalfOpen
		}
	}
	return BreakerSnapshot{State: state, Failures: b.failures, Count: b.count}
}

type breaker = Breaker

func newBreaker(threshold int, recovery time.Duration) *breaker {
	return NewBreaker(BreakerConfig{FailureThreshold: threshold, RecoveryTimeout: recovery})
}
