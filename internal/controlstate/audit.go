package controlstate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SafeAuditMetadata redacts sensitive fields before persisting to audit log.
func SafeAuditMetadata(metadata map[string]interface{}) json.RawMessage {
	if metadata == nil {
		return nil
	}
	safe := make(map[string]interface{})
	for k, v := range metadata {
		if k == "api_key" || k == "authorization" || k == "ciphertext" || k == "nonce" || k == "raw_prompt" || k == "raw_upstream" {
			safe[k] = "***REDACTED***"
		} else {
			safe[k] = v
		}
	}
	b, _ := json.Marshal(safe)
	return json.RawMessage(b)
}

func (s *AdminProviderService) RecordAudit(ctx context.Context, action string, targetID string, outcome string, metadata map[string]interface{}) {
	actor := "system"
	// Assuming "admin_user" or similar is set by middleware. Let's use "admin_subject" or what the middleware uses.
	// In the absence of specifics from the prompt about context keys, we default to "system" and check for "admin_user".
	if a, ok := ctx.Value("admin_user").(string); ok && a != "" {
		actor = a
	}

	event := &AuditEvent{
		ID:        fmt.Sprintf("%s-%d", action, time.Now().UTC().UnixNano()),
		Actor:     actor,
		Action:    action,
		TargetID:  targetID,
		Outcome:   outcome,
		Metadata:  SafeAuditMetadata(metadata),
		Timestamp: time.Now().UTC(),
	}

	_ = s.repo.Audit().Log(ctx, event)
}

func (s *AdminProviderService) PurgeAuditBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.repo.Audit().PurgeOld(ctx, cutoff.Format(time.RFC3339))
}
