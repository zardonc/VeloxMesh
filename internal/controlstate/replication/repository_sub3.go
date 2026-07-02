package replication

import (
	"context"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/pipeline"
)

// SemanticRules Repo
type semanticRulesRepo struct {
	underlying controlstate.SemanticRuleStore
	r          *replicatedRepository
}

func (s *semanticRulesRepo) GetGlobalDefaults(ctx context.Context) (*pipeline.SemanticPipelineConfig, error) {
	return s.underlying.GetGlobalDefaults(ctx)
}
func (s *semanticRulesRepo) GetUserConfig(ctx context.Context, userID string) (*pipeline.SemanticPipelineConfig, error) {
	return s.underlying.GetUserConfig(ctx, userID)
}
func (s *semanticRulesRepo) ListUserConfigs(ctx context.Context) (map[string]*pipeline.SemanticPipelineConfig, error) {
	return s.underlying.ListUserConfigs(ctx)
}
func (s *semanticRulesRepo) SaveGlobalDefaults(ctx context.Context, cfg *pipeline.SemanticPipelineConfig) error {
	if !s.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := s.underlying.SaveGlobalDefaults(ctx, cfg)
	if err == nil {
		evt, _ := NewChangeEvent("semantic_rules", "UPDATE", "global", cfg)
		s.r.publish(ctx, evt)
	}
	return err
}
func (s *semanticRulesRepo) SaveUserConfig(ctx context.Context, userID string, cfg *pipeline.SemanticPipelineConfig) error {
	if !s.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := s.underlying.SaveUserConfig(ctx, userID, cfg)
	if err == nil {
		evt, _ := NewChangeEvent("semantic_rules", "UPDATE", userID, cfg)
		s.r.publish(ctx, evt)
	}
	return err
}

// FallbackLog Repo
type fallbackLogRepo struct {
	underlying controlstate.FallbackLogRepository
	r          *replicatedRepository
}

func (f *fallbackLogRepo) Insert(ctx context.Context, record *controlstate.FallbackLogRecord) error {
	if !f.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := f.underlying.Insert(ctx, record)
	if err == nil {
		evt, _ := NewChangeEvent("fallback_log", "CREATE", record.ID, record)
		f.r.publish(ctx, evt)
	}
	return err
}
func (f *fallbackLogRepo) ListPending(ctx context.Context, limit int) ([]*controlstate.FallbackLogRecord, error) {
	return f.underlying.ListPending(ctx, limit)
}
func (f *fallbackLogRepo) UpdateStatus(ctx context.Context, id, status string) error {
	if !f.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := f.underlying.UpdateStatus(ctx, id, status)
	if err == nil {
		evt, _ := NewChangeEvent("fallback_log", "UPDATE", id, map[string]string{"status": status})
		f.r.publish(ctx, evt)
	}
	return err
}

// LimitRules Repo
type limitRulesRepo struct {
	underlying controlstate.LimitRuleRepository
	r          *replicatedRepository
}

func (l *limitRulesRepo) ListByTarget(ctx context.Context, scope controlstate.LimitRuleScope, targetID string) ([]*controlstate.LimitRule, error) {
	return l.underlying.ListByTarget(ctx, scope, targetID)
}
func (l *limitRulesRepo) Save(ctx context.Context, rule *controlstate.LimitRule) error {
	if !l.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := l.underlying.Save(ctx, rule)
	if err == nil {
		evt, _ := NewChangeEvent("limit_rules", "CREATE", rule.ID, rule)
		l.r.publish(ctx, evt)
	}
	return err
}
func (l *limitRulesRepo) Delete(ctx context.Context, id string) error {
	if !l.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := l.underlying.Delete(ctx, id)
	if err == nil {
		evt, _ := NewChangeEvent("limit_rules", "DELETE", id, nil)
		l.r.publish(ctx, evt)
	}
	return err
}

// SessionBlacklist Repo
type sessionBlacklistRepo struct {
	underlying controlstate.SessionBlacklistRepository
	r          *replicatedRepository
}

func (s *sessionBlacklistRepo) IsBlacklisted(ctx context.Context, sessionHash string) (bool, error) {
	return s.underlying.IsBlacklisted(ctx, sessionHash)
}
func (s *sessionBlacklistRepo) Blacklist(ctx context.Context, record *controlstate.SessionBlacklistRecord) error {
	if !s.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := s.underlying.Blacklist(ctx, record)
	if err == nil {
		evt, _ := NewChangeEvent("session_blacklist", "CREATE", record.SessionHash, record)
		s.r.publish(ctx, evt)
	}
	return err
}
func (s *sessionBlacklistRepo) PurgeExpired(ctx context.Context) (int64, error) {
	if !s.r.coord.IsWritable() {
		return 0, ErrWriteNotWritable
	}
	n, err := s.underlying.PurgeExpired(ctx)
	if err == nil {
		evt, _ := NewChangeEvent("session_blacklist", "PURGE", "", nil)
		s.r.publish(ctx, evt)
	}
	return n, err
}
