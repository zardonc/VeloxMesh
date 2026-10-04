package providers

import (
	"context"
	"net/http"
	"time"

	gwerr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

type protectedStreamRun struct {
	parent          context.Context
	attempt         *protectedAttempt
	source          <-chan llm.StreamEvent
	events          chan llm.StreamEvent
	provider, model string
	idleDuration    time.Duration
	idle            *time.Timer
	idleC           <-chan time.Time
	receivedContent bool
}

func (adapter *protectedAdapter) startStream(ctx context.Context, request *llm.LLMRequest, call func(context.Context, *llm.LLMRequest) (<-chan llm.StreamEvent, error)) (<-chan llm.StreamEvent, error) {
	run, err := adapter.policy.begin(ctx)
	if err != nil {
		return nil, err
	}
	source, err := call(run.ctx, request)
	if err = run.resultError(err); err != nil {
		run.finish()
		return nil, err
	}
	if source == nil {
		run.finish()
		return nil, gwerr.NewGatewayError(gwerr.ProviderBadResponse, "Provider returned no stream", http.StatusBadGateway)
	}
	events := make(chan llm.StreamEvent, 1)
	stream := &protectedStreamRun{parent: ctx, attempt: run, source: source, events: events,
		provider: adapter.ID(), model: request.Model, idleDuration: adapter.policy.StreamIdle}
	go stream.relay()
	return events, nil
}

func (stream *protectedStreamRun) relay() {
	defer stream.finish()
	for {
		event, open, err := stream.receive()
		if err != nil {
			stream.sendError(err)
			return
		}
		if !open {
			return
		}
		meaningful := meaningfulStreamEvent(event)
		if !meaningful && event.Usage == nil && !event.Done && event.Error == nil {
			continue
		}
		if meaningful {
			stream.attempt.contentReceived()
			stream.receivedContent = true
		}
		stream.pauseIdle()
		if !stream.send(event) {
			stream.sendError(stream.attempt.resultError(context.Canceled))
			return
		}
		if event.Done || event.Error != nil {
			return
		}
		if stream.receivedContent {
			stream.resumeIdle()
		}
	}
}

func meaningfulStreamEvent(event llm.StreamEvent) bool {
	return event.DeltaContent != "" || len(event.ToolCalls) > 0 || event.FinishReason != ""
}

func (stream *protectedStreamRun) receive() (llm.StreamEvent, bool, error) {
	select {
	case <-stream.attempt.ctx.Done():
		return llm.StreamEvent{}, false, context.Cause(stream.attempt.ctx)
	case <-stream.idleC:
		err := protectionTimeout(gwerr.ProviderStreamIdleTimeout)
		stream.attempt.cancel(err)
		return llm.StreamEvent{}, false, err
	case event, open := <-stream.source:
		return event, open, stream.attempt.resultError(nil)
	}
}

func (stream *protectedStreamRun) send(event llm.StreamEvent) bool {
	select {
	case stream.events <- event:
		return true
	case <-stream.attempt.ctx.Done():
		return false
	}
}

func (stream *protectedStreamRun) sendError(err error) {
	if err == nil {
		return
	}
	select {
	case stream.events <- llm.StreamEvent{Error: err, Provider: stream.provider, Model: stream.model}:
	case <-stream.parent.Done():
	}
}

func (stream *protectedStreamRun) pauseIdle() {
	if stream.idle == nil {
		return
	}
	if !stream.idle.Stop() {
		select {
		case <-stream.idle.C:
		default:
		}
	}
	stream.idleC = nil
}

func (stream *protectedStreamRun) resumeIdle() {
	if stream.idleDuration <= 0 {
		return
	}
	if stream.idle == nil {
		stream.idle = time.NewTimer(stream.idleDuration)
	} else {
		stream.idle.Reset(stream.idleDuration)
	}
	stream.idleC = stream.idle.C
}

func (stream *protectedStreamRun) finish() {
	close(stream.events)
	stream.pauseIdle()
	stream.attempt.cancel(context.Canceled)
	// Actual adapters honor context cancellation. Keep the permit until their
	// reader exits, so cancelled readers cannot oversubscribe a shared resource.
	for range stream.source {
	}
	stream.attempt.finish()
}
