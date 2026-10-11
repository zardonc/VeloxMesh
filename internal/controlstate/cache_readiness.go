package controlstate

import "context"

// Readiness comes from active, unexpired relational entries. Empty/pending
// scopes can miss without embedding or probing a collection not yet created.
type SemanticCacheReadinessRepository interface {
	HasCandidates(ctx context.Context, scope, model string) (bool, error)
}
