package predictor

import "veloxmesh/internal/scheduler"

type clientBreaker = scheduler.Breaker

func newClientBreaker(cfg PythonClientConfig) *scheduler.Breaker {
	return scheduler.NewBreaker(scheduler.BreakerConfig{
		FailureThreshold: cfg.BreakerFailureThreshold,
		RecoveryTimeout:  cfg.BreakerRecoveryTimeout,
	})
}
