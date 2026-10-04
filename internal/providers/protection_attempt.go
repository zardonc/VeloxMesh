package providers

import (
	"context"
	"net/http"
	"net/http/httptrace"
	"time"

	gwerr "veloxmesh/internal/errors"
)

type protectedAttempt struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	release func()
	timers  []*time.Timer
	content *time.Timer
}

func (policy ProtectionPolicy) begin(ctx context.Context) (*protectedAttempt, error) {
	release, err := acquireProtectionPermit(ctx, policy.permits)
	if err != nil {
		return nil, err
	}
	child, cancel := context.WithCancelCause(ctx)
	run := &protectedAttempt{ctx: child, cancel: cancel, release: release}
	run.addTimer(policy.Total, gwerr.ProviderOverallTimeout)
	firstByte := run.addTimer(policy.FirstByte, gwerr.ProviderFirstByteTimeout)
	run.content = run.addTimer(policy.FirstContent, gwerr.ProviderFirstContentTimeout)
	if firstByte != nil {
		run.ctx = httptrace.WithClientTrace(child, &httptrace.ClientTrace{GotFirstResponseByte: func() { firstByte.Stop() }})
	}
	return run, nil
}

func protectionTimeout(code string) error {
	return gwerr.NewGatewayError(code, "Provider response exceeded the configured phase deadline", http.StatusGatewayTimeout)
}

func (run *protectedAttempt) addTimer(duration time.Duration, code string) *time.Timer {
	if duration <= 0 {
		return nil
	}
	timer := time.AfterFunc(duration, func() { run.cancel(protectionTimeout(code)) })
	run.timers = append(run.timers, timer)
	return timer
}

func (run *protectedAttempt) contentReceived() {
	if run.content != nil {
		run.content.Stop()
	}
}

func (run *protectedAttempt) resultError(err error) error {
	if cause := context.Cause(run.ctx); cause != nil {
		return cause
	}
	return err
}

func (run *protectedAttempt) finish() {
	for _, timer := range run.timers {
		timer.Stop()
	}
	run.cancel(context.Canceled)
	run.release()
}
