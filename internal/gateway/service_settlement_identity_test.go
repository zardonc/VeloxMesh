package gateway_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
)

// Independent operations sharing a client trace must each retain their own
// usage and debit. Caching is disabled here so a legitimate hit cannot mask it.
func TestServiceSettlementIdentityRepeatedTrace(t *testing.T) {
	f := newIdentityFixture(t, identityFixtureOptions{})
	f.call(t, identityUserA, "first question")
	f.call(t, identityUserA, "second question")
	records := f.usages(t)
	assertIdentityRecords(t, records, string(controlstate.SettlementStatusSettled))
	for _, record := range records {
		if record.Key != identityUserA || !record.Credits.Valid || record.Credits.Int64 != identityRequestCost {
			t.Errorf("unexpected billed usage: %+v", record)
		}
	}
	f.assertBalance(t, identityUserA, identityInitialBalance-2*identityRequestCost)
	assertIdentityProviderTraces(t, f)
}

func TestServiceSettlementIdentityCrossUserCache(t *testing.T) {
	f := newIdentityFixture(t, identityFixtureOptions{cache: true})
	for _, key := range []string{identityUserA, identityUserB} {
		f.call(t, key, "same question")
		f.stages.waitForWrite(t)
	}
	records := f.usages(t)
	assertIdentityRecords(t, records, string(controlstate.SettlementStatusSettled))
	for _, key := range []string{identityUserA, identityUserB} {
		f.assertBalance(t, key, identityInitialBalance-identityRequestCost)
		entries := f.candidates(t, key)
		if len(entries) != 1 {
			t.Errorf("%s cache entries=%d, want 1", key, len(entries))
			continue
		}
		assertIdentityCacheLink(t, entries[0], identityCacheExpectation{key: key, records: records})
	}
	assertIdentityProviderTraces(t, f)
	assertIdentityCacheTrace(t, f)
}

func TestServiceSettlementIdentityMissingUsage(t *testing.T) {
	f := newIdentityFixture(t, identityFixtureOptions{missingUsage: true})
	f.call(t, identityUserA, "first question")
	f.call(t, identityUserA, "second question")
	records := f.usages(t)
	assertIdentityRecords(t, records, string(controlstate.SettlementStatusMissingUsage))
	for _, record := range records {
		if record.Key != identityUserA || record.Credits.Valid {
			t.Errorf("missing-usage record has wrong owner or charge: %+v", record)
		}
	}
	f.assertBalance(t, identityUserA, identityInitialBalance)
	assertIdentityProviderTraces(t, f)
}

func assertIdentityRecords(t *testing.T, records []identityUsage, status string) {
	t.Helper()
	if len(records) != 2 {
		t.Errorf("independent completions persisted %d usage records, want 2: %+v", len(records), records)
	}
	ids := map[string]bool{}
	for _, record := range records {
		assertIdentityUUID(t, record.ID)
		if ids[record.ID] || record.Status != status {
			t.Errorf("usage identity/status mismatch: %+v", record)
		}
		ids[record.ID] = true
	}
}

func assertIdentityUUID(t *testing.T, id string) {
	t.Helper()
	if _, err := uuid.Parse(id); err != nil || id == identityTraceID {
		t.Errorf("storage ID %q must be a server UUID distinct from client trace: %v", id, err)
	}
}

func assertIdentityProviderTraces(t *testing.T, f *identityFixture) {
	t.Helper()
	if len(f.adapter.traceIDs) != 2 {
		t.Errorf("provider calls=%d, want 2", len(f.adapter.traceIDs))
	}
	for _, id := range f.adapter.traceIDs {
		if id != identityTraceID {
			t.Errorf("provider trace=%q, want original %q", id, identityTraceID)
		}
	}
}

type identityCacheExpectation struct {
	key     string
	records []identityUsage
}

func assertIdentityCacheLink(t *testing.T, entry *controlstate.SemanticCacheEntry, expected identityCacheExpectation) {
	t.Helper()
	assertIdentityUUID(t, entry.ID)
	var choices []llm.Choice
	if err := json.Unmarshal([]byte(entry.Response), &choices); err != nil || len(choices) != 1 || choices[0].Message.Content != "answer for "+expected.key {
		t.Errorf("cache response belongs to wrong request for %s: %q", expected.key, entry.Response)
	}
	if entry.UsageID == nil || *entry.UsageID != entry.ID {
		t.Errorf("cache and usage identity mismatch: %+v", entry)
		return
	}
	for _, record := range expected.records {
		if record.ID == *entry.UsageID && record.Key == expected.key {
			return
		}
	}
	t.Errorf("%s cache entry does not link to its own usage record: %+v", expected.key, entry)
}

func assertIdentityCacheTrace(t *testing.T, f *identityFixture) {
	t.Helper()
	wanted := map[string]int{"write_enqueue": 0, "write_queue_wait": 0, "repo_write": 0, "store_total": 0}
	for _, stage := range f.stages.snapshot() {
		if _, ok := wanted[stage.Name]; !ok {
			continue
		}
		wanted[stage.Name]++
		if stage.ID != identityTraceID {
			t.Errorf("%s stage ID=%q, want client trace %q", stage.Name, stage.ID, identityTraceID)
		}
	}
	for stage, count := range wanted {
		if count != 2 {
			t.Errorf("%s measurements=%d, want 2 completed writes", stage, count)
		}
	}
}
