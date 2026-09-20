package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/hotstate"
)

func TestPriorityResolverOnlyReturnsThreeClasses(t *testing.T) {
	resolver := NewPriorityResolver(nil)
	for _, input := range []string{"high", "normal", "low", "interactive", "batch", "background", "bad"} {
		got := resolver.Resolve(context.Background(), "id", input, "", PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh})
		if got.Resolved != PriorityHigh && got.Resolved != PriorityNormal && got.Resolved != PriorityLow {
			t.Fatalf("resolved invalid class for %q: %#v", input, got)
		}
	}
}

func TestPriorityResolverIgnoresTextPriorityInstructions(t *testing.T) {
	resolver := NewPriorityResolver(nil)
	got := resolver.Resolve(context.Background(), "id", "", "normal", PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh})
	if got.Resolved != PriorityNormal {
		t.Fatalf("text should not be an input to resolver: %#v", got)
	}
}

func TestPriorityResolverMaxPolicyDowngradesHigh(t *testing.T) {
	resolver := NewPriorityResolver(nil)
	got := resolver.Resolve(context.Background(), "id", "high", "", PriorityPolicy{Default: PriorityNormal, Max: PriorityNormal})
	if got.Resolved != PriorityNormal || got.DowngradeReason != "policy" {
		t.Fatalf("expected policy downgrade, got %#v", got)
	}
}

func TestPriorityResolverQuotaDowngradesHigh(t *testing.T) {
	resolver := NewPriorityResolver(hotstate.NewLocalHotState())
	policy := PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh, HighQuotaPerMinute: 1}
	_ = resolver.Resolve(context.Background(), "id", "high", "", policy)
	got := resolver.Resolve(context.Background(), "id", "high", "", policy)
	if got.Resolved != PriorityNormal || got.DowngradeReason != "quota" {
		t.Fatalf("expected quota downgrade, got %#v", got)
	}
}

func TestPriorityResolverQuotaUnavailableDowngradesHigh(t *testing.T) {
	resolver := NewPriorityResolver(nil)
	policy := PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh, HighQuotaPerMinute: 1}
	got := resolver.Resolve(context.Background(), "id", " high ", "", policy)
	if got.Resolved != PriorityNormal || got.DowngradeReason != "quota" || got.Rejected {
		t.Fatalf("expected fail-open quota downgrade, got %#v", got)
	}
}

func TestPriorityResolverStrictQuotaUnavailableRejects(t *testing.T) {
	resolver := NewPriorityResolver(failingLimiter{})
	policy := PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh, HighQuotaPerMinute: 1, Strict: true}
	got := resolver.Resolve(context.Background(), "id", "high", "", policy)
	if !got.Rejected || !errors.Is(got.Err, errLimiterUnavailable) {
		t.Fatalf("expected strict quota rejection, got %#v", got)
	}
}

var errLimiterUnavailable = errors.New("limiter unavailable")

type failingLimiter struct{}

func (failingLimiter) CheckAndIncrement(context.Context, string, int64, time.Duration) (int64, bool, error) {
	return 0, false, errLimiterUnavailable
}
