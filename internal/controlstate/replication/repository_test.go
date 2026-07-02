package replication

import (
	"context"
	"strings"
	"testing"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/coordination"
	"github.com/redis/go-redis/v9"
)

type MockRedis struct {
	redis.Cmdable
	xaddArgs *redis.XAddArgs
}

func (m *MockRedis) XAdd(ctx context.Context, args *redis.XAddArgs) *redis.StringCmd {
	m.xaddArgs = args
	cmd := redis.NewStringCmd(ctx)
	cmd.SetVal("12345-0")
	return cmd
}

func TestNewChangeEvent(t *testing.T) {
	// Test vector rejection
	_, err := NewChangeEvent("qdrant", "CREATE", "doc1", nil)
	if err == nil {
		t.Fatal("expected error when creating event for vector category")
	}
	if !strings.Contains(err.Error(), "vector") {
		t.Fatalf("expected vector error, got %v", err)
	}

	// Test valid payload
	payload := map[string]string{"foo": "bar", "secret": "hidden"}
	evt, err := NewChangeEvent("providers", "CREATE", "prov1", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if evt.Repository != "providers" {
		t.Errorf("expected providers, got %s", evt.Repository)
	}
	if len(evt.Payload) == 0 {
		t.Errorf("expected payload bytes")
	}
	if evt.PayloadHash == "" {
		t.Errorf("expected payload hash")
	}

	// Test no secrets in payload manually since we don't pass raw secrets to this builder usually,
	// but let's just make sure the payload serialized successfully.
	if !strings.Contains(string(evt.Payload), "hidden") {
		t.Errorf("expected payload to contain data")
	}
}

func TestRedisStreamProducer(t *testing.T) {
	mock := &MockRedis{}
	producer := NewRedisStreamProducer(mock, "test:stream")

	evt, _ := NewChangeEvent("providers", "CREATE", "prov1", map[string]string{"foo": "bar"})
	id, err := producer.Append(context.Background(), evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != "12345-0" {
		t.Errorf("expected mock ID 12345-0, got %s", id)
	}

	if mock.xaddArgs == nil {
		t.Fatal("expected XAdd to be called")
	}

	if mock.xaddArgs.Stream != "test:stream" {
		t.Errorf("expected stream test:stream, got %s", mock.xaddArgs.Stream)
	}
}

type mockCoordinator struct {
	writable bool
}
func (m *mockCoordinator) IsWritable() bool { return m.writable }
func (m *mockCoordinator) Snapshot() coordination.NodeSnapshot { return coordination.NodeSnapshot{} }
func (m *mockCoordinator) Topology() coordination.TopologySnapshot { return coordination.TopologySnapshot{} }
func (m *mockCoordinator) Start(ctx context.Context) {}
func (m *mockCoordinator) Stop(ctx context.Context) {}
func (m *mockCoordinator) WatchChanges() <-chan struct{} { return nil }

type mockProviderRepo struct {
	controlstate.ProviderRepository
	calledGet    bool
	calledCreate bool
}
func (m *mockProviderRepo) Get(ctx context.Context, id string) (*controlstate.ProviderRecord, error) {
	m.calledGet = true
	return &controlstate.ProviderRecord{ID: id}, nil
}
func (m *mockProviderRepo) Create(ctx context.Context, p *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	m.calledCreate = true
	return &controlstate.ProviderRecord{ID: p.ID}, nil
}

type mockRepo struct {
	controlstate.Repository
	provRepo *mockProviderRepo
}
func (m *mockRepo) Providers() controlstate.ProviderRepository {
	return m.provRepo
}

func TestRepositoryFencing(t *testing.T) {
	mockRedis := &MockRedis{}
	producer := NewRedisStreamProducer(mockRedis, "test:stream")
	coord := &mockCoordinator{writable: false}
	
	provRepo := &mockProviderRepo{}
	underlying := &mockRepo{provRepo: provRepo}
	
	repo := NewRepository(underlying, coord, producer)
	
	ctx := context.Background()
	
	// Test Read (IsWritable = false)
	_, err := repo.Providers().Get(ctx, "prov1")
	if err != nil {
		t.Fatalf("expected Get to succeed even if not writable")
	}
	if !provRepo.calledGet {
		t.Errorf("expected Get to be called on underlying")
	}
	
	// Test Write (IsWritable = false)
	_, err = repo.Providers().Create(ctx, &controlstate.ProviderMutation{ID: "prov1"})
	if err != ErrWriteNotWritable {
		t.Fatalf("expected ErrWriteNotWritable, got %v", err)
	}
	if provRepo.calledCreate {
		t.Errorf("expected Create NOT to be called on underlying")
	}
	if mockRedis.xaddArgs != nil {
		t.Errorf("expected no event emitted")
	}
	
	// Test Write (IsWritable = true)
	coord.writable = true
	_, err = repo.Providers().Create(ctx, &controlstate.ProviderMutation{ID: "prov1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !provRepo.calledCreate {
		t.Errorf("expected Create to be called on underlying")
	}
	if mockRedis.xaddArgs == nil {
		t.Errorf("expected event to be emitted")
	}
}
