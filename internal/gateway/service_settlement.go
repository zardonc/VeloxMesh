package gateway

import (
	"context"
	"log/slog"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/routing"
)

const anonymousAPIKeyID = "anonymous"

func (s *Service) settleCompleted(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision, usage *llm.Usage, latency time.Duration) {
	s.settleTerminal(ctx, req, decision, classifyTerminal(nil, usage), latency)
}

func (s *Service) settleTerminal(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision, outcome terminalOutcome, latency time.Duration) {
	if s.repo == nil || outcome.kind != terminalCompleted {
		return
	}

	record := usageRecord(ctx, req, decision, terminalUsage(outcome), latency)
	if !outcome.hasUsage {
		s.logMissingUsage(record, outcome.kind)
		return
	}

	if err := s.repo.Settle(context.Background(), record); err != nil {
		logSettlementFailure("settle", outcome.kind, record, err)
		return
	}
	s.aggregateSettledCost(record, outcome.kind)
}

func usageRecord(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision, usage *llm.Usage, latency time.Duration) *controlstate.UsageRecord {
	record := &controlstate.UsageRecord{
		ID:         req.RequestID,
		ProviderID: decision.ProviderID,
		Model:      settlementModel(req, decision),
		DurationMs: latency.Milliseconds(),
		Timestamp:  time.Now().UTC(),
	}
	if usage == nil {
		record.Status = controlstate.SettlementStatusMissingUsage
	} else {
		record.PromptTokens = usage.PromptTokens
		record.ResponseTokens = usage.CompletionTokens
		record.TotalTokens = usage.TotalTokens
	}
	if identity := middleware.GetAuthIdentity(ctx); identity != nil && identity.ID != "dev-key" && identity.ID != "admin-key" {
		record.APIKeyID = &identity.ID
	}
	return record
}

func settlementModel(req *llm.LLMRequest, decision routing.RoutingDecision) string {
	if decision.UpstreamModel != "" {
		return decision.UpstreamModel
	}
	return req.Model
}

func (s *Service) logMissingUsage(record *controlstate.UsageRecord, outcome terminalOutcomeKind) {
	if err := s.repo.Usage().Log(context.Background(), record); err != nil {
		logSettlementFailure("usage_log", outcome, record, err)
	}
}

func (s *Service) aggregateSettledCost(record *controlstate.UsageRecord, outcome terminalOutcomeKind) {
	if s.costAggregator == nil || record.CreditsConsumed == nil {
		return
	}
	apiKeyID := anonymousAPIKeyID
	if record.APIKeyID != nil {
		apiKeyID = *record.APIKeyID
	}
	if err := s.costAggregator.AggregateCost(context.Background(), record.ProviderID, record.Model, apiKeyID, *record.CreditsConsumed); err != nil {
		logSettlementFailure("aggregate_cost", outcome, record, err)
	}
}

func logSettlementFailure(operation string, outcome terminalOutcomeKind, record *controlstate.UsageRecord, err error) {
	slog.Warn("usage persistence failed",
		"operation", operation,
		"terminal_kind", outcome,
		"provider", record.ProviderID,
		"model", record.Model,
		"error_class", settlementErrorClass(err),
	)
}

func settlementErrorClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case err == context.Canceled:
		return "cancelled"
	case err == context.DeadlineExceeded:
		return "deadline_exceeded"
	default:
		return "persistence_error"
	}
}
