package replication

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
)

type mockApplier struct {
	err error
	evt ChangeEvent
}

func (m *mockApplier) Apply(ctx context.Context, evt ChangeEvent) error {
	m.evt = evt
	return m.err
}

type mockFallbackRepo struct {
	records []*controlstate.FallbackLogRecord
	err     error
}

func (m *mockFallbackRepo) Insert(ctx context.Context, record *controlstate.FallbackLogRecord) error {
	m.records = append(m.records, record)
	return m.err
}
func (m *mockFallbackRepo) ListPending(ctx context.Context, limit int) ([]*controlstate.FallbackLogRecord, error) {
	if m.err != nil {
		return nil, m.err
	}
	var res []*controlstate.FallbackLogRecord
	for _, r := range m.records {
		if r.Status == "pending" && len(res) < limit {
			res = append(res, r)
		}
	}
	return res, nil
}
func (m *mockFallbackRepo) UpdateStatus(ctx context.Context, id, status string) error {
	for _, r := range m.records {
		if r.ID == id {
			r.Status = status
			return nil
		}
	}
	return nil
}

type mockStreamProducer struct {
	err error
	evt ChangeEvent
}

func (m *mockStreamProducer) Append(ctx context.Context, event ChangeEvent) (string, error) {
	m.evt = event
	return "1-0", m.err
}

func TestConsumerLagSnapshot(t *testing.T) {
	c := &Consumer{}
	lag := c.ReportLag()
	if lag.Elapsed != 0 {
		t.Fatalf("expected 0 elapsed, got %v", lag.Elapsed)
	}

	c.lastEventTime = time.Now().Add(-10 * time.Second)
	lag = c.ReportLag()
	if lag.Elapsed < 9*time.Second || lag.Elapsed > 11*time.Second {
		t.Fatalf("expected ~10s elapsed, got %v", lag.Elapsed)
	}
}

func TestRecoveryWorker_Success(t *testing.T) {
	repo := &mockFallbackRepo{}
	applier := &mockApplier{}
	worker := NewRecoveryWorker(repo, applier)

	evt := ChangeEvent{Repository: "test", Operation: "LOG"}
	payload := SyncPayload{Event: evt, RetryCount: 0}
	b, _ := json.Marshal(payload)

	rec := &controlstate.FallbackLogRecord{
		ID:      "1",
		Type:    "sync",
		Payload: string(b),
		Status:  "pending",
	}
	_ = repo.Insert(context.Background(), rec)

	worker.ProcessPending(context.Background())

	if repo.records[0].Status != "applied" {
		t.Fatalf("expected applied, got %s", repo.records[0].Status)
	}
	if applier.evt.Repository != "test" {
		t.Fatalf("expected applier to be called with event")
	}
}

func TestRecoveryWorker_RepublishesUnstreamedEvent(t *testing.T) {
	repo := &mockFallbackRepo{}
	applier := &mockApplier{}
	producer := &mockStreamProducer{}
	worker := NewRecoveryWorker(repo, applier, producer)

	evt := ChangeEvent{Repository: "providers", Operation: "CREATE"}
	payload := SyncPayload{Event: evt, RetryCount: 0}
	b, _ := json.Marshal(payload)

	_ = repo.Insert(context.Background(), &controlstate.FallbackLogRecord{
		ID:      "1",
		Type:    "sync",
		Payload: string(b),
		Status:  "pending",
	})

	worker.ProcessPending(context.Background())

	if repo.records[0].Status != "applied" {
		t.Fatalf("expected applied, got %s", repo.records[0].Status)
	}
	if producer.evt.Repository != "providers" {
		t.Fatalf("expected producer to republish event")
	}
	if applier.evt.Repository != "" {
		t.Fatalf("expected local applier not to be called")
	}
}

func TestRecoveryWorker_FailureRetry(t *testing.T) {
	repo := &mockFallbackRepo{}
	applier := &mockApplier{err: errors.New("apply error")}
	worker := NewRecoveryWorker(repo, applier)

	evt := ChangeEvent{Repository: "test", Operation: "LOG"}
	payload := SyncPayload{Event: evt, RetryCount: 0}
	b, _ := json.Marshal(payload)

	_ = repo.Insert(context.Background(), &controlstate.FallbackLogRecord{
		ID:      "1",
		Type:    "sync",
		Payload: string(b),
		Status:  "pending",
	})

	worker.ProcessPending(context.Background())

	if repo.records[0].Status != "failed" {
		t.Fatalf("expected old record failed, got %s", repo.records[0].Status)
	}
	if len(repo.records) != 2 {
		t.Fatalf("expected new retry record, got %d", len(repo.records))
	}
	if repo.records[1].Status != "pending" {
		t.Fatalf("expected new record pending, got %s", repo.records[1].Status)
	}

	var newPayload SyncPayload
	_ = json.Unmarshal([]byte(repo.records[1].Payload), &newPayload)
	if newPayload.RetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", newPayload.RetryCount)
	}
}

func TestRecoveryWorker_ExhaustsRetries(t *testing.T) {
	repo := &mockFallbackRepo{}
	applier := &mockApplier{err: errors.New("apply error")}
	worker := NewRecoveryWorker(repo, applier)

	evt := ChangeEvent{Repository: "test", Operation: "LOG"}
	payload := SyncPayload{Event: evt, RetryCount: 3}
	b, _ := json.Marshal(payload)

	_ = repo.Insert(context.Background(), &controlstate.FallbackLogRecord{
		ID:      "1",
		Type:    "sync",
		Payload: string(b),
		Status:  "pending",
	})

	worker.ProcessPending(context.Background())

	if repo.records[0].Status != "failed" {
		t.Fatalf("expected old record failed, got %s", repo.records[0].Status)
	}
	if len(repo.records) != 1 {
		t.Fatalf("expected no new retry record after exhausting retries")
	}
}
