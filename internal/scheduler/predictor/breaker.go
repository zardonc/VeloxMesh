package predictor

import "time"

type clientBreaker struct {
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
	return &clientBreaker{threshold: threshold, recovery: recovery}
}

func (b *clientBreaker) Allow() bool {
	if b.openedAt.IsZero() {
		return true
	}
	return time.Since(b.openedAt) >= b.recovery
}

func (b *clientBreaker) Record(success bool) {
	if success {
		b.failures = 0
		b.openedAt = time.Time{}
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.openedAt = time.Now()
	}
}
