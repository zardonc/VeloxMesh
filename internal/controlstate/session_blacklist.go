package controlstate

import (
	"context"
	"time"
)

type SessionBlacklistRecord struct {
	SessionHash string    `json:"session_hash"`
	Reason      string    `json:"reason"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type SessionBlacklistRepository interface {
	IsBlacklisted(ctx context.Context, sessionHash string) (bool, error)
	Blacklist(ctx context.Context, record *SessionBlacklistRecord) error
	PurgeExpired(ctx context.Context) (int64, error)
}
