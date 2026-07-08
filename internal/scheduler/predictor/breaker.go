package predictor

import "time"

type clientBreaker struct {
	events    []bool
	next      int
	count     int
	failures  int
	openedAt  time.Time
	threshold int
	recovery  time.Duration
}

func newClientBreaker(cfg PythonClientConfig) *clientBreaker {
	threshold := cfg.BreakerFailureThreshold
	if threshold < 1 {
		threshold = 3
	}
	recovery := cfg.BreakerRecoveryTimeout
	if recovery <= 0 {
		recovery = time.Minute
	}
	return &clientBreaker{events: make([]bool, threshold), threshold: threshold, recovery: recovery}
}

func (b *clientBreaker) Allow() bool {
	if b.openedAt.IsZero() {
		return true
	}
	return time.Since(b.openedAt) >= b.recovery
}

func (b *clientBreaker) Record(success bool) {
	if !b.openedAt.IsZero() {
		if success {
			b.reset()
			return
		}
		b.openedAt = time.Now()
		return
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
		b.openedAt = time.Now()
		return
	}
	b.openedAt = time.Time{}
}

func (b *clientBreaker) reset() {
	for i := range b.events {
		b.events[i] = false
	}
	b.next = 0
	b.count = 0
	b.failures = 0
	b.openedAt = time.Time{}
}
