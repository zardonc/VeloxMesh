package gateway

import (
	"bytes"
	"context"
	stderrors "errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/routing"
)

type settlementUsageRepo struct {
	calls   int
	records []*controlstate.UsageRecord
	err     error
	order   *[]string
}

func (r *settlementUsageRepo) Log(_ context.Context, record *controlstate.UsageRecord) error {
	r.calls++
	copy := *record
	r.records = append(r.records, &copy)
	if r.order != nil {
		*r.order = append(*r.order, "usage_log")
	}
	return r.err
}

type settlementRepo struct {
	controlstate.Repository
	usage       *settlementUsageRepo
	settleCalls int
	settled     []*controlstate.UsageRecord
	settleErr   error
	credits     int64
	order       *[]string
}

func (r *settlementRepo) Usage() controlstate.UsageRepository { return r.usage }

func (r *settlementRepo) Settle(_ context.Context, record *controlstate.UsageRecord) error {
	r.settleCalls++
	copy := *record
	r.settled = append(r.settled, &copy)
	if r.order != nil {
		*r.order = append(*r.order, "settle")
	}
	if r.settleErr != nil {
		return r.settleErr
	}
	record.CreditsConsumed = &r.credits
	return nil
}

type settlementAggregator struct {
	calls int
	err   error
	order *[]string
}

func (a *settlementAggregator) AggregateCost(context.Context, string, string, string, int64) error {
	a.calls++
	if a.order != nil {
		*a.order = append(*a.order, "aggregate_cost")
	}
	return a.err
}

var _ hotstate.CostAggregator = (*settlementAggregator)(nil)

func newSettlementService(repo *settlementRepo, aggregator hotstate.CostAggregator) *Service {
	return &Service{repo: repo, costAggregator: aggregator}
}

func settlementRequest() *llm.LLMRequest {
	return &llm.LLMRequest{RequestID: "usage-request", Model: "model-a"}
}

func settlementDecision() routing.RoutingDecision {
	return routing.RoutingDecision{ProviderID: "provider-a"}
}

func testSettlementRepo() *settlementRepo {
	return &settlementRepo{usage: &settlementUsageRepo{}, credits: 7}
}

func TestServiceSettlementSettlesCompletedUsageOnce(t *testing.T) {
	repo := testSettlementRepo()
	aggregator := &settlementAggregator{}
	service := newSettlementService(repo, aggregator)
	usage := &llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}

	service.settleTerminal(context.Background(), settlementRequest(), settlementDecision(), classifyTerminal(nil, usage), time.Second)

	if repo.settleCalls != 1 || repo.usage.calls != 0 || aggregator.calls != 1 {
		t.Fatalf("unexpected settlement calls: settle=%d log=%d aggregate=%d", repo.settleCalls, repo.usage.calls, aggregator.calls)
	}
}

func TestServiceSettlementLogsMissingUsageWithoutDebiting(t *testing.T) {
	repo := testSettlementRepo()
	service := newSettlementService(repo, nil)

	service.settleTerminal(context.Background(), settlementRequest(), settlementDecision(), classifyTerminal(nil, nil), time.Second)

	if repo.settleCalls != 0 || repo.usage.calls != 1 {
		t.Fatalf("unexpected missing usage calls: settle=%d log=%d", repo.settleCalls, repo.usage.calls)
	}
	if got := repo.usage.records[0].Status; got != controlstate.SettlementStatusMissingUsage {
		t.Fatalf("expected missing usage status, got %q", got)
	}
}

func TestServiceSettlementSkipsNonCompletedOutcomes(t *testing.T) {
	usage := &llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}
	for name, outcome := range settlementNonCompletedOutcomes(usage) {
		t.Run(name, func(t *testing.T) {
			repo := testSettlementRepo()
			service := newSettlementService(repo, &settlementAggregator{})

			service.settleTerminal(context.Background(), settlementRequest(), settlementDecision(), outcome, time.Second)

			if repo.settleCalls != 0 || repo.usage.calls != 0 {
				t.Fatalf("non-completed outcome persisted usage: settle=%d log=%d", repo.settleCalls, repo.usage.calls)
			}
		})
	}
}

func settlementNonCompletedOutcomes(usage *llm.Usage) map[string]terminalOutcome {
	return map[string]terminalOutcome{
		"provider_error":    classifyTerminal(gatewayerrors.NewGatewayError(gatewayerrors.ProviderUnavailable, "unavailable", 503), usage),
		"client_cancelled":  classifyTerminal(context.Canceled, usage),
		"policy_rejected":   classifyTerminal(gatewayerrors.ErrPolicyBlocked, usage),
		"internal_abnormal": classifyTerminal(stderrors.New("internal failure"), usage),
	}
}

func TestServiceSettlementLogsSanitizedPersistenceFailures(t *testing.T) {
	for _, test := range settlementFailureCases() {
		t.Run(test.name, func(t *testing.T) {
			repo := testSettlementRepo()
			repo.settleErr = test.settleErr
			repo.usage.err = test.usageErr
			aggregator := &settlementAggregator{err: test.aggregateErr}
			logs := captureSettlementLogs(t)

			newSettlementService(repo, aggregator).settleCompleted(context.Background(), settlementRequest(), settlementDecision(), test.usage, time.Second)

			assertSettlementWarning(t, logs.String(), test.operation)
		})
	}
}

type settlementFailureCase struct {
	name         string
	operation    string
	usage        *llm.Usage
	settleErr    error
	usageErr     error
	aggregateErr error
}

func settlementFailureCases() []settlementFailureCase {
	secret := stderrors.New("request content must not appear")
	return []settlementFailureCase{
		{name: "settle", operation: "settle", usage: &llm.Usage{TotalTokens: 1}, settleErr: secret},
		{name: "usage_log", operation: "usage_log", usageErr: secret},
		{name: "aggregate", operation: "aggregate_cost", usage: &llm.Usage{TotalTokens: 1}, aggregateErr: secret},
	}
}

func captureSettlementLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	previous := slog.Default()
	buffer := &bytes.Buffer{}
	slog.SetDefault(slog.New(slog.NewJSONHandler(buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return buffer
}

func assertSettlementWarning(t *testing.T, logs, operation string) {
	t.Helper()
	if strings.Count(logs, "usage persistence failed") != 1 || !strings.Contains(logs, `"operation":"`+operation+`"`) {
		t.Fatalf("missing operation warning %q: %s", operation, logs)
	}
	if strings.Contains(logs, "request content must not appear") {
		t.Fatalf("persistence warning included sensitive error text: %s", logs)
	}
}

func TestStreamTerminalSettlesBeforeReleaseDespitePersistenceFailure(t *testing.T) {
	order := []string{}
	repo := testSettlementRepo()
	repo.settleErr = stderrors.New("persistence unavailable")
	repo.order = &order
	store := health.NewInMemoryStore()
	store.EnsureProvider("provider-a", 3, 1)
	store.BeginRequest("provider-a")
	service := newSettlementService(repo, nil)
	service.healthStore = store
	service.cb = NewCircuitBreaker(CircuitBreakerConfig{})
	terminal := service.newStreamTerminal(streamTerminalConfig{
		ctx: context.Background(), req: settlementRequest(), decision: settlementDecision(), settlementDecision: settlementDecision(),
		model: "model-a", start: time.Now(), settle: true,
		release: func() { order = append(order, "release") },
	})

	terminal.complete(nil, &llm.Usage{TotalTokens: 1}, 0)
	terminal.complete(nil, &llm.Usage{TotalTokens: 1}, 0)

	if strings.Join(order, ",") != "settle,release" {
		t.Fatalf("expected settlement then one release, got %v", order)
	}
}
