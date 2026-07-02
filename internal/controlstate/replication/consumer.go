package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/pipeline"
)

type LagSnapshot struct {
	Elapsed time.Duration
	Pending int64
}

const reportLagTimeout = 500 * time.Millisecond

type Consumer struct {
	client      redis.Cmdable
	stream      string
	group       string
	consumer    string
	repo        controlstate.Repository
	fallbackLog controlstate.FallbackLogRepository

	lastEventTime time.Time
	OnApplied     func(ChangeEvent)
}

func NewConsumer(client redis.Cmdable, stream, group, consumerName string, repo controlstate.Repository, fallbackLog controlstate.FallbackLogRepository) *Consumer {
	return &Consumer{
		client:      client,
		stream:      stream,
		group:       group,
		consumer:    consumerName,
		repo:        repo,
		fallbackLog: fallbackLog,
	}
}

func (c *Consumer) ReportLag() LagSnapshot {
	var pending int64
	if c.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), reportLagTimeout)
		defer cancel()

		groups, err := c.client.XInfoGroups(ctx, c.stream).Result()
		if err != nil {
			groups = nil
		}
		for _, g := range groups {
			if g.Name == c.group {
				// g.Lag might be -1 if Redis cannot determine it yet.
				lag := g.Lag
				if lag < 0 {
					lag = 0
				}
				pending = g.Pending + lag
				break
			}
		}
	}

	var elapsed time.Duration
	if !c.lastEventTime.IsZero() {
		elapsed = time.Since(c.lastEventTime)
	}

	return LagSnapshot{
		Elapsed: elapsed,
		Pending: pending,
	}
}

func (c *Consumer) Start(ctx context.Context) {
	// Ensure group exists
	_ = c.client.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err()

	go c.loop(ctx)
}

func (c *Consumer) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		args := &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  []string{c.stream, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}

		streams, err := c.client.XReadGroup(ctx, args).Result()
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		for _, s := range streams {
			for _, msg := range s.Messages {
				if eventStr, ok := msg.Values["event"].(string); ok {
					var evt ChangeEvent
					if err := json.Unmarshal([]byte(eventStr), &evt); err == nil {
						evt.StreamID = msg.ID
						c.lastEventTime = evt.Timestamp
						if err := c.Apply(ctx, evt); err != nil {
							// Record to fallback log on failure to apply cleanly
							if c.fallbackLog != nil {
								c.recordFallback(ctx, evt, err)
							}
						} else if c.OnApplied != nil {
							c.OnApplied(evt)
						}
					}
				}
				c.client.XAck(ctx, c.stream, c.group, msg.ID)
			}
		}
	}
}

func (c *Consumer) recordFallback(ctx context.Context, evt ChangeEvent, err error) {
	if c.fallbackLog == nil {
		return
	}
	payload := SyncPayload{
		Event:      evt,
		RetryCount: 0,
	}
	b, _ := json.Marshal(payload)
	record := &controlstate.FallbackLogRecord{
		ID:        fmt.Sprintf("sync-err-%s-%s", evt.StreamID, time.Now().UTC().Format(time.RFC3339Nano)),
		Type:      "sync",
		Payload:   string(b),
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = c.fallbackLog.Insert(ctx, record)
}

func (c *Consumer) Apply(ctx context.Context, evt ChangeEvent) error {
	switch evt.Repository {
	case "providers":
		switch evt.Operation {
		case "CREATE":
			var m controlstate.ProviderMutation
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			_, err := c.repo.Providers().Create(ctx, &m)
			return err
		case "UPDATE":
			var m controlstate.ProviderMutation
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			_, err := c.repo.Providers().Update(ctx, &m)
			return err
		case "DELETE":
			return c.repo.Providers().Delete(ctx, evt.TargetID)
		}
	case "providers_secrets":
		if evt.Operation == "UPDATE" {
			var payload struct {
				ID         string `json:"id"`
				Ciphertext []byte `json:"ciphertext"`
				Nonce      []byte `json:"nonce"`
				KeyID      string `json:"key_id"`
			}
			if err := json.Unmarshal(evt.Payload, &payload); err != nil {
				return err
			}
			return c.repo.Providers().PutEncryptedSecret(ctx, payload.ID, payload.Ciphertext, payload.Nonce, payload.KeyID)
		}
	case "combos":
		switch evt.Operation {
		case "CREATE":
			var m controlstate.ComboMutation
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			_, err := c.repo.Combos().Create(ctx, &m)
			return err
		case "UPDATE":
			var m controlstate.ComboMutation
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			_, err := c.repo.Combos().Update(ctx, &m)
			return err
		case "DELETE":
			return c.repo.Combos().Delete(ctx, evt.TargetID)
		}
	case "routing":
		if evt.Operation == "UPDATE" {
			var m controlstate.RoutingConfig
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.Routing().Save(ctx, &m)
		}
	case "api_keys":
		switch evt.Operation {
		case "CREATE":
			var m controlstate.APIKeyRecord
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.APIKeys().Create(ctx, &m)
		case "UPDATE":
			var m controlstate.APIKeyRecord
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.APIKeys().Update(ctx, &m)
		case "DELETE":
			return c.repo.APIKeys().Delete(ctx, evt.TargetID)
		}
	case "rates":
		switch evt.Operation {
		case "UPDATE":
			var m controlstate.ProviderModelRate
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.Rates().Save(ctx, &m)
		case "DELETE":
			// targetID format: provider:model
			return nil // simple implementation
		}
	case "usage":
		if evt.Operation == "LOG" {
			var m controlstate.UsageRecord
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.Usage().Log(ctx, &m)
		}
	case "audit":
		switch evt.Operation {
		case "LOG":
			var m controlstate.AuditEvent
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.Audit().Log(ctx, &m)
		case "PURGE":
			_, err := c.repo.Audit().PurgeOld(ctx, evt.TargetID)
			return err
		}
	case "idempotency":
		if evt.Operation == "CREATE" {
			var m controlstate.IdempotencyRecord
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.Idempotency().Save(ctx, &m)
		}
	case "semantic_cache":
		switch evt.Operation {
		case "CREATE":
			var m controlstate.SemanticCacheEntry
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.SemanticCache().Store(ctx, &m)
		case "UPDATE":
			return c.repo.SemanticCache().RecordHit(ctx, evt.TargetID)
		case "DELETE":
			return c.repo.SemanticCache().Disable(ctx, evt.TargetID)
		}
	case "semantic_rules":
		if evt.Operation == "UPDATE" {
			var m pipeline.SemanticPipelineConfig
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			if evt.TargetID == "global" {
				return c.repo.SemanticRules().SaveGlobalDefaults(ctx, &m)
			} else {
				return c.repo.SemanticRules().SaveUserConfig(ctx, evt.TargetID, &m)
			}
		}
	case "fallback_log":
		switch evt.Operation {
		case "CREATE":
			var m controlstate.FallbackLogRecord
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.FallbackLog().Insert(ctx, &m)
		case "UPDATE":
			var m map[string]string
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.FallbackLog().UpdateStatus(ctx, evt.TargetID, m["status"])
		}
	case "limit_rules":
		switch evt.Operation {
		case "CREATE":
			var m controlstate.LimitRule
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.LimitRules().Save(ctx, &m)
		case "DELETE":
			return c.repo.LimitRules().Delete(ctx, evt.TargetID)
		}
	case "session_blacklist":
		switch evt.Operation {
		case "CREATE":
			var m controlstate.SessionBlacklistRecord
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.SessionBlacklist().Blacklist(ctx, &m)
		case "PURGE":
			_, err := c.repo.SessionBlacklist().PurgeExpired(ctx)
			return err
		}
	case "transaction":
		// no-op or ignored
	case "repository":
		if evt.Operation == "SETTLE" {
			var m controlstate.UsageRecord
			if err := json.Unmarshal(evt.Payload, &m); err != nil {
				return err
			}
			return c.repo.Settle(ctx, &m)
		}
	}
	return nil
}
